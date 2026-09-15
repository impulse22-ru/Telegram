package org.telegram.ui;

import android.app.Activity;
import android.os.Bundle;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.TextView;

import androidx.annotation.NonNull;
import androidx.recyclerview.widget.LinearLayoutManager;
import androidx.recyclerview.widget.RecyclerView;

import org.json.JSONArray;
import org.json.JSONObject;
import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.R;
import org.telegram.messenger.UserConfig;
import org.telegram.messenger.Utilities;
import org.telegram.ui.ActionBar.AlertDialog;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.RadialProgressView;

import java.util.ArrayList;

// Админ-панель: статистика, топ видео, репорты (этап 3).
public class MediaAdminActivity extends Activity {

    private final int currentAccount = UserConfig.selectedAccount;
    private boolean destroyed;
    private RecyclerView list;
    private TextView statsView;
    private AdminAdapter adapter;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        setTheme(R.style.Theme_TMessages);
        super.onCreate(savedInstanceState);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Theme.getColor(Theme.key_windowBackgroundWhite));

        statsView = new TextView(this);
        statsView.setTextSize(14f);
        statsView.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        statsView.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));
        statsView.setOnClickListener(v -> manageFilterWords());
        root.addView(statsView, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT));

        list = new RecyclerView(this);
        list.setLayoutManager(new LinearLayoutManager(this));
        adapter = new AdminAdapter();
        list.setAdapter(adapter);
        root.addView(list, LayoutHelper.createFrameMarginPx(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT, Gravity.TOP, 0, AndroidUtilities.dp(120), 0, 0));

        final RadialProgressView loading = new RadialProgressView(this);
        root.addView(loading, LayoutHelper.createFrame(48, 48, Gravity.CENTER));
        setContentView(root);

        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONObject stats = MediaFeedServerApi.getInstance().adminStats();
                JSONArray top = MediaFeedServerApi.getInstance().adminTop();
                JSONArray reports = MediaFeedServerApi.getInstance().adminReports();
                JSONArray shops = MediaFeedServerApi.getInstance().shops();
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed) {
                        return;
                    }
                    statsView.setText(describe(stats));
                    adapter.set(top, reports, shops);
                    loading.setVisibility(View.GONE);
                });
            } catch (Exception e) {
                AndroidUtilities.runOnUIThread(() -> {
                    loading.setVisibility(View.GONE);
                    if (!destroyed) {
                        statsView.setText(e.getMessage());
                    }
                });
            }
        });
    }

    private String describe(JSONObject s) {
        StringBuilder sb = new StringBuilder();
        sb.append("👑 Admin  (tap = filter words)\n");
        sb.append("users: ").append(s.optLong("users")).append("\n");
        sb.append("videos: ").append(s.optLong("videos")).append("\n");
        sb.append("shops: ").append(s.optLong("shops")).append("\n");
        sb.append("orders: ").append(s.optLong("orders")).append("\n");
        sb.append("reports: ").append(s.optLong("reports"));
        return sb.toString();
    }

    private void manageFilterWords() {
        final AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

        final TextView wordsList = new TextView(this);
        wordsList.setTextSize(14f);
        wordsList.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        content.addView(wordsList, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        final EditText newWord = new EditText(this);
        newWord.setHint("new word");
        newWord.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        newWord.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        content.addView(newWord, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        builder.setTitle("Фильтр-слова");
        builder.setView(content);
        builder.setPositiveButton("Добавить", (dialog, which) -> {
            String word = newWord.getText().toString().trim();
            if (word.isEmpty()) return;
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().adminFilterWordAdd(word);
                } catch (Exception ignore) {
                }
                refresh();
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);
        builder.show();

        Utilities.stageQueue.postRunnable(() -> {
            final StringBuilder sb = new StringBuilder();
            try {
                JSONArray words = MediaFeedServerApi.getInstance().adminFilterWords();
                for (int i = 0; i < words.length(); i++) {
                    String w = words.optString(i);
                    if (w.length() == 0 && !words.isNull(i)) continue;
                    sb.append("• ").append(w).append("\n");
                }
            } catch (Exception ignore) {
            }
            final String text = sb.length() == 0 ? "(empty)" : sb.toString();
            AndroidUtilities.runOnUIThread(() -> wordsList.setText(text));
        });
    }

    private void refresh() {
        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONObject stats = MediaFeedServerApi.getInstance().adminStats();
                JSONArray top = MediaFeedServerApi.getInstance().adminTop();
                JSONArray reports = MediaFeedServerApi.getInstance().adminReports();
                JSONArray shops = MediaFeedServerApi.getInstance().shops();
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed) return;
                    statsView.setText(describe(stats));
                    adapter.set(top, reports, shops);
                });
            } catch (Exception ignore) {
            }
        });
    }

    private class AdminAdapter extends RecyclerView.Adapter<AdminHolder> {

        private final ArrayList<AdminItem> items = new ArrayList<>();

        void set(JSONArray top, JSONArray reports, JSONArray shops) {
            items.clear();
            if (top != null) {
                for (int i = 0; i < top.length(); i++) {
                    JSONObject v = top.optJSONObject(i);
                    if (v == null) continue;
                    AdminItem it = new AdminItem();
                    it.videoId = v.optLong("id");
                    it.title = (v.optString("title", "").isEmpty() ? "video#" + it.videoId : v.optString("title"))
                            + "\n👁 " + v.optLong("views") + " ❤ " + v.optLong("likes")
                            + " 💬 " + v.optLong("comments");
                    it.isTop = true;
                    items.add(it);
                }
            }
            if (reports != null) {
                for (int i = 0; i < reports.length(); i++) {
                    JSONObject r = reports.optJSONObject(i);
                    if (r == null) continue;
                    AdminItem it = new AdminItem();
                    it.videoId = r.optLong("video_id");
                    it.reportId = r.optLong("id");
                    it.title = "⚠ " + r.optString("reason", "report") + " (video " + it.videoId + ")\n"
                            + "by " + r.optLong("user_id") + " @ " + r.optString("created_at");
                    items.add(it);
                }
            }
            if (shops != null) {
                for (int i = 0; i < shops.length(); i++) {
                    JSONObject s = shops.optJSONObject(i);
                    if (s == null) continue;
                    AdminItem it = new AdminItem();
                    it.shopId = s.optLong("id");
                    it.title = "🏬 " + s.optString("title") + "\n" + s.optString("payment_info")
                            + " (" + s.optString("status") + ")";
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        @NonNull
        @Override
        public AdminHolder onCreateViewHolder(@NonNull ViewGroup parent, int viewType) {
            return new AdminHolder(parent);
        }

        @Override
        public void onBindViewHolder(@NonNull AdminHolder holder, int position) {
            holder.bind(items.get(position));
        }

        @Override
        public int getItemCount() {
            return items.size();
        }
    }

    private class AdminItem {
        long videoId;
        long reportId;
        long shopId;
        boolean isTop;
        String title;
    }

    private class AdminHolder extends RecyclerView.ViewHolder {

        private final TextView text;
        private AdminItem item;

        AdminHolder(ViewGroup parent) {
            super(new LinearLayout(parent.getContext()));
            LinearLayout row = (LinearLayout) itemView;
            row.setOrientation(LinearLayout.VERTICAL);
            row.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(10), AndroidUtilities.dp(12), AndroidUtilities.dp(10));
            row.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            text = new TextView(row.getContext());
            text.setTextSize(14f);
            text.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            row.addView(text, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            row.setOnClickListener(v -> onRowClicked());
        }

        private void onRowClicked() {
            if (item == null) {
                return;
            }
            if (item.shopId != 0 && item.videoId == 0) {
                final int shopId = (int) item.shopId;
                final AlertDialog.Builder sb = new AlertDialog.Builder(MediaAdminActivity.this);
                sb.setTitle("Shop #" + shopId);
                sb.setMessage(item.title);
                sb.setPositiveButton("⛔ Suspend", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminSuspendShop(shopId);
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                sb.setNegativeButton(getString(R.string.Cancel), null);
                sb.show();
                return;
            }
            if (item.videoId == 0) {
                return;
            }
            final int videoId = (int) item.videoId;
            final boolean isReport = item.reportId != 0;
            final int reportId = (int) item.reportId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaAdminActivity.this);
            builder.setTitle(isReport ? "Report #" + reportId : "Video #" + videoId);
            builder.setMessage(item.title);
            if (isReport) {
                builder.setPositiveButton("✓ reviewed", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminReportStatus(reportId, "reviewed");
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                builder.setNeutralButton("✗ dismissed", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminReportStatus(reportId, "dismissed");
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                builder.setNegativeButton(getString(R.string.Cancel), null);
            } else if (item.isTop) {
                builder.setPositiveButton("🚫 Ban", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminBanVideo(videoId);
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                builder.setNeutralButton("↩ Unban", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminUnbanVideo(videoId);
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                builder.setNegativeButton("🗑 Delete", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminDeleteVideo(videoId);
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
            } else {
                builder.setPositiveButton("🚫 Ban", (dialog, which) ->
                        Utilities.stageQueue.postRunnable(() -> {
                            try {
                                MediaFeedServerApi.getInstance().adminBanVideo(videoId);
                            } catch (Exception ignore) {
                            }
                            refresh();
                        }));
                builder.setNegativeButton(getString(R.string.Cancel), null);
            }
            builder.show();
        }

        void bind(AdminItem it) {
            item = it;
            text.setText(it.title);
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        destroyed = true;
    }
}