-- 0005_content_uniqueness.sql — защита от дубликатов контента в ленте.
--
-- Три уровня дедупликации (см. server-architecture.md и IMPROVEMENTS.md):
--   0. file_unique_id        — стабильный ID файла в Telegram. Одинаковый для
--                               одного и того же файла, даже если он перезалит.
--   1. content_signature     — хэш (duration, width, height, file_size). Ловит
--                               перекодированные копии без скачивания файла.
--   2. content_hash          — SHA-256 содержимого файла, вычисляется в relay
--                               во время заливки в кэш (точная проверка).
--
-- STATUS: новый статус 'duplicate' скрывает видео из ленты; уведомление об
-- обнаруженном дубликате отправляется оператору через бота (indexer).

-- Стабильный идентификатор файла из Telegram (у разных копий одного файла — одинаковый).
ALTER TABLE videos ADD COLUMN IF NOT EXISTS file_unique_id TEXT;

-- Сигнатура метаданных: sha256(duration_ms|width|height|file_size).
ALTER TABLE videos ADD COLUMN IF NOT EXISTS content_signature TEXT;

-- SHA-256 содержимого файла (заполняется relay-ом после скачивания в кэш).
ALTER TABLE videos ADD COLUMN IF NOT EXISTS content_hash TEXT;

-- Разрешаем статус 'duplicate' для скрытия повторов из ленты.
ALTER TABLE videos DROP CONSTRAINT IF EXISTS videos_status_check;
ALTER TABLE videos ADD CONSTRAINT videos_status_check
    CHECK (status IN ('visible', 'banned', 'archived', 'duplicate'));

-- Индекс по сигнатуре: быстрый поиск возможных дубликатов при индексации.
CREATE INDEX IF NOT EXISTS videos_signature_idx ON videos (content_signature)
    WHERE status = 'visible';

-- Индекс по SHA-256 контента: поиск по хэшу после заливки кэша relay-ом.
CREATE INDEX IF NOT EXISTS videos_content_hash_idx ON videos (content_hash)
    WHERE status = 'visible';

-- Индекс по стабильному ID файла: точная дедупликация перезалитых файлов.
CREATE INDEX IF NOT EXISTS videos_file_unique_idx ON videos (file_unique_id)
    WHERE status = 'visible';