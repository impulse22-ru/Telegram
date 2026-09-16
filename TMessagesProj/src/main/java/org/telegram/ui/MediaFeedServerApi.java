package org.telegram.ui;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONArray;
import org.json.JSONObject;
import org.telegram.messenger.ApplicationLoader;

import java.io.BufferedReader;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;

/**
 * HTTP-клиент (singleton) для взаимодействия с Go-сервером медиа-фидом.
 *
 * Хранит три ключевых параметра в SharedPreferences ("media_feed_config"):
 * - apiUrl  — базовый URL API-сервера (эндпоинты /v1/...).
 * - relayUrl — URL relay-сервера (стриминг видео).
 * - token   — bearer-токен авторизации, получаемый при вызове auth().
 *
 * Все сетевые запросы выполняются синхронно через {@link HttpURLConnection}.
 * Каждый публичный метод обёрнут в один из вариантов request() и бросает
 * Exception при HTTP-ошибке (код ответа не в диапазоне 200–299).
 */
public class MediaFeedServerApi {

    // --- Имена ключей SharedPreferences ---
    // PREF_NAME — имя файла настроек.
    public static final String PREF_NAME = "media_feed_config";
    // PREF_API_URL — ключ для базового URL API.
    public static final String PREF_API_URL = "api_url";
    // PREF_RELAY_URL — ключ для URL relay-сервера.
    public static final String PREF_RELAY_URL = "relay_url";
    // PREF_TOKEN — ключ для bearer-токена.
    public static final String PREF_TOKEN = "token";

    // --- Значения по умолчанию (localhost через эмулятор Android) ---
    // DEFAULT_API_URL — API-сервер по умолчанию.
    private static final String DEFAULT_API_URL = "http://10.0.2.2:8080";
    // DEFAULT_RELAY_URL — relay-сервер по умолчанию.
    private static final String DEFAULT_RELAY_URL = "http://10.0.2.2:8082";

    // instance — единственная точка доступа (singleton).
    private static MediaFeedServerApi instance;

    // --- Текущие значения конфигурации, загружаемые из SharedPreferences ---
    // apiUrl — базовый URL API-сервера.
    private String apiUrl;
    // relayUrl — базовый URL relay-сервера (стриминг).
    private String relayUrl;
    // token — bearer-токен авторизации (null до первого auth()).
    private String token;

    // getInstance — получить единственный экземпляр singleton-а.
    // Если экземпляр ещё не создан, вызывается приватный конструктор.
    public static MediaFeedServerApi getInstance() {
        if (instance == null) {
            instance = new MediaFeedServerApi();
        }
        return instance;
    }

    // Приватный конструктор: загружает apiUrl, relayUrl, token из SharedPreferences.
    // Вызывается один раз при первом обращении к getInstance().
    private MediaFeedServerApi() {
        SharedPreferences prefs = prefs();
        apiUrl = prefs.getString(PREF_API_URL, DEFAULT_API_URL);
        relayUrl = prefs.getString(PREF_RELAY_URL, DEFAULT_RELAY_URL);
        token = prefs.getString(PREF_TOKEN, null);
    }

    // prefs — возвращает SharedPreferences для файла "media_feed_config".
    // Используется для чтения/записи конфигурации и токена.
    private static SharedPreferences prefs() {
        return ApplicationLoader.applicationContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE);
    }

    // getApiUrl — возвращает текущий базовый URL API-сервера.
    public String getApiUrl() {
        return apiUrl;
    }

    // getRelayUrl — возвращает текущий базовый URL relay-сервера (стриминг).
    public String getRelayUrl() {
        return relayUrl;
    }

    // setUrls — обновляет apiUrl и relayUrl в памяти и сохраняет в SharedPreferences.
    // Параметры: apiUrl — новый URL API-сервера; relayUrl — новый URL relay-сервера.
    public void setUrls(String apiUrl, String relayUrl) {
        this.apiUrl = apiUrl;
        this.relayUrl = relayUrl;
        prefs().edit().putString(PREF_API_URL, apiUrl).putString(PREF_RELAY_URL, relayUrl).apply();
    }

    // getToken — возвращает bearer-токен авторизации (или null, если авторизация не пройдена).
    public String getToken() {
        return token;
    }

    // streamUrl — формирует полный URL для стриминга видео по его videoId.
    // Возвращает: URL вида "<relayUrl>/media/stream/<videoId>".
    public String streamUrl(long videoId) {
        return relayUrl + "/media/stream/" + videoId;
    }

    // auth — POST /v1/auth/telegram — авторизация пользователя через Telegram.
    // Параметры: tgUserId — Telegram user ID; phone — номер телефона (опц.); name — имя (опц.).
    // Возвращает: JSONObject с полем "token", которое сохраняется как bearer-токен.
    // Исключения: Exception при HTTP-ошибке или ошибке парсинга ответа.
    public JSONObject auth(long tgUserId, String phone, String name) throws Exception {
        JSONObject body = new JSONObject();
        body.put("tg_user_id", tgUserId);
        if (phone != null) {
            body.put("phone", phone);
        }
        if (name != null) {
            body.put("name", name);
        }
        JSONObject resp = request("POST", "/v1/auth/telegram", body, null);
        token = resp.optString("token", null);
        if (token != null) {
            prefs().edit().putString(PREF_TOKEN, token).apply();
        }
        return resp;
    }

    // feed() — GET /v1/feed — загрузка ленты видео с дефолтным offset=0, limit=20.
    // Возвращает: JSONArray из объектов видео.
    public JSONArray feed() throws Exception {
        return feed(0, 20);
    }

    // feed(offset, limit) — GET /v1/feed?offset=<offset>&limit=<limit> — загрузка ленты видео.
    // Параметры: offset — смещение для пагинации; limit — максимальное кол-во видео.
    // Возвращает: JSONArray из объектов видео (поле "videos" ответа).
    // Исключения: Exception при HTTP-ошибке.
    public JSONArray feed(long offset, long limit) throws Exception {
        JSONObject resp = request("GET", "/v1/feed?offset=" + offset + "&limit=" + limit, null, token);
        return resp.optJSONArray("videos");
    }

    // hasMore — GET /v1/feed?offset=<offset>&limit=<limit> — проверка наличия ещё видео после offset.
    // Параметры: offset — текущее смещение; limit — размер страницы.
    // Возвращает: boolean — true, если есть ещё видео; false — конец ленты.
    public boolean hasMore(long offset, long limit) throws Exception {
        JSONObject resp = request("GET", "/v1/feed?offset=" + offset + "&limit=" + limit, null, token);
        return resp.optBoolean("has_more", false);
    }

    // search — GET /v1/search?q=<q> — поиск видео по запросу (URL-encoded).
    // Параметры: q — поисковый запрос.
    // Возвращает: JSONArray из объектов видео (поле "videos" ответа).
    public JSONArray search(String q) throws Exception {
        JSONObject resp = request("GET", "/v1/search?q=" + java.net.URLEncoder.encode(q, "UTF-8"), null, token);
        return resp.optJSONArray("videos");
    }

    // like — POST /v1/videos/<videoId>/like — поставить лайк видео.
    // Параметры: videoId — идентификатор видео.
    public void like(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/like", new JSONObject(), token);
    }

    // unlike — POST /v1/videos/<videoId>/unlike — снять лайк с видео.
    // Параметры: videoId — идентификатор видео.
    public void unlike(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/unlike", new JSONObject(), token);
    }

    // comment — POST /v1/videos/<videoId>/comment — оставить комментарий под видео.
    // Тело запроса: {"text": text}. Параметры: videoId — видео; text — текст комментария.
    public void comment(long videoId, String text) throws Exception {
        JSONObject body = new JSONObject();
        body.put("text", text);
        request("POST", "/v1/videos/" + videoId + "/comment", body, token);
    }

    // comments — GET /v1/videos/<videoId>/comments — получить список комментариев к видео.
    // Возвращает: JSONArray с комментариями (поле "comments" ответа).
    public JSONArray comments(long videoId) throws Exception {
        JSONObject resp = request("GET", "/v1/videos/" + videoId + "/comments", null, token);
        return resp.optJSONArray("comments");
    }

    // report — POST /v1/videos/<videoId>/report — пожаловаться на видео.
    // Тело запроса: {"reason": reason}. Параметры: videoId — видео; reason — причина жалобы.
    public void report(long videoId, String reason) throws Exception {
        JSONObject body = new JSONObject();
        body.put("reason", reason);
        request("POST", "/v1/videos/" + videoId + "/report", body, token);
    }

    // view — POST /v1/videos/<videoId>/view — зафиксировать просмотр видео (счётчик).
    // Параметры: videoId — идентификатор видео.
    public void view(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/view", new JSONObject(), token);
    }

    // --- Магазины и товары (этап 6) ---

    // catalog — GET /v1/catalog[?category=<category>] — получить каталог товаров.
    // Параметры: category — фильтр по категории (опционально, URL-encoded).
    // Возвращает: JSONArray с товарами (поле "items" ответа).
    public JSONArray catalog(String category) throws Exception {
        JSONObject resp = request("GET", "/v1/catalog" + (category != null && !category.isEmpty() ? "?category=" + java.net.URLEncoder.encode(category, "UTF-8") : ""), null, token);
        return resp.optJSONArray("items");
    }

    // shops — GET /v1/shops — получить список всех магазинов.
    // Возвращает: JSONArray с магазинами (поле "shops" ответа).
    public JSONArray shops() throws Exception {
        JSONObject resp = request("GET", "/v1/shops", null, token);
        return resp.optJSONArray("shops");
    }

    // myShops — GET /v1/shops/me — список магазинов текущего пользователя.
    // Возвращает: JSONArray с магазинами (поле "shops" ответа).
    public JSONArray myShops() throws Exception {
        JSONObject resp = request("GET", "/v1/shops/me", null, token);
        return resp.optJSONArray("shops");
    }

    // shop — GET /v1/shop/<shopId> — получить карточку магазина.
    // Параметры: shopId — идентификатор магазина.
    // Возвращает: JSONObject с данными магазина.
    public JSONObject shop(int shopId) throws Exception {
        return request("GET", "/v1/shop/" + shopId, null, token);
    }

    // updateShop — PUT /v1/shop/<shopId> — обновить данные магазина.
    // Параметры: shopId — id магазина; title — название; description — описание;
    // paymentInfo — платёжные данные; imageURL — URL картинки (опц.).
    public void updateShop(int shopId, String title, String description, String paymentInfo, String imageURL) throws Exception {
        JSONObject body = new JSONObject();
        body.put("title", title);
        body.put("description", description);
        body.put("payment_info", paymentInfo);
        if (imageURL != null) body.put("image_url", imageURL);
        request("PUT", "/v1/shop/" + shopId, body, token);
    }

    // deleteShop — DELETE /v1/shop/<shopId> — удалить магазин.
    // Параметры: shopId — идентификатор магазина.
    public void deleteShop(int shopId) throws Exception {
        request("DELETE", "/v1/shop/" + shopId, null, token);
    }

    // searchShops — GET /v1/shops/search?q=<q> — поиск магазинов по запросу.
    // Параметры: q — поисковый запрос (URL-encoded).
    // Возвращает: JSONArray с магазинами (поле "shops" ответа).
    public JSONArray searchShops(String q) throws Exception {
        return request("GET", "/v1/shops/search?q=" + java.net.URLEncoder.encode(q, "UTF-8"), null, token)
                .optJSONArray("shops");
    }

    // createShop — POST /v1/shop — создать магазин, привязанный к Telegram-чату.
    // Параметры: tgChatId — id Telegram-чата; title — название; description — описание (опц.);
    // paymentInfo — платёжные данные (опц.); imageURL — URL картинки (опц.).
    // Возвращает: JSONObject созданного магазина.
    public JSONObject createShop(long tgChatId, String title, String description, String paymentInfo, String imageURL) throws Exception {
        JSONObject body = new JSONObject();
        body.put("tg_chat_id", tgChatId);
        body.put("title", title);
        if (description != null) body.put("description", description);
        if (paymentInfo != null) body.put("payment_info", paymentInfo);
        if (imageURL != null) body.put("image_url", imageURL);
        return request("POST", "/v1/shop", body, token);
    }

    // createShopAuto — POST /v1/shop — создать магазин в авто-режиме (без явного tg_chat_id).
    // Параметры: title — название; description — описание (опц.); paymentInfo — платёжные данные (опц.);
    // imageURL — URL картинки (опц.). Возвращает: JSONObject созданного магазина.
    public JSONObject createShopAuto(String title, String description, String paymentInfo, String imageURL) throws Exception {
        JSONObject body = new JSONObject();
        body.put("title", title);
        if (description != null) body.put("description", description);
        if (paymentInfo != null) body.put("payment_info", paymentInfo);
        if (imageURL != null) body.put("image_url", imageURL);
        return request("POST", "/v1/shop", body, token);
    }

    // subscribe — POST /v1/shops/<shopId>/subscribe — подписаться на магазин.
    // Параметры: shopId — идентификатор магазина.
    public void subscribe(int shopId) throws Exception {
        request("POST", "/v1/shops/" + shopId + "/subscribe", new JSONObject(), token);
    }

    // unsubscribe — DELETE /v1/shops/<shopId>/subscribe — отписаться от магазина.
    // Параметры: shopId — идентификатор магазина.
    public void unsubscribe(int shopId) throws Exception {
        request("DELETE", "/v1/shops/" + shopId + "/subscribe", null, token);
    }

    // mySubscriptions — GET /v1/me/subscriptions — список id каналов магазинов, на которые подписан текущий пользователь.
    // Возвращает: JSONArray с id каналов (поле "channel_ids" ответа).
    public JSONArray mySubscriptions() throws Exception {
        JSONObject resp = request("GET", "/v1/me/subscriptions", null, token);
        return resp.optJSONArray("channel_ids");
    }

    // product — GET /v1/product/<productId> — получить карточку товара.
    // Параметры: productId — идентификатор товара.
    // Возвращает: JSONObject с данными товара.
    public JSONObject product(int productId) throws Exception {
        return request("GET", "/v1/product/" + productId, null, token);
    }

    // updateProduct — PUT /v1/product/<productId> — обновить данные товара.
    // Параметры: productId — id товара; title — название; description — описание;
    // price — цена; currency — валюта; category — категория; imageURL — URL картинки (опц.).
    public void updateProduct(int productId, String title, String description, double price, String currency, String category, String imageURL) throws Exception {
        JSONObject body = new JSONObject();
        body.put("title", title);
        body.put("description", description);
        body.put("price", price);
        body.put("currency", currency);
        body.put("category", category);
        if (imageURL != null) body.put("image_url", imageURL);
        request("PUT", "/v1/product/" + productId, body, token);
    }

    // deleteProduct — DELETE /v1/product/<productId> — удалить товар.
    // Параметры: productId — идентификатор товара.
    public void deleteProduct(int productId) throws Exception {
        request("DELETE", "/v1/product/" + productId, null, token);
    }

    // review — POST /v1/product/<productId>/review — оставить отзыв о товаре.
    // Параметры: productId — товар; rating — оценка 1-5; text — текст отзыва (опц.).
    public void review(int productId, int rating, String text) throws Exception {
        JSONObject body = new JSONObject();
        body.put("rating", rating);
        if (text != null) body.put("text", text);
        request("POST", "/v1/product/" + productId + "/review", body, token);
    }

    // reviews — GET /v1/product/<productId>/reviews — получить все отзывы о товаре.
    // Возвращает: JSONObject с массивом отзывов.
    public JSONObject reviews(int productId) throws Exception {
        return request("GET", "/v1/product/" + productId + "/reviews", null, token);
    }

    // productView — POST /v1/product/<productId>/view — зафиксировать просмотр товара (счётчик).
    // Параметры: productId — идентификатор товара.
    public void productView(int productId) throws Exception {
        request("POST", "/v1/product/" + productId + "/view", new JSONObject(), token);
    }

    // --- Заказы (этап 7) ---

    // createOrder — POST /v1/order — создать новый заказ.
    // Параметры: productId — товар; quantity — количество; price — цена;
    // contact — контактные данные покупателя.
    // Возвращает: JSONObject созданного заказа.
    public JSONObject createOrder(int productId, int quantity, double price, String contact) throws Exception {
        JSONObject body = new JSONObject();
        body.put("product_id", productId);
        body.put("quantity", quantity);
        body.put("price_amount", price);
        body.put("contact_details", contact);
        return request("POST", "/v1/order", body, token);
    }

    // order — GET /v1/order/<orderId> — получить карточку заказа.
    // Параметры: orderId — идентификатор заказа. Возвращает: JSONObject с данными заказа.
    public JSONObject order(int orderId) throws Exception {
        return request("GET", "/v1/order/" + orderId, null, token);
    }

    // orderChat — GET /v1/order/<orderId>/chat — получить чат (переписку) по заказу.
    // Параметры: orderId — идентификатор заказа. Возвращает: JSONObject с данными чата.
    public JSONObject orderChat(int orderId) throws Exception {
        return request("GET", "/v1/order/" + orderId + "/chat", null, token);
    }

    // payOrder — POST /v1/order/<orderId>/pay — пометить заказ как оплаченный.
    // Параметры: orderId — идентификатор заказа.
    public JSONObject payOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/pay", new JSONObject(), token);
    }

    // cancelOrder — POST /v1/order/<orderId>/cancel — отменить заказ.
    // Параметры: orderId — идентификатор заказа.
    public JSONObject cancelOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/cancel", new JSONObject(), token);
    }

    // confirmOrder — POST /v1/order/<orderId>/confirm — подтвердить выполнение заказа продавцом.
    // Параметры: orderId — идентификатор заказа.
    public JSONObject confirmOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/confirm", new JSONObject(), token);
    }

    // myOrders — GET /v1/orders/me — список заказов текущего пользователя (покупателя).
    // Возвращает: JSONArray с заказами (поле "orders" ответа).
    public JSONArray myOrders() throws Exception {
        JSONObject resp = request("GET", "/v1/orders/me", null, token);
        return resp.optJSONArray("orders");
    }

    // sellerOrders — GET /v1/orders/seller — список заказов, где текущий пользователь — продавец.
    // Возвращает: JSONArray с заказами (поле "orders" ответа).
    public JSONArray sellerOrders() throws Exception {
        JSONObject resp = request("GET", "/v1/orders/seller", null, token);
        return resp.optJSONArray("orders");
    }

    // myStats — GET /v1/stats/me — статистика по заказам текущего пользователя.
    // Возвращает: JSONObject со статистикой.
    public JSONObject myStats() throws Exception {
        return request("GET", "/v1/stats/me", null, token);
    }

    public JSONObject salesStats() throws Exception {
        return request("GET", "/v1/stats/sales", null, token);
    }

    // --- Админ (этап 3) ---

    public JSONObject adminStats() throws Exception {
        return request("GET", "/v1/admin/stats", null, token);
    }

    public JSONArray adminTop() throws Exception {
        JSONObject resp = request("GET", "/v1/admin/top", null, token);
        return resp.optJSONArray("top");
    }

    public JSONArray adminReports() throws Exception {
        JSONObject resp = request("GET", "/v1/admin/reports", null, token);
        return resp.optJSONArray("reports");
    }

    public void adminBanVideo(int videoId) throws Exception {
        request("POST", "/v1/admin/videos/" + videoId + "/ban", new JSONObject(), token);
    }

    public void adminUnbanVideo(int videoId) throws Exception {
        request("POST", "/v1/admin/videos/" + videoId + "/unban", new JSONObject(), token);
    }

    public void adminDeleteVideo(int videoId) throws Exception {
        request("DELETE", "/v1/admin/videos/" + videoId, null, token);
    }

    public void adminDeleteComment(int commentId) throws Exception {
        request("DELETE", "/v1/admin/comments/" + commentId, null, token);
    }

    public JSONArray adminComments() throws Exception {
        return request("GET", "/v1/admin/comments", null, token).optJSONArray("comments");
    }

    public void adminBanUser(int userId) throws Exception {
        request("POST", "/v1/admin/users/" + userId + "/ban", new JSONObject(), token);
    }

    public void adminUnbanUser(int userId) throws Exception {
        request("POST", "/v1/admin/users/" + userId + "/unban", new JSONObject(), token);
    }

    public void adminSuspendShop(int shopId) throws Exception {
        request("POST", "/v1/admin/shops/" + shopId + "/suspend", new JSONObject(), token);
    }

    public void adminReportStatus(int reportId, String status) throws Exception {
        JSONObject body = new JSONObject();
        body.put("status", status);
        request("POST", "/v1/admin/reports/" + reportId + "/status", body, token);
    }

    public JSONArray adminFilterWords() throws Exception {
        JSONObject resp = request("GET", "/v1/admin/filter-words", null, token);
        return resp.optJSONArray("words");
    }

    public void adminFilterWordAdd(String word) throws Exception {
        JSONObject body = new JSONObject();
        body.put("word", word);
        request("POST", "/v1/admin/filter-word", body, token);
    }

    public void adminFilterWordRemove(String word) throws Exception {
        request("DELETE", "/v1/admin/filter-word/" + word, null, token);
    }

    private JSONObject request(String method, String path, JSONObject body, String bearer) throws Exception {
        URL url = new URL(apiUrl + path);
        HttpURLConnection conn = (HttpURLConnection) url.openConnection();
        conn.setRequestMethod(method);
        conn.setConnectTimeout(15000);
        conn.setReadTimeout(15000);
        if (bearer != null) {
            conn.setRequestProperty("Authorization", "Bearer " + bearer);
        }
        conn.setRequestProperty("Content-Type", "application/json");
        conn.setDoInput(true);
        if (body != null) {
            conn.setDoOutput(true);
            byte[] data = body.toString().getBytes(StandardCharsets.UTF_8);
            conn.setFixedLengthStreamingMode(data.length);
            try (OutputStream os = conn.getOutputStream()) {
                os.write(data);
            }
        }
        int code = conn.getResponseCode();
        InputStream in = code >= 400 ? conn.getErrorStream() : conn.getInputStream();
        StringBuilder sb = new StringBuilder();
        if (in != null) {
            try (BufferedReader r = new BufferedReader(new InputStreamReader(in, StandardCharsets.UTF_8))) {
                String line;
                while ((line = r.readLine()) != null) {
                    sb.append(line);
                }
            }
        }
        conn.disconnect();
        if (code < 200 || code >= 300) {
            throw new Exception("http " + code + ": " + sb);
        }
        if (sb.length() == 0) {
            return new JSONObject();
        }
        return new JSONObject(sb.toString());
    }

    // --- Видео (одиночное + статистика) ---

    public JSONObject video(int videoId) throws Exception {
        return request("GET", "/v1/videos/" + videoId, null, token);
    }

    public JSONObject videoStats(int videoId) throws Exception {
        return request("GET", "/v1/videos/" + videoId + "/stats", null, token);
    }

    public String join() throws Exception {
        JSONObject resp = request("POST", "/v1/join", new JSONObject(), token);
        return resp.optString("invite_link");
    }

    public JSONObject createProduct(long shopId, String title, String description,
                                    double price, String currency, String category, String imageURL) throws Exception {
        JSONObject body = new JSONObject();
        body.put("shop_id", shopId);
        body.put("title", title);
        body.put("description", description);
        body.put("price_amount", price);
        body.put("price_currency", currency);
        if (category != null) {
            body.put("category", category);
        }
        if (imageURL != null) {
            body.put("image_url", imageURL);
        }
        return request("POST", "/v1/product", body, token);
    }

    public String uploadImage(byte[] data) throws Exception {
        String boundary = "----media" + System.currentTimeMillis();
        java.net.HttpURLConnection conn = (java.net.HttpURLConnection) new java.net.URL(apiUrl + "/v1/upload").openConnection();
        conn.setRequestMethod("POST");
        conn.setDoOutput(true);
        conn.setUseCaches(false);
        conn.setRequestProperty("Authorization", "Bearer " + token);
        conn.setRequestProperty("Content-Type", "multipart/form-data; boundary=" + boundary);
        java.io.OutputStream os = conn.getOutputStream();
        os.write(("--" + boundary + "\r\n" +
                "Content-Disposition: form-data; name=\"file\"; filename=\"img.jpg\"\r\n" +
                "Content-Type: image/jpeg\r\n\r\n").getBytes(StandardCharsets.UTF_8));
        os.write(data);
        os.write(("\r\n--" + boundary + "--\r\n").getBytes(StandardCharsets.UTF_8));
        os.flush();
        os.close();
        int code = conn.getResponseCode();
        java.io.InputStream in = code >= 400 ? conn.getErrorStream() : conn.getInputStream();
        StringBuilder sb = new StringBuilder();
        if (in != null) {
            try (java.io.BufferedReader r = new java.io.BufferedReader(new java.io.InputStreamReader(in, StandardCharsets.UTF_8))) {
                String line;
                while ((line = r.readLine()) != null) {
                    sb.append(line);
                }
            }
        }
        conn.disconnect();
        if (code < 200 || code >= 300) {
            throw new Exception("http " + code + ": " + sb);
        }
        return new JSONObject(sb.toString()).optString("url", "");
    }
}