# TG Cloud Server — архитектура

> Аналитический документ: как устроена серверная часть (Go) на текущий момент —
> компоненты, потоки данных, решения, слабые места. Дополняет `server/README.md`,
> который описывает запуск и проверку. `server-plan.md` — исторический план этапов.

## 1. Обзор

Единый монорепо-пакет `tgcloud/server`. В рантайме — **три процесса**, разделённых по
принципу «один процесс — одна роль», и общее хранилище:

```
cmd/api     — REST API (главный процесс)        :8080
cmd/worker  — индексер канала + команды бота    (long polling Telegram)
cmd/relay   — медиа-прокси стриминга            :8082
(БД: PostgreSQL 16 + Redis 7, поднимаются docker-compose)
```

Пакеты разделены по обязанностям:

| Пакет | Обязанность |
|---|---|
| `internal/httpapi` | маршруты, middleware, хендлеры (≈30 эндпоинтов) |
| `internal/store` | репозитории над pgx + Redis |
| `internal/auth` | JWT Manager: access + refresh (opaque, sha256 в Redis) |
| `internal/indexer` | подписка на канал, индексация видео, админ-команды бота |
| `internal/media` (relay) | HTTP Range + дисковый кэш видео из Telegram CDN |
| `internal/tgbot` | тонкая обёртка локального Bot API |
| `internal/config`, `internal/log` | конфиг из env, логгер |

Миграции — в `server/migrations/` (0001–0004), применяются контейнером `migrate`
и вручную (`psql -f ...`).

## 2. Данные: Telegram отдаёт файлы, наш сервер — метаданные и логика

**Главная идея сервиса** — «TikTok/маркетплейс поверх Telegram»:

- **Видео** хранятся на серверах Telegram в канале. В PostgreSQL — только `file_id`
  и метаданные (продолжительность, размеры, caption).
- **Релей (relay)** по запросу качает файл из Telegram CDN (через локальный Bot API)
  и отдаёт клиенту с поддержкой HTTP Range и дисковым кэшем (`media_cache/`).
  Кэш опционален: после первой отдачи файл лежит на диске relay.
- **Картинки товаров/магазинов** — пока локально: `POST /v1/upload` пишет байты
  в `uploadDir` (по умолчанию `uploads/`), раздаются через `http.FileServer`.
  Это **единственный** тип файлов на нашем диске — следующий шаг: перевести их на
  Telegram-хостинг (sendPhoto → file_id → relay-прокси), чтобы не требовать диска.

Закрытый канал ленты (`FEED_CHAT_ID`) — источник видео; воркер индексирует
`channel_post`, а по `chat_join_request` автоматически одобряет вступление
(пригласительная ссылка генерируется ботом).

## 3. Модель данных (PostgreSQL)

Миграция `0001_init.sql` + расширения 0002–0004:

- **Пользователи/каналы**: `users` (id, tg_user_id, phone, name, role, banned),
  `channels` (соответствие наших каналов и tg_chat_id).
- **Видео и вовлечённость**: `videos` (file_id, метаданные, status, reason, channel_id),
  `likes`, `comments` (deleted-флаг), `views_log`, `subscriptions`.
- **Магазины и товары**: `shops` (owner_id, tg_chat_id, payment_info, image_url),
  `products` (shop_id, цена/валюта, category, image_url, status), `product_views`, `reviews`.
- **Заказы**: `orders` (product, quantity, price_amount, contact, payment_status,
  notified_at), `order_chat_link` (связка заказа с чатом TG).
- **Модерация**: `reports`, `filter_words`.

Лента — Redis ZSET (по каждому каналу), счётчики вовлечённости — тоже.

## 4. Авторизация

- **Access**: JWT (HS256), payload — `user_id`, `tg_user_id`, `role`. Время жизни ~15 мин.
- **Refresh**: opaque-токен (32 случайных байта в hex), в Redis **только sha256(raw)**
  ключ `refresh:<hash>` → user_id, TTL = 30 дней (настраивается `JWT_REFRESH_EXPIRY`).
  Интерфейс `auth.SessionStore`, реализация — `store/sessions_repo.go` (Redis).
- **Поток**: `POST /v1/auth/telegram` (public) → пара access+refresh.
  `POST /v1/auth/refresh` (public) → ротация: старый отзывается, выдаётся новый.
  `POST /v1/auth/logout` → отзыв refresh-токена.
- **Роли**: `user` / `admin`. Бан — флаг в БД, проверяется на каждый запрос в authMW.

## 5. Маршруты и middleware

```
requestLog → corsMW → mux
   ├─ public:             /health, /metrics, /v1/auth/telegram|refresh|logout
   ├─ authMW:             feed, search, videos(+stats/comments/like/comment/report/view),
   │                      shops, catalog(+categories), products, orders, stats, join
   ├─ authMW + rlMW:      like, unlike, comment, report, review   (гонко-чувствительные)
   ├─ authMW + adminMW:   /v1/admin/*  (запрет не-админам)
   └─ /uploads/*          → http.FileServer (дисковые файлы картинок)
```

Порядок middleware важен: `authMW` кладёт claims в `context`; `rlMW` и `adminMW`
читают их оттуда.

## 6. Потоки данных

1. **Публикация видео**: оператор постит ролик в канал → worker получает `channel_post`
   → `indexVideo` (дедупликация по `tg_msg_id`) → PG → лента в Redis.
2. **Аутентификация**: клиент шлёт `tg_user_id` (+ phone/name) → сервер находит/создаёт
   user → возвращает access + refresh.
3. **Стриминг**: клиент → relay `/media/stream/{id}` → проверка `status='visible'` →
   кэш-файл есть? → отдать через `http.ServeContent` (Range/Last-Modified) /
   иначе `fillCache` (getFile → download → `.part` → атомарный rename).
4. **Заказ**: клиент → `POST /v1/order` → сервер берёт цену с товара (не из запроса),
   привязывает продавца по `shop.owner_id` → уведомление продавцу; продавец
   подтверждает (`/confirm`), покупатель оплачивает (`/pay`).

## 7. Качество и мониторинг

- **Лог**: структурированный `internal/log`; requestLog пишет method/path/status/dur.
- **Метрики**: `GET /metrics` (Prometheus text format) на api — счётчики запросов,
  статусы, длительность, uptime; на relay — `relay_streams_total`, `relay_bytes_total`,
  `relay_cache_hits_total`, `relay_not_found_total`.
- **Compose**: prometheus + grafana (провижининг дашборда TG Cloud) + postgres-exporter;
  сервис `backup` (pg_dump -Fc каждые 6 часов, retention 14 дней, `scripts/backup.sh`/`restore.sh`).

## 8. Сильные стороны

- Чистое разделение слоёв; репозитории за интерфейсами (`Products`, `Users`, …) —
  тестируемо.
- Двухтокенная auth с ротацией и sha256-хэшами в Redis — выше типовой защиты.
- Атомарный кэш relay (`.part` → rename), HTTP Range через стандартную библиотеку.
- Метрики + мониторинг + бэкапы с рождения.

## 9. Слабые места / риски (роадмап)

1. **authMW делает SELECT на бан каждый запрос** — при росте нужен кэш бан-листа в Redis.
2. **Rate-limit в памяти api** — слетает при рестарте, не разделён между инстансами.
3. **Refresh / restart без idempotency**: два параллельных `/refresh` с одним токеном —
   один выигрывает, второму 401; клиент должен это переживать.
4. **store мало покрыт тестами** — `go test` покрывает в основном `auth`, `httpapi`,
   `indexer`; репозитории `orders`/`products` — нет.
5. **Метрики в памяти**: avg длительности вместо histogram-распределений.
6. **Миграции без производственных индексов** — seq scan при росте (JOIN shops↔products,
   фильтры videos).
7. **Картинки на нашем диске** (`/v1/upload` + FileServer) — план перевода на
   Telegram-хостинг через sendPhoto + relay-проксировать по file_id.

## 10. Запуск (кратко)

```bash
cd server
set -a; source .env; set +a        # нужны TELEGRAM_API_ID/HASH, BOT_TOKEN, FEED_CHAT_ID
docker compose up -d postgres redis # инфраструктура (миграции: контейнер migrate или psql -f)
go run ./cmd/api                     # API :8080
go run ./cmd/worker                  # индексер + бот
go run ./cmd/relay                   # медиа :8082 (опц.)
```

Подробнее — `server/README.md`.