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
        JSONObject resp = request("GET", "/v1/feed", null, token);
        return resp.optJSONArray("videos");
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

    public void report(long videoId, String reason) throws Exception {
        JSONObject body = new JSONObject();
        body.put("reason", reason);
        request("POST", "/v1/videos/" + videoId + "/report", body, token);
    }

    public void view(long videoId) throws Exception {
        request("POST", "/v1/videos/" + videoId + "/view", new JSONObject(), token);
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
}