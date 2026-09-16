#!/bin/sh
# Периодический бэкап Postgres (custom format). Запускается backup-сервисом
# в docker-compose каждые 6 часов; retention — 14 дней (BACKUP_RETENTION_DAYS).
#
# Формат имени: tiktok_<timestamp>.dump, custom-формат pg_dump (сжатый, с поддержкой
# pg_restore --list/--selective). Данные лягут в смонтированный том ./backups хоста.
set -e

BACKUP_DIR=${BACKUP_DIR:-/backups}
BACKUP_RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-14}
PGDATABASE=${PGDATABASE:-tiktok}

TS=$(date +%Y%m%d_%H%M%S)
DEST="$BACKUP_DIR/tiktok_$TS.dump"

echo "[backup] starting pg_dump -> $DEST"
pg_dump -Fc -f "$DEST" "$PGDATABASE"

echo "[backup] removing backups older than ${BACKUP_RETENTION_DAYS}d"
find "$BACKUP_DIR" -name "tiktok_*.dump" -mtime "+$BACKUP_RETENTION_DAYS" -delete

echo "[backup] done"