# TG Cloud — роадмап улучшений

> Приоритизированный список улучшений приложения и сервера. Основан на слабых
> местах из `server-architecture.md` (п.9) и текущем функционале.
> Категории: критичные → продуктовые → безопасность → клиент → мониторинг.

---

## 1. Критичные (инфраструктура / стабильность)

1. **Индексы PostgreSQL** — сейчас только primary keys; JOIN `shops↔products`,
   фильтры `videos`, выборки by user идут seq-scan. Новая миграция `0005_indexes.sql`:
   - `videos(tg_chat_id, status)`, `videos(status, created_at)` — лента;
   - `likes(user_id, video_id)` — уникальные лайки;
   - `products(shop_id, category)`, `products(shop_id, status)` — каталог;
   - `orders(product_id)`, `orders(seller/buyer_id, status)` — заказы;
   - `views_log(video_id, user_id)`, `product_views(product_id, user_id)` — просмотры;
   - `reports(status)`, `comments(video_id, parent_id)` — модерация.

2. **Тесты на store-репозитории** — `orders`, `products`, `videos` непокрыты.
   Сценарии: создание заказа с серверной ценой (не из запроса), привязка продавца
   по `shop.owner_id`, ротация refresh-токенов, бан-лист, дедупликация просмотров.
   Подход: testcontainers (реальная PG+Redis) или pgx-mock.

3. **Персистентный rate-limit** — вынести из памяти (`rate_limit.go`, per-user
   leaky bucket) в Redis: не слетает при рестарте api, работает между инстансами.
   Алгоритм: fixed/sliding window, ключ `rl:<user_id>:<route>`.

4. **Кэш бан-листа в Redis** — сейчас `authMW` делает SELECT по users на каждый
   запрос. Кэш `banned:<user_id>` (TTL ~60с) + инвалидация при бан/анбан.

## 2. Продуктовые (функционал)

5. **Картинки на Telegram-хостинг** — `/v1/upload`: `sendPhoto` в канал → `file_id`
   → relay-прокси `/media/image/{file_id}` (без дискового кэша). Убрать `uploadDir`
   и `http.FileServer` → ноль файлов на нашем диске.

6. **Апдейты в реальном времени** — сейчас клиент работает по pull. Long-polling
   или WebSocket: новая лента, статусы заказов (new→paid→confirmed), новые
   комментарии. Мгновенные оповещения продавца и покупателя.

7. **Push-уведомления (FCM)** — на клиенте при новых заказах, подтверждениях,
   комментариях, жалобах (для админа).

8. **Keyset-пагинация** — для каталога/магазинов/ленты вместо `offset`:
   `where created_at < $cursor order by created_at desc limit N`. Стабильно при
   вставках, не прыгает на первых страницах.

9. **Ранжирование ленты** — использовать счётчики (просмотры/лайки) при сортировке,
   а не только `created_at`. Лёгкая вариация: `created_at desc`, определённые
   подгруппы с теплостой (hot score).

10. **Склад и валюта** — `products.stock` + жёсткая проверка `quantity <= stock`
    при заказе; `products.cost_price` для маржинальной статистики продавца;
    единый справочник валют.

## 3. Безопасность и зрелость

11. **Лимит и проверка загрузок** — `http.MaxBytesReader` на `/v1/upload`; валидация
    типа по сигнатуре байтов (JPEG/PNG/WebP), отклонение остального.

12. **Anti-replay на refresh** — защита от гонки двух параллельных `/refresh`
    с одним токеном (только первый выигрывает, второй — 401 без ошибки).

13. **Секреты наружу** — секреты из env → secret-manager (Vault/.env mounted);
    ротация JWT-ключей, раздельные ключи для access и refresh.

14. **Audit-log модерации** — таблица `admin_actions` (admin_id, action, target,
    created_at): кто и что забанил/удалил/заморозил.

15. **Бэкапы во внешнее хранилище** — сейчас `backups/` локальный docker volume;
    вынести в S3-совместимое хранилище, retention в днях, тест восстановления.

## 4. Клиентская часть (Android)

16. **Обратная связь действий** — после like/comment/видео обновлять карточки в фоне
    (сейчас многие запросы без отображения результата в списке).

17. **Offline-кэш ленты** — Room/дисковый кэш последних страниц; показ при ошибке
    сети + кнопка «перезагрузить».

18. **Светлая тема и состояния экранов** — loading / error / empty / retry на всех
    трёх экранах (сейчас в основном тёмная тема).

19. **Deep-links на сущности** — `tg://product/{id}`, `tg://shop/{id}`,
    `tg://order/{id}` для шеринга.

20. **Экран настроек** — редактирование `api_url`/`relay_url` без пересборки
    (вместо правки SharedPreferences вручную).

## 5. Мониторинг

21. **Alertmanager + алерты** — SLO: доля 5xx, latency p95, падение процессов
    (api/worker/relay), рост числа жалоб.

22. **JSON-логи + центральный сбор** — логи в JSON, сбор через Loki/Grafana,
    искомость requestLog по trace-id/route.

23. **Distributed tracing (OpenTelemetry)** — сквозные трассировки между
    api→store→relay; сейчас длительность только агрегатом.

## 6. Быстрые победы (1–2 дня каждая)

- миграция `0005_indexes.sql` (п.1);
- лимит upload + проверка типа (п.11);
- кэш бан-листа в Redis (п.4);
- idempotency refresh (п.12);
- keyset-пагинация на самом горячем пути — лента (п.8);