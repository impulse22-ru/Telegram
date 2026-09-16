package media

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// Relay — медиа-прокси стриминга видео из Telegram CDN (локальный Bot API).
// Поддерживает HTTP Range (http.ServeContent) и дисковый кэш.
//
// Как это работает:
//  1. Клиент запрашивает GET /media/stream/{video_id} с HTTP Range
//     (т.е. с байтовым диапазоном) — так работают плееры при перемотке.
//  2. Relay смотрит в кэш: media_cache/video_<id>.mp4.
//     - файл есть → отдаём через http.ServeContent (Range + Last-Modified + ETag).
//     - файла нет → fillCache: getFile → скачивание → атомарный write в кэш.
//  3. После первого запроса файл лежит на диске — дальнейшие стримы мгновенные.
//
// HTTP Range реализуется не вручную, а через стандартный http.ServeContent,
// который сам разбирает заголовок Range и отвечает 206 Partial Content.
type Relay struct {
	repos      *store.Repos  // доступ к видео по ID (проверка существования и статуса)
	bot        *tgbot.Client // клиент Bot API: getFile + download URL
	cacheDir   string        // директория для кэш-файлов .mp4
	httpClient *http.Client  // клиент скачивания (таймаут 5 мин — большие видео)
	metrics    *RelayMetrics // счётчики стримов, кэш-хитов, байт и ошибок
}

// RelayMetrics — простые счётчики (атомарные) для /metrics на relay.
type RelayMetrics struct {
	streams   atomic.Int64 // всего стрим-запросов
	bytes     atomic.Int64 // отданных байт
	cacheHits atomic.Int64 // стримов из кэша (без скачивания)
	notFound  atomic.Int64 // ошибок not found / недоступных видео
}

// Streams — количество стрим-запросов.
func (m *RelayMetrics) Streams() int64 { return m.streams.Load() }

// Bytes — отданных байт суммарно.
func (m *RelayMetrics) Bytes() int64 { return m.bytes.Load() }

// CacheHits — стримов из кэша.
func (m *RelayMetrics) CacheHits() int64 { return m.cacheHits.Load() }

// NotFound — ошибок not found.
func (m *RelayMetrics) NotFound() int64 { return m.notFound.Load() }

// Metrics — доступ к счётчикам relay (для экспорта в /metrics).
func (r *Relay) Metrics() *RelayMetrics { return r.metrics }

// New — конструктор Relay.
//
// Параметры:
//   - repos: репозитории store; используется Videos.Get для проверки видео.
//   - bot: клиент Bot API (получение file_path по file_id).
//   - cacheDir: путь к директории кэша (RelayCacheDir из конфига).
//
// Таймаут httpClient 5 минут рассчитан на скачивание больших видео файлов.
func New(repos *store.Repos, bot *tgbot.Client, cacheDir string) *Relay {
	return &Relay{
		repos:      repos,
		bot:        bot,
		cacheDir:   cacheDir,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		metrics:    &RelayMetrics{},
	}
}

// cachePath — путь файла кэша для video id.
// Формат: <cacheDir>/video_<videoID>.mp4. Именование по внутреннему ID
// гарантирует глобальную уникальность имени и быстрый поиск.
func (r *Relay) cachePath(videoID int64) string {
	return filepath.Join(r.cacheDir, "video_"+strconv.FormatInt(videoID, 10)+".mp4")
}

// Stream отдаёт видео по id с поддержкой Range.
//
// Последовательность действий:
//  1. Ищем видео в БД; если его нет или FileID пуст / статус != "visible" → ErrNotFound.
//  2. Проверяем наличие кэш-файла; отсутствует → заполняем кэш (fillCache).
//  3. Открываем файл и отдаём через http.ServeContent — он сам обрабатывает Range,
//     If-Modified-Since, Content-Type и Content-Length.
//  4. Возвращает nil при успехе, ErrNotFound — если видео недоступно.
//
// Потоковая передача не буферизует весь файл в память: ServeContent
// читает с диска по мере надобности (сколько потребовал Range).
func (r *Relay) Stream(w http.ResponseWriter, req *http.Request, videoID int64) error {
	ctx := req.Context()

	// Проверка: видео существует и видимое (не забанено, FileID заполнен).
	video, err := r.repos.Videos.Get(ctx, videoID)
	if err != nil {
		return ErrNotFound
	}
	if video.FileID == "" || video.Status != "visible" {
		return ErrNotFound
	}

	path := r.cachePath(videoID)

	// Залить в кэш при первом запросе.
	// os.IsNotExist проверяет «файла в кэше ещё нет» — независимо от других ошибок Stat.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := r.fillCache(ctx, video, path); err != nil {
			return err
		}
	}

	// Открываем кэш-файл; defer f.Close() освободит дескриптор после ServeContent.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Time from Stat используется как Last-Modified: годность кэша определяется mtime файла.
	st, _ := f.Stat()
	// Явно заявляем поддержку range-запросов (иначе плееры не будут перематывать).
	w.Header().Set("Accept-Ranges", "bytes")
	r.metrics.streams.Add(1)
	r.metrics.cacheHits.Add(1)
	http.ServeContent(w, req, "video.mp4", st.ModTime(), f)
	// Вносятся после ServeContent: счётчик так же отражает фактически отданные байты.
	if st != nil {
		r.metrics.bytes.Add(st.Size())
	}
	return nil
}

// fillCache — заполнение дискового кэша файлом из Telegram CDN (атомарно: .part → rename).
//
// ПОЧЕМУ АТОМАРНО: параллельные запросы на одно видео не должны видеть
// «половину» файла. Схема:
//  1. getFile → получаем file_path (путь в локальном Bot API).
//  2. GET {baseURL}/file/bot<token>/<path> → скачиваем потоковым io.Copy.
//  3. Пишем во временный файл <path>.part (в той же директории/FS).
//  4. fsync (out.Sync) + close, затем os.Rename — операция атомарная в POSIX.
//
// Если rename проходит успешно — .part не существует, defer os.Remove — no-op.
// Возвращает ошибку на любом этапе: getFile, сетевой download, запись на диск.
func (r *Relay) fillCache(ctx context.Context, video *store.Video, path string) error {
	// Шаг 1: метаданные файла — file_path для построения download URL.
	file, err := r.bot.GetFile(video.FileID)
	if err != nil {
		return fmt.Errorf("getFile: %w", err)
	}

	// Шаг 2: скачиваем файл с локального Bot API.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.bot.FileURL(file), nil)
	if err != nil {
		return fmt.Errorf("download req: %w", err)
	}
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download: http %d", resp.StatusCode)
	}

	// Шаг 3: создаём временный файл .part и пишем в него потоком.
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	// Чистим .part при ошибке; после успешного rename файла не существует.
	defer os.Remove(tmp) // игнор: если rename успешен — файла нет

	// Шаг 4: потоковое копирование из ответа HTTP в файл.
	ok := false
	defer func() {
		if !ok {
			_ = out.Close() // при ошибке закрываем, чтобы deferred Remove смог удалить
		}
	}()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}

	// Шаг 5: fsync + close — гарантируем, что данные реально на диске.
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true

	// Шаг 6: атомарный rename .part → финальный путь.
	// Повторные одновременные запросы могут писать один .part — допускается,
	// последний выигравший rename побеждает (данные одни и те же).
	return os.Rename(tmp, path)
}

// ErrNotFound — sentinel-ошибка «медиа не найдено».
// RelayHandler по ней отвечает 404, остальные ошибки — 502.
var ErrNotFound = errors.New("media not found")

// Handler — http.Handler для роута /media/stream/{video_id}.
//
// Извлекает video_id из path, парсит в int64 (иначе — 400 Bad Request).
// Вызывает Stream; результат маппится в HTTP:
//   - ErrNotFound → 404 Not Found.
//   - остальные ошибки → 502 Bad Gateway (проблема со скачиванием из Telegram CDN).
//
// Используется как http.HandlerFunc для встраивания в mux relay-процесса.
func RelayHandler(r *Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(req.PathValue("video_id"), 10, 64)
		if err != nil {
			http.Error(w, "bad video id", http.StatusBadRequest)
			return
		}
		if err := r.Stream(w, req, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				r.metrics.notFound.Add(1)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("relay stream: %v", err)
			http.Error(w, "relay error", http.StatusBadGateway)
		}
	}
}
