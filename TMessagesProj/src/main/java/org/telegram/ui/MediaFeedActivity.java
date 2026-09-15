package org.telegram.ui;

import android.app.Activity;
import android.content.Intent;
import android.content.res.Configuration;
import android.net.Uri;
import android.os.Bundle;
import android.view.GestureDetector;
import android.view.Gravity;
import android.view.MotionEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.ImageView;
import android.widget.LinearLayout;
import android.widget.TextView;

import androidx.annotation.NonNull;
import androidx.recyclerview.widget.LinearLayoutManager;
import androidx.recyclerview.widget.PagerSnapHelper;
import androidx.recyclerview.widget.RecyclerView;

import org.json.JSONArray;
import org.json.JSONObject;
import org.telegram.messenger.AndroidUtilities;
import org.telegram.messenger.R;
import org.telegram.messenger.UserConfig;
import org.telegram.messenger.Utilities;
import org.telegram.tgnet.TLRPC;
import org.telegram.ui.ActionBar.AlertDialog;
import org.telegram.ui.ActionBar.Theme;
import org.telegram.ui.Components.LayoutHelper;
import org.telegram.ui.Components.RadialProgressView;
import org.telegram.ui.Components.VideoPlayer;

public class MediaFeedActivity extends Activity {

    private FeedAdapter adapter;
    private final int currentAccount = UserConfig.selectedAccount;
    private boolean destroyed;
    private long feedOffset;
    private boolean feedLoading;
    private boolean searchMode;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        setTheme(R.style.Theme_TMessages);
        super.onCreate(savedInstanceState);
        getWindow().getDecorView().setBackgroundColor(0xff000000);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(0xff000000);

        RecyclerView list = new RecyclerView(this);
        list.setLayoutManager(new LinearLayoutManager(this, LinearLayoutManager.VERTICAL, false));
        list.setClipToPadding(false);
        PagerSnapHelper pagerSnapHelper = new PagerSnapHelper();
        pagerSnapHelper.attachToRecyclerView(list);

        adapter = new FeedAdapter();
        list.setAdapter(adapter);

        RadialProgressView loading = new RadialProgressView(this);

        list.addOnScrollListener(new RecyclerView.OnScrollListener() {
            @Override
            public void onScrollStateChanged(@NonNull RecyclerView rv, int state) {
                if (state == RecyclerView.SCROLL_STATE_IDLE) {
                    updateActivePosition(rv);
                }
            }

            @Override
            public void onScrolled(@NonNull RecyclerView rv, int dx, int dy) {
                RecyclerView.LayoutManager lm = rv.getLayoutManager();
                if (lm instanceof LinearLayoutManager) {
                    LinearLayoutManager llm = (LinearLayoutManager) lm;
                    int last = llm.findLastVisibleItemPosition();
                    if (last >= adapter.getItemCount() - 3 && !feedLoading) {
                        loadMore();
                    }
                }
            }
        });

        root.addView(list, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));
        root.addView(loading, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));

        TextView searchBtn = new TextView(this);
        searchBtn.setText("🔍");
        searchBtn.setTextSize(20f);
        searchBtn.setTextColor(0xffffffff);
        searchBtn.setBackgroundResource(R.drawable.bg_media_feed_btn);
        searchBtn.setGravity(Gravity.CENTER);
        searchBtn.setPadding(AndroidUtilities.dp(4), AndroidUtilities.dp(4), AndroidUtilities.dp(4), AndroidUtilities.dp(4));
        searchBtn.setOnClickListener(v -> openSearchDialog());
        root.addView(searchBtn, LayoutHelper.createFrame(AndroidUtilities.dp(44), AndroidUtilities.dp(44), Gravity.TOP | Gravity.END, 0, AndroidUtilities.dp(16), AndroidUtilities.dp(16), 0));

        setContentView(root);

        loadFeed(() -> AndroidUtilities.runOnUIThread(() -> loading.setVisibility(View.GONE)));
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        if (adapter != null) {
            adapter.notifyDataSetChanged();
        }
    }

    private void updateActivePosition(RecyclerView list) {
        RecyclerView.LayoutManager lm = list.getLayoutManager();
        if (!(lm instanceof LinearLayoutManager)) {
            return;
        }
        LinearLayoutManager llm = (LinearLayoutManager) lm;
        int first = llm.findFirstVisibleItemPosition();
        int last = llm.findLastVisibleItemPosition();
        if (first == RecyclerView.NO_POSITION) {
            return;
        }
        int center = list.getHeight() / 2;
        int best = first;
        int bestDelta = Integer.MAX_VALUE;
        for (int i = first; i <= last && i >= 0; i++) {
            View v = llm.findViewByPosition(i);
            if (v == null) {
                continue;
            }
            int delta = Math.abs((v.getTop() + v.getHeight() / 2) - center);
            if (delta < bestDelta) {
                bestDelta = delta;
                best = i;
            }
        }
        if (adapter != null) {
            adapter.setActivePosition(best);
        }
    }

    private void loadFeed(Runnable onDone) {
        feedOffset = 0;
        searchMode = false;
        loadPage(true, onDone);
    }

    private void loadMore() {
        if (searchMode) {
            return;
        }
        loadPage(false, null);
    }

    private void openSearchDialog() {
        final AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final EditText editText = new EditText(this);
        editText.setHint(getString(R.string.MediaFeedSearchHint));
        editText.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        editText.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        editText.setSingleLine(false);
        FrameLayout frame = new FrameLayout(this);
        frame.addView(editText, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.CENTER, 16, 16, 16, 16));
        builder.setTitle(getString(R.string.MediaFeedSearch));
        builder.setView(frame);
        builder.setPositiveButton(getString(R.string.Search), (dialog, which) -> {
            final String q = editText.getText().toString().trim();
            if (!q.isEmpty()) {
                runSearch(q);
            }
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);
        builder.show();
    }

    private void runSearch(final String query) {
        if (feedLoading) {
            return;
        }
        feedLoading = true;
        searchMode = true;
        Utilities.stageQueue.postRunnable(() -> {
            try {
                MediaFeedServerApi api = MediaFeedServerApi.getInstance();
                JSONArray videos = api.search(query);
                AndroidUtilities.runOnUIThread(() -> {
                    feedLoading = false;
                    if (destroyed) {
                        return;
                    }
                    if (adapter != null) {
                        adapter.setVideos(videos);
                    }
                    feedOffset = 0;
                });
            } catch (Exception ignore) {
                AndroidUtilities.runOnUIThread(() -> {
                    feedLoading = false;
                    AndroidUtilities.shakeView(getWindow().getDecorView());
                });
            }
        });
    }

    private void loadPage(final boolean first, final Runnable onDone) {
        if (feedLoading) {
            return;
        }
        feedLoading = true;
        final long offset = feedOffset;
        Utilities.stageQueue.postRunnable(() -> {
            try {
                MediaFeedServerApi api = MediaFeedServerApi.getInstance();
                UserConfig userConfig = UserConfig.getInstance(currentAccount);
                long userId = userConfig.getClientUserId();
                TLRPC.User user = userConfig.getCurrentUser();
                String phone = user != null ? user.phone : null;
                String name = user != null ? user.first_name : null;
                api.auth(userId, phone, name);
                JSONArray videos = api.feed(offset, 20);
                getSharedPreferences("mediafeed_cache", 0).edit()
                        .putString("feed", videos.toString())
                        .apply();
                AndroidUtilities.runOnUIThread(() -> {
                    feedLoading = false;
                    if (destroyed) {
                        return;
                    }
                    if (adapter != null) {
                        if (first) {
                            adapter.setVideos(videos);
                        } else {
                            adapter.addVideos(videos);
                        }
                    }
                    feedOffset = offset + videos.length();
                    if (onDone != null) {
                        onDone.run();
                    }
                });
            } catch (Exception ignore) {
                if (first) {
                    final String cached = getSharedPreferences("mediafeed_cache", 0).getString("feed", null);
                    if (cached != null) {
                        try {
                            final JSONArray cachedVideos = new JSONArray(cached);
                            AndroidUtilities.runOnUIThread(() -> {
                                feedLoading = false;
                                if (destroyed) {
                                    return;
                                }
                                if (adapter != null) {
                                    adapter.setVideos(cachedVideos);
                                }
                                feedOffset = cachedVideos.length();
                                if (onDone != null) {
                                    onDone.run();
                                }
                            });
                        } catch (Exception e2) {
                            failLoad(first, onDone);
                        }
                        return;
                    }
                }
                failLoad(first, onDone);
            }
        });
    }

    private void failLoad(final boolean first, final Runnable onDone) {
        AndroidUtilities.runOnUIThread(() -> {
            feedLoading = false;
            if (onDone != null) {
                onDone.run();
            }
            if (first) {
                AndroidUtilities.shakeView(getWindow().getDecorView());
            }
        });
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        destroyed = true;
        if (adapter != null) {
            adapter.releaseAll();
        }
    }

    private static class VideoItem {
        long id;
        String fileId;
        String caption;
        String title;
        int width;
        int height;
        long durationMs;
        long likeCount;
        long viewCount;
        long commentCount;
        boolean liked;
    }

    private class FeedAdapter extends RecyclerView.Adapter<FeedHolder> {

        private final java.util.ArrayList<VideoItem> items = new java.util.ArrayList<>();
        private final android.util.SparseArray<FeedHolder> holders = new android.util.SparseArray<>();
        private int active = -1;

        void setVideos(JSONArray videos) {
            items.clear();
            if (videos != null) {
                for (int i = 0; i < videos.length(); i++) {
                    VideoItem v = parseVideo(videos.optJSONObject(i));
                    if (v != null) {
                        items.add(v);
                    }
                }
            }
            notifyDataSetChanged();
            if (active < 0 && !items.isEmpty()) {
                setActivePosition(0);
            }
        }

        void addVideos(JSONArray videos) {
            if (videos != null) {
                for (int i = 0; i < videos.length(); i++) {
                    VideoItem v = parseVideo(videos.optJSONObject(i));
                    if (v != null) {
                        items.add(v);
                    }
                }
            }
            notifyDataSetChanged();
        }

        private VideoItem parseVideo(JSONObject o) {
            if (o == null) {
                return null;
            }
            VideoItem v = new VideoItem();
            v.id = o.optLong("id");
            v.fileId = o.optString("file_id");
            v.caption = o.optString("caption");
            v.title = o.optString("title");
            v.width = o.optInt("width");
            v.height = o.optInt("height");
            v.durationMs = o.optLong("duration_ms");
            return v.id != 0 ? v : null;
        }

        void setActivePosition(int position) {
            int prev = active;
            active = position;
            FeedHolder prevHolder = holders.get(prev);
            if (prevHolder != null && prev != position) {
                prevHolder.pause();
            }
            FeedHolder holder = holders.get(position);
            if (holder != null) {
                holder.play();
            }
        }

        void releaseAll() {
            for (int i = 0; i < holders.size(); i++) {
                FeedHolder h = holders.valueAt(i);
                if (h != null) {
                    h.release();
                }
            }
            holders.clear();
        }

        @NonNull
        @Override
        public FeedHolder onCreateViewHolder(@NonNull ViewGroup parent, int viewType) {
            return new FeedHolder(parent);
        }

        @Override
        public void onBindViewHolder(@NonNull FeedHolder holder, int position) {
            if (position < 0 || position >= items.size()) {
                return;
            }
            holder.bind(items.get(position));
            if (position == active) {
                holder.play();
            } else {
                holder.pause();
            }
        }

        @Override
        public void onViewAttachedToWindow(@NonNull FeedHolder holder) {
            super.onViewAttachedToWindow(holder);
            int pos = holder.getAdapterPosition();
            if (pos != RecyclerView.NO_POSITION) {
                holders.put(pos, holder);
                if (pos == active) {
                    holder.play();
                }
            }
        }

        @Override
        public void onViewDetachedFromWindow(@NonNull FeedHolder holder) {
            super.onViewDetachedFromWindow(holder);
            int pos = holder.getAdapterPosition();
            if (pos != RecyclerView.NO_POSITION) {
                holders.remove(pos);
            }
            holder.pause();
        }

        @Override
        public void onViewRecycled(@NonNull FeedHolder holder) {
            super.onViewRecycled(holder);
            holder.release();
        }

        @Override
        public int getItemCount() {
            return items.size();
        }
    }

    private class FeedHolder extends RecyclerView.ViewHolder {

        private final FrameLayout container;
        private final android.view.TextureView textureView;
        private final RadialProgressView loader;
        private final ImageView likeButton;
        private final TextView likeCount;
        private final ImageView commentButton;
        private final TextView commentCount;
        private final ImageView reportButton;
        private VideoPlayer videoPlayer;
        private VideoItem item;
        private boolean playing;
        private boolean released;

        FeedHolder(ViewGroup parent) {
            super(new FrameLayout(parent.getContext()));
            container = (FrameLayout) itemView;
            container.setBackgroundColor(0xff000000);
            container.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

            textureView = new android.view.TextureView(container.getContext());
            container.addView(textureView, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));

            loader = new RadialProgressView(container.getContext());
            loader.setSize(AndroidUtilities.dp(48));
            container.addView(loader, LayoutHelper.createFrame(48, 48, Gravity.CENTER));

            LinearLayout side = new LinearLayout(container.getContext());
            side.setOrientation(LinearLayout.VERTICAL);

            likeButton = createSideButton(R.drawable.media_like);
            commentButton = createSideButton(R.drawable.menu_comments);
            reportButton = createSideButton(R.drawable.msg_report);

            side.addView(likeButton, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(52)));
            likeCount = createCountLabel();
            side.addView(likeCount, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(20)));
            side.addView(commentButton, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(52)));
            commentCount = createCountLabel();
            side.addView(commentCount, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(20)));
            side.addView(reportButton, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(52)));
            ImageView shareButton = createSideButton(R.drawable.msg_share);
            side.addView(shareButton, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(52)));
            container.addView(side, LayoutHelper.createFrame(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, Gravity.BOTTOM | Gravity.RIGHT, 0, 0, 8, 24));

            final GestureDetector gd = new GestureDetector(container.getContext(), new GestureDetector.SimpleOnGestureListener() {
                @Override
                public boolean onDoubleTap(MotionEvent e) {
                    onLike();
                    return true;
                }
            });
            container.setOnTouchListener((v, event) -> {
                gd.onTouchEvent(event);
                return false;
            });

            likeButton.setOnClickListener(v -> onLike());
            commentButton.setOnClickListener(v -> onComment());
            shareButton.setOnClickListener(v -> onShare());
            reportButton.setOnClickListener(v -> onReport());
        }

        private ImageView createSideButton(int res) {
            ImageView iv = new ImageView(container.getContext());
            iv.setImageResource(res);
            iv.setColorFilter(0xffffffff);
            iv.setScaleType(ImageView.ScaleType.CENTER_INSIDE);
            iv.setPadding(AndroidUtilities.dp(6), AndroidUtilities.dp(6), AndroidUtilities.dp(6), AndroidUtilities.dp(6));
            return iv;
        }

        private TextView createCountLabel() {
            TextView tv = new TextView(container.getContext());
            tv.setTextColor(0xffffffff);
            tv.setTextSize(11f);
            tv.setGravity(Gravity.CENTER);
            return tv;
        }

        void bind(VideoItem item) {
            this.item = item;
            released = false;
            playing = false;
            loader.setVisibility(View.VISIBLE);
            updateLikeButton();
            commentCount.setText(item.commentCount > 0
                    ? String.valueOf(item.commentCount)
                    : "");
        }

        void play() {
            if (item == null || released || playing) {
                return;
            }
            playing = true;
            loadStats();
            reportView();
            if (videoPlayer == null) {
                videoPlayer = new VideoPlayer(false, false);
                videoPlayer.setDelegate(new VideoPlayer.VideoPlayerDelegate() {
                    @Override
                    public void onStateChanged(boolean playWhenReady, int playbackState) {
                    }

                    @Override
                    public void onError(VideoPlayer player, Exception e) {
                    }

                    @Override
                    public void onVideoSizeChanged(int width, int height, int unappliedRotationDegrees, float pixelWidthHeightRatio) {
                    }

                    @Override
                    public void onRenderedFirstFrame() {
                        AndroidUtilities.runOnUIThread(() -> {
                            if (loader != null) {
                                loader.setVisibility(View.GONE);
                            }
                        });
                    }
                });
            }
            videoPlayer.setTextureView(textureView);
            videoPlayer.preparePlayer(Uri.parse(MediaFeedServerApi.getInstance().streamUrl(item.id)), "other");
            videoPlayer.setLooping(true);
            videoPlayer.play();
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().view(item.id);
                } catch (Exception ignore) {
                }
            });
        }

        void pause() {
            playing = false;
            if (videoPlayer != null) {
                videoPlayer.pause();
            }
        }

        void release() {
            released = true;
            playing = false;
            if (videoPlayer != null) {
                videoPlayer.releasePlayer(true);
                videoPlayer = null;
            }
        }

        private void onLike() {
            if (item == null) {
                return;
            }
            final long id = item.id;
            final boolean wasLiked = item.liked;
            item.liked = !wasLiked;
            item.likeCount += wasLiked ? -1 : 1;
            updateLikeButton();
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    if (wasLiked) {
                        MediaFeedServerApi.getInstance().unlike(id);
                    } else {
                        MediaFeedServerApi.getInstance().like(id);
                    }
                } catch (Exception ignore) {
                }
            });
        }

        private void updateLikeButton() {
            if (item == null) {
                return;
            }
            likeButton.setImageResource(item.liked
                    ? R.drawable.media_like_active
                    : R.drawable.media_like);
            likeCount.setText(item.likeCount > 0
                    ? String.valueOf(item.likeCount)
                    : "");
        }

        private void onComment() {
            if (item == null) {
                return;
            }
            openCommentDialog(item.id);
        }

        private void onShare() {
            if (item == null) {
                return;
            }
            final String url = MediaFeedServerApi.getInstance().streamUrl(item.id);
            AndroidUtilities.runOnUIThread(() -> {
                if (released) {
                    return;
                }
                Intent share = new Intent(Intent.ACTION_SEND);
                share.setType("text/plain");
                share.putExtra(Intent.EXTRA_TEXT, url);
                MediaFeedActivity.this.startActivity(Intent.createChooser(share, getString(R.string.ShareLink)));
            });
        }

        private void onReport() {
            if (item == null) {
                return;
            }
            openReportDialog(item.id);
        }

        private void loadStats() {
            final long id = item.id;
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONObject stats = MediaFeedServerApi.getInstance().videoStats((int) id);
                    final long likes = stats.optLong("likes");
                    final long comments = stats.optLong("comments");
                    final long views = stats.optLong("views");
                    AndroidUtilities.runOnUIThread(() -> {
                        if (item == null || item.id != id || released) {
                            return;
                        }
                        item.likeCount = likes;
                        item.commentCount = comments;
                        item.viewCount = views;
                        updateLikeButton();
                        commentCount.setText(comments > 0 ? String.valueOf(comments) : "");
                    });
                } catch (Exception ignore) {
                }
            });
        }

        private void reportView() {
            final long id = item.id;
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().view(id);
                } catch (Exception ignore) {
                }
            });
        }
    }

    private void openCommentDialog(final long videoId) {
        final AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(AndroidUtilities.dp(8), AndroidUtilities.dp(8), AndroidUtilities.dp(8), 0);

        final android.widget.TextView list = new android.widget.TextView(this);
        list.setTextSize(13f);
        list.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        list.setLineSpacing(AndroidUtilities.dp(2), 1f);
        content.addView(list, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f));

        final EditText editText = new EditText(this);
        editText.setHint(getString(R.string.AddCommentHint));
        editText.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        editText.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        content.addView(editText, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        builder.setTitle(getString(R.string.MediaFeedComments));
        builder.setView(content);
        builder.setPositiveButton(getString(R.string.Send), (dialog, which) -> {
            final String text = editText.getText().toString().trim();
            if (text.isEmpty()) {
                return;
            }
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().comment(videoId, text);
                } catch (Exception ignore) {
                }
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);

        Utilities.stageQueue.postRunnable(() -> {
            final java.util.Map<Long, JSONObject> byId = new java.util.HashMap<>();
            final java.util.List<JSONObject> topLevel = new java.util.ArrayList<>();
            try {
                JSONArray comments = MediaFeedServerApi.getInstance().comments(videoId);
                for (int i = 0; i < comments.length(); i++) {
                    JSONObject c = comments.optJSONObject(i);
                    if (c == null) continue;
                    byId.put(c.optLong("id"), c);
                    if (c.optLong("parent_id") == 0) {
                        topLevel.add(c);
                    }
                }
            } catch (Exception ignore) {
            }
            final StringBuilder sb = new StringBuilder();
            for (JSONObject c : topLevel) {
                appendComment(sb, c, byId, 0);
            }
            final String text = sb.length() == 0 ? getString(R.string.MediaFeedNoComments) : sb.toString();
            AndroidUtilities.runOnUIThread(() -> list.setText(text));
        });

        builder.show();
    }

    private void appendComment(StringBuilder sb, JSONObject c, java.util.Map<Long, JSONObject> byId, int depth) {
        try {
            long uid = c.optLong("user_id");
            String text = c.optString("text");
            String time = c.optString("created_at", "");
            String indent = depth > 0 ? "  ".repeat(depth) + "└ " : "";
            sb.append(indent).append("👤 #").append(uid);
            if (time.length() > 5) {
                sb.append(" · ").append(time.substring(11, 16));
            }
            sb.append("\n").append(indent).append(text).append("\n");
            for (java.util.Map.Entry<Long, JSONObject> e : byId.entrySet()) {
                if (e.getValue().optLong("parent_id") == c.optLong("id")) {
                    appendComment(sb, e.getValue(), byId, depth + 1);
                }
            }
        } catch (Exception ignore) {
        }
    }

    private void openReportDialog(final long videoId) {
        final AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final EditText editText = new EditText(this);
        editText.setHint(getString(R.string.ReportReasonHint));
        editText.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        editText.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        FrameLayout frame = new FrameLayout(this);
        frame.addView(editText, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.CENTER, 16, 16, 16, 16));
        builder.setTitle(getString(R.string.MediaFeedReportTitle));
        builder.setView(frame);
        builder.setPositiveButton(getString(R.string.Send), (dialog, which) -> {
            final String reason = editText.getText().toString();
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().report(videoId, reason);
                } catch (Exception ignore) {
                }
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);
        builder.show();
    }
}