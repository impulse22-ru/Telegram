# TG Cloud Server — этап 0-1 (каркас)

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

# 3. Сервер
go run ./cmd/api          # API на :8080

# (опционально)
go run ./cmd/worker       # фоновая индексация (пока ticker-заглушка)
go run ./cmd/relay        # медиа-релей (пока заглушка)
```

## Проверка

```bash
curl localhost:8080/health

# Логин (подставить свой tg_user_id) → вернёт JWT
curl -X POST localhost:8080/v1/auth/telegram \
  -H 'Content-Type: application/json' \
  -d '{"tg_user_id":123456,"name":"demo"}'

# Лента (с токеном из предыдущего шага)
curl localhost:8080/v1/feed -H 'Authorization: Bearer <TOKEN>'
```

## Статус по этапам

- [x] Этап 0: docker-compose (PG, Redis, локальный Bot API), конфиг, каркас
- [ ] Этап 1: бот-индексация канала (worker), автодобавление пользователей в канал
- [ ] Этап 2: API (like, comment, report), лента на Redis ZSET
- [ ] Этап 3: медиа-релей (HTTP Range, кэш)
- [ ] Этап 4: клиент — экран ленты + автоплей (форк)
- [ ] Этап 5: кастомные команды бота, фильтрация, мониторинг
- [ ] Этап 6–7: магазины (создание, товары, заказы), фичи для бизнеса

## Заметки

- **Локальный Bot API** снимает лимит 50 МБ → 2 ГБ (флаг `--local` в docker-compose).
- Видео в ленте клиент тянет напрямую из канала (MTProto, лимит 2 ГБ),
  ботовый релей для медиа нужен только при блокировке и для товаров.