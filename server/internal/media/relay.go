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
	"time"

	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// Relay — медиа-прокси стриминга видео из Telegram CDN (локальный Bot API).
// Поддерживает HTTP Range (http.ServeContent) и дисковый кэш.
type Relay struct {
	repos      *store.Repos
	bot        *tgbot.Client
	cacheDir   string
	httpClient *http.Client
}

func New(repos *store.Repos, bot *tgbot.Client, cacheDir string) *Relay {
	return &Relay{
		repos:      repos,
		bot:        bot,
		cacheDir:   cacheDir,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

// cachePath — путь файла кэша для video id.
func (r *Relay) cachePath(videoID int64) string {
	return filepath.Join(r.cacheDir, "video_"+strconv.FormatInt(videoID, 10)+".mp4")
}

// Stream отдаёт видео по id с поддержкой Range.
func (r *Relay) Stream(w http.ResponseWriter, req *http.Request, videoID int64) error {
	ctx := req.Context()

	video, err := r.repos.Videos.Get(ctx, videoID)
	if err != nil {
		return ErrNotFound
	}
	if video.FileID == "" || video.Status != "visible" {
		return ErrNotFound
	}

	path := r.cachePath(videoID)

	// Залить в кэш при первом запросе.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := r.fillCache(ctx, video, path); err != nil {
			return err
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, _ := f.Stat()
	w.Header().Set("Accept-Ranges", "bytes")
	http.ServeContent(w, req, "video.mp4", st.ModTime(), f)
	return nil
}

// fillCache скачивает файл из Bot API в кэш (атомарно: .part → rename).
func (r *Relay) fillCache(ctx context.Context, video *store.Video, path string) error {
	file, err := r.bot.GetFile(video.FileID)
	if err != nil {
		return fmt.Errorf("getFile: %w", err)
	}

	// Скачиваем файл
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

	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return err
	}
	tmp := path + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer os.Remove(tmp) // игнор: если rename успешен — файла нет

	ok := false
	defer func() {
		if !ok {
			_ = out.Close()
		}
	}()
	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	ok = true
	return os.Rename(tmp, path)
}

var ErrNotFound = errors.New("media not found")

// Handler — http.Handler для роута /media/stream/{video_id}
func RelayHandler(r *Relay) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		id, err := strconv.ParseInt(req.PathValue("video_id"), 10, 64)
		if err != nil {
			http.Error(w, "bad video id", http.StatusBadRequest)
			return
		}
		if err := r.Stream(w, req, id); err != nil {
			if errors.Is(err, ErrNotFound) {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			log.Printf("relay stream: %v", err)
			http.Error(w, "relay error", http.StatusBadGateway)
		}
	}
}
