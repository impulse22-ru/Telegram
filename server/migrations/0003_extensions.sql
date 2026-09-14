-- 0003_extensions.sql — этапы 3, 5, 6, 7.
-- Аналитика, фильтрация, подписки, просмотры товаров, уведомления.

-- Чёрный список слов для авто-бана видео (этап 5 — фильтрация).
CREATE TABLE IF NOT EXISTS filter_words (
    id          BIGSERIAL PRIMARY KEY,
    word        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Аккаунты: чтоб различать создателей магазинов (обязательный владелец).
ALTER TABLE shops
    ALTER COLUMN owner_id DROP NOT NULL; -- категория канала может быть без владельца

-- Просмотры товаров (для аналитики продаж, этап 7).
CREATE TABLE IF NOT EXISTS product_views (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id  BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS product_views_prod_idx ON product_views (product_id, created_at);

-- Индексы для поиска видео/товаров (этап 5 — фильтрация/поиск в боте).
CREATE INDEX IF NOT EXISTS videos_caption_idx   ON videos (caption);
CREATE INDEX IF NOT EXISTS videos_title_idx     ON videos (title);
CREATE INDEX IF NOT EXISTS products_title_idx   ON products (title);
CREATE INDEX IF NOT EXISTS products_status_idx  ON products (shop_id, status);

-- Бан-статус видео уже в CHECK; добавим признак причины.
ALTER TABLE videos ADD COLUMN IF NOT EXISTS reason TEXT;

-- Расширяем статусы заказов: отменён/возврат уже есть; добавим нечитаемый default для истории.
ALTER TABLE orders ADD COLUMN IF NOT EXISTS notified_at TIMESTAMPTZ;