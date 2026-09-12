-- 0001_init.sql — схема БД TikTok-сервиса поверх Telegram.
-- Точки входа: /v1/auth/telegram, /v1/join, /v1/feed, магазины, заказы.

-- Пользователи сервиса (телеграм-аккаунты, вошедшие через клиент).
CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL PRIMARY KEY,
    tg_user_id  BIGINT NOT NULL UNIQUE,
    phone       TEXT,
    name        TEXT,
    role        TEXT NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Каналы сервиса (в т.ч. закрытый канал с видео и каналы-магазины).
CREATE TABLE IF NOT EXISTS channels (
    id          BIGSERIAL PRIMARY KEY,
    tg_chat_id  BIGINT NOT NULL UNIQUE,
    title       TEXT,
    kind        TEXT NOT NULL DEFAULT 'feed' CHECK (kind IN ('feed', 'shop')),
    is_private  BOOLEAN NOT NULL DEFAULT TRUE,
    owner_id    BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Видео-посты из канала(ов) ленты.
CREATE TABLE IF NOT EXISTS videos (
    id            BIGSERIAL PRIMARY KEY,
    tg_msg_id     BIGINT NOT NULL,
    file_id       TEXT NOT NULL,
    caption       TEXT,
    duration_ms   INTEGER,
    width         INTEGER,
    height        INTEGER,
    title         TEXT,
    tags          TEXT[] NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'visible' CHECK (status IN ('visible', 'banned', 'archived')),
    channel_id    BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    posted_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (channel_id, tg_msg_id)
);

-- Лайки/дизлайки видео.
CREATE TABLE IF NOT EXISTS likes (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id    BIGINT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, video_id)
);

-- Комментарии.
CREATE TABLE IF NOT EXISTS comments (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id    BIGINT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    text        TEXT NOT NULL,
    parent_id   BIGINT REFERENCES comments(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Аналитика просмотров.
CREATE TABLE IF NOT EXISTS views_log (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id       BIGINT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    watch_seconds  INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS views_log_video_idx ON views_log (video_id, created_at);
CREATE INDEX IF NOT EXISTS views_log_user_idx  ON views_log (user_id, created_at);

-- Подписки пользователя на каналы/магазины.
CREATE TABLE IF NOT EXISTS subscriptions (
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id  BIGINT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, channel_id)
);

-- Магазины (создаются любым пользователем черезbot-канал).
CREATE TABLE IF NOT EXISTS shops (
    id           BIGSERIAL PRIMARY KEY,
    owner_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tg_chat_id   BIGINT NOT NULL UNIQUE,
    title        TEXT NOT NULL,
    description  TEXT,
    payment_info TEXT,                 -- СБП/крипта-реквизиты продавца
    status       TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Товары (посты канала-магазина).
CREATE TABLE IF NOT EXISTS products (
    id             BIGSERIAL PRIMARY KEY,
    shop_id        BIGINT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    tg_msg_id      BIGINT NOT NULL,
    file_id        TEXT NOT NULL,
    title          TEXT NOT NULL,
    description    TEXT,
    price_amount   NUMERIC(18,2) NOT NULL DEFAULT 0,
    price_currency TEXT NOT NULL DEFAULT 'RUB',
    category       TEXT,
    status         TEXT NOT NULL DEFAULT 'on_sale' CHECK (status IN ('on_sale', 'hidden', 'sold_out')),
    posted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (shop_id, tg_msg_id)
);

-- Заказы.
CREATE TABLE IF NOT EXISTS orders (
    id              BIGSERIAL PRIMARY KEY,
    shop_id         BIGINT NOT NULL REFERENCES shops(id) ON DELETE CASCADE,
    buyer_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id      BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    quantity        INTEGER NOT NULL DEFAULT 1,
    price_amount    NUMERIC(18,2) NOT NULL,
    price_currency  TEXT NOT NULL DEFAULT 'RUB',
    payment_status  TEXT NOT NULL DEFAULT 'pending'
                    CHECK (payment_status IN ('pending', 'paid', 'confirmed', 'cancelled')),
    contact_details TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS orders_buyer_idx  ON orders (buyer_id);
CREATE INDEX IF NOT EXISTS orders_shop_idx   ON orders (shop_id);

-- Чат покупатель-продавец после подтверждения оплаты.
CREATE TABLE IF NOT EXISTS order_chat_link (
    id        BIGSERIAL PRIMARY KEY,
    order_id  BIGINT NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    tg_chat_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);