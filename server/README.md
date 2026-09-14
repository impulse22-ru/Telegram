# TG Cloud Server — этапы 0-7

Серверная часть TikTok-сервиса поверх Telegram. Полный план в `../server-plan.md`.

## Структура

```
server/
  cmd/api/      — REST API (auth, feed, shops, orders)
  cmd/worker/   — фоновая индексация канала через локальный Bot API (заготовка)
  cmd/relay/    — медиа-прокси стриминга (заготовка, этап 3)
  internal/
    config/     — конфиг из .env
    auth/       — JWT (access + refresh-политика)
    store/      — PG (pgx) + Redis + репозитории
    httpapi/    — хендлеры, middleware
    log/        — логгер
  migrations/   — SQL-схема БД
  docker-compose.yml — postgres + redis + локальный Bot API
```

## Локальный запуск

```bash
cd server

# 1. Инфраструктура (PG + Redis + локальный Bot API)
cp .env.example .env
#    → заполнить TELEGRAM_API_ID / TELEGRAM_API_HASH в .env (my.telegram.org)
docker compose up -d postgres redis

# 2. Миграции
psql "postgres://tg:tg@localhost:5432/tiktok" -f migrations/0001_init.sql
psql "postgres://tg:tg@localhost:5432/tiktok" -f migrations/0002_reports.sql
psql "postgres://tg:tg@localhost:5432/tiktok" -f migrations/0003_extensions.sql

# 3. Сервер
go run ./cmd/api          # API на :8080

# (опционально)
go run ./cmd/worker       # фоновая индексация + команды бота (getUpdates)
go run ./cmd/relay        # медиа-релей на :8082 (HTTP Range + кэш)
```

## Проверка

```bash
curl localhost:8080/health
curl localhost:8080/metrics   # мониторинг (счётчики запросов)

# Логин (подставить свой tg_user_id) → вернёт JWT
curl -X POST localhost:8080/v1/auth/telegram \
  -H 'Content-Type: application/json' \
  -d '{"tg_user_id":123456,"name":"demo"}'

# Вступление: канал должен быть в режиме запроса на вступление (request to join);
# бот — администратором. Worker автоматически одобряет chat_join_request.
curl -X POST localhost:8080/v1/join -H 'Authorization: Bearer <TOKEN>'

# Лента (с токеном из предыдущего шага)
curl localhost:8080/v1/feed -H 'Authorization: Bearer <TOKEN>'
```

## Статус по этапам

- [x] Этап 0: docker-compose (PG, Redis, локальный Bot API), конфиг, каркас
- [x] Этап 1: бот-индексация канала (worker), автодобавление пользователей в канал
- [x] Этап 2: API (like, unlike, comment, report, view), лента на Redis ZSET
- [x] Этап 3: медиа-релей (HTTP Range, кэш)
- [x] Этап 4: клиент — экран ленты + автоплей (MediaFeedActivity, deep link tg://feed)
- [x] Этап 5: кастомные команды бота, фильтрация, мониторинг
- [x] Этап 6–7: магазины (создание, товары, заказы), фичи для бизнеса

## Команды бота (этап 5)

| Команда | Назначение |
|---|---|
| `/start` | приветствие + пригласительная ссылка в канал |
| `!search <текст>` | поиск видео по названию/подписи |
| `!top` | топ видео по просмотрам |
| `!stat` | статистика ленты |
| Админ-команды | `!ban <id>`, `!unban <id>`, `!filter <слово>`, `!unfilter <слово>` |

## API (этапы 3, 5–7)

Проверка через `GET /metrics`; демо-эндпоинты аналитики (все под JWT):

```bash
# Аналитика видео (этап 3)
curl localhost:8080/v1/videos/1/stats -H 'Authorization: Bearer <TOKEN>'

# Админ-дашборд
curl localhost:8080/v1/admin/stats -H 'Authorization: Bearer <TOKEN>'
curl localhost:8080/v1/admin/top     -H 'Authorization: Bearer <TOKEN>'
curl localhost:8080/v1/admin/reports -H 'Authorization: Bearer <TOKEN>'

# Витрина магазинов и товаров (этап 6)
curl localhost:8080/v1/catalog      -H 'Authorization: Bearer <TOKEN>'

# Подписки (этап 6)
curl -X POST localhost:8080/v1/shops/1/subscribe   -H 'Authorization: Bearer <TOKEN>'
curl localhost:8080/v1/subscriptions/me            -H 'Authorization: Bearer <TOKEN>'

# Продажи продавца (этап 7)
curl localhost:8080/v1/stats/sales -H 'Authorization: Bearer <TOKEN>'
```

## Заметки

- **Локальный Bot API** снимает лимит 50 МБ → 2 ГБ (флаг `--local` в docker-compose).
- Видео в ленте клиент тянет напрямую из канала (MTProto, лимит 2 ГБ),
  ботовый релей для медиа нужен только при блокировке и для товаров.
- Товары из каналов-магазинов индексируются воркером автоматически (этап 6):
  caption-пост магазина → товар в `products`.