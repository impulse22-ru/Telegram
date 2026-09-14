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

public class MediaFeedServerApi {

    public static final String PREF_NAME = "media_feed_config";
    public static final String PREF_API_URL = "api_url";
    public static final String PREF_RELAY_URL = "relay_url";
    public static final String PREF_TOKEN = "token";

    private static final String DEFAULT_API_URL = "http://10.0.2.2:8080";
    private static final String DEFAULT_RELAY_URL = "http://10.0.2.2:8082";

    private static MediaFeedServerApi instance;

    private String apiUrl;
    private String relayUrl;
    private String token;

    public static MediaFeedServerApi getInstance() {
        if (instance == null) {
            instance = new MediaFeedServerApi();
        }
        return instance;
    }

    private MediaFeedServerApi() {
        SharedPreferences prefs = prefs();
        apiUrl = prefs.getString(PREF_API_URL, DEFAULT_API_URL);
        relayUrl = prefs.getString(PREF_RELAY_URL, DEFAULT_RELAY_URL);
        token = prefs.getString(PREF_TOKEN, null);
    }

    private static SharedPreferences prefs() {
        return ApplicationLoader.applicationContext.getSharedPreferences(PREF_NAME, Context.MODE_PRIVATE);
    }

    public String getApiUrl() {
        return apiUrl;
    }

    public String getRelayUrl() {
        return relayUrl;
    }

    public void setUrls(String apiUrl, String relayUrl) {
        this.apiUrl = apiUrl;
        this.relayUrl = relayUrl;
        prefs().edit().putString(PREF_API_URL, apiUrl).putString(PREF_RELAY_URL, relayUrl).apply();
    }

    public String getToken() {
        return token;
    }

    public String streamUrl(long videoId) {
        return relayUrl + "/media/stream/" + videoId;
    }

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

    public JSONArray feed() throws Exception {
        return feed(0, 20);
    }

    public JSONArray feed(long offset, long limit) throws Exception {
        JSONObject resp = request("GET", "/v1/feed?offset=" + offset + "&limit=" + limit, null, token);
        return resp.optJSONArray("videos");
    }

    public boolean hasMore(long offset, long limit) throws Exception {
        JSONObject resp = request("GET", "/v1/feed?offset=" + offset + "&limit=" + limit, null, token);
        return resp.optBoolean("has_more", false);
    }

    public void like(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/like", new JSONObject(), token);
    }

    public void unlike(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/unlike", new JSONObject(), token);
    }

    public void comment(long videoId, String text) throws Exception {
        JSONObject body = new JSONObject();
        body.put("text", text);
        request("POST", "/v1/videos/" + videoId + "/comment", body, token);
    }

    public JSONArray comments(long videoId) throws Exception {
        JSONObject resp = request("GET", "/v1/videos/" + videoId + "/comments", null, token);
        return resp.optJSONArray("comments");
    }

    public void report(long videoId, String reason) throws Exception {
        JSONObject body = new JSONObject();
        body.put("reason", reason);
        request("POST", "/v1/videos/" + videoId + "/report", body, token);
    }

    public void view(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/view", new JSONObject(), token);
    }

    // --- Магазины и товары (этап 6) ---

    public JSONArray catalog() throws Exception {
        JSONObject resp = request("GET", "/v1/catalog", null, token);
        return resp.optJSONArray("items");
    }

    public JSONArray shops() throws Exception {
        JSONObject resp = request("GET", "/v1/shops", null, token);
        return resp.optJSONArray("shops");
    }

    public JSONArray myShops() throws Exception {
        JSONObject resp = request("GET", "/v1/shops/me", null, token);
        return resp.optJSONArray("shops");
    }

    public JSONObject shop(int shopId) throws Exception {
        return request("GET", "/v1/shop/" + shopId, null, token);
    }

    public JSONObject createShop(long tgChatId, String title, String description, String paymentInfo) throws Exception {
        JSONObject body = new JSONObject();
        body.put("tg_chat_id", tgChatId);
        body.put("title", title);
        if (description != null) body.put("description", description);
        if (paymentInfo != null) body.put("payment_info", paymentInfo);
        return request("POST", "/v1/shop", body, token);
    }

    public JSONObject createShopAuto(String title, String description, String paymentInfo) throws Exception {
        JSONObject body = new JSONObject();
        body.put("title", title);
        if (description != null) body.put("description", description);
        if (paymentInfo != null) body.put("payment_info", paymentInfo);
        return request("POST", "/v1/shop", body, token);
    }

    public void subscribe(int shopId) throws Exception {
        request("POST", "/v1/shops/" + shopId + "/subscribe", new JSONObject(), token);
    }

    public void unsubscribe(int shopId) throws Exception {
        request("DELETE", "/v1/shops/" + shopId + "/subscribe", null, token);
    }

    public JSONArray mySubscriptions() throws Exception {
        JSONObject resp = request("GET", "/v1/me/subscriptions", null, token);
        return resp.optJSONArray("channel_ids");
    }

    public JSONObject product(int productId) throws Exception {
        return request("GET", "/v1/product/" + productId, null, token);
    }

    public void productView(int productId) throws Exception {
        request("POST", "/v1/product/" + productId + "/view", new JSONObject(), token);
    }

    // --- Заказы (этап 7) ---

    public JSONObject createOrder(int productId, int quantity, double price, String contact) throws Exception {
        JSONObject body = new JSONObject();
        body.put("product_id", productId);
        body.put("quantity", quantity);
        body.put("price_amount", price);
        body.put("contact_details", contact);
        return request("POST", "/v1/order", body, token);
    }

    public JSONObject order(int orderId) throws Exception {
        return request("GET", "/v1/order/" + orderId, null, token);
    }

    public JSONObject orderChat(int orderId) throws Exception {
        return request("GET", "/v1/order/" + orderId + "/chat", null, token);
    }

    public JSONObject payOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/pay", new JSONObject(), token);
    }

    public JSONObject cancelOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/cancel", new JSONObject(), token);
    }

    public JSONObject confirmOrder(int orderId) throws Exception {
        return request("POST", "/v1/order/" + orderId + "/confirm", new JSONObject(), token);
    }

    public JSONArray myOrders() throws Exception {
        JSONObject resp = request("GET", "/v1/orders/me", null, token);
        return resp.optJSONArray("orders");
    }

    public JSONArray sellerOrders() throws Exception {
        JSONObject resp = request("GET", "/v1/orders/seller", null, token);
        return resp.optJSONArray("orders");
    }

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
                                    double price, String currency, String category) throws Exception {
        JSONObject body = new JSONObject();
        body.put("shop_id", shopId);
        body.put("title", title);
        body.put("description", description);
        body.put("price_amount", price);
        body.put("price_currency", currency);
        if (category != null) {
            body.put("category", category);
        }
        return request("POST", "/v1/product", body, token);
    }
}