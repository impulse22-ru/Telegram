#!/bin/sh
# Восстановление базы из custom-формата дампа. Интерактивный выбор файла
# из каталога бэкапов (последние 20) + опциональный предварительный DROP.
#
# Использование:
#   ./restore.sh                     # показать список и выбрать
#   ./restore.sh <путь-или-имя-файла>  # восстановить конкретный дамп
#
# ВАЖНО: восстановление перезапишет текущие данные (в режиме clean — полный DROP).
set -e

BACKUP_DIR=${BACKUP_DIR:-backups}
DB=${PGDATABASE:-tiktok}

if [ -n "$1" ]; then
    DUMP="$1"
    case "$DUMP" in
        /*) ;;
        *) DUMP="$BACKUP_DIR/$DUMP" ;;
    esac
    [ -f "$DUMP" ] || { echo "файл не найден: $DUMP" >&2; exit 1; }
else
    echo "Список бэкапов (последние 20):"
    ls -1t "$BACKUP_DIR"/tiktok_*.dump 2>/dev/null | head -20 | nl
    echo
    printf "Введите номер файла: "
    read -r N
    F=$(ls -1t "$BACKUP_DIR"/tiktok_*.dump 2>/dev/null | head -20 | sed -n "${N}p")
    [ -n "$F" ] || { echo "некорректный номер" >&2; exit 1; }
    DUMP="$F"
fi

echo "Выбран дамп: $DUMP"

printf "Полное восстановление (DROP SCHEMA public + re/create)? [y/N] "
read -r CLEAN
if [ "$CLEAN" = "y" ] || [ "$CLEAN" = "Y" ]; then
    psql -v ON_ERROR_STOP=1 "$DB" <<'SQL'
DROP SCHEMA public CASCADE;
CREATE SCHEMA public;
GRANT ALL ON SCHEMA public TO PUBLIC;
GRANT ALL ON SCHEMA public TO "${PGUSER}";
SQL
fi

pg_restore --no-owner --no-privileges -d "$DB" "$DUMP"
echo "[restore] done: $DUMP"