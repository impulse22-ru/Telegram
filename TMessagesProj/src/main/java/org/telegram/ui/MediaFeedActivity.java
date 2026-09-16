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

/**
 * Полноэкранная лента видео (вертикальный пейджер).
 * Работает как TikTok / Instagram Reels: один ролик на весь экран,
 * автовоспроизведение активного видео, управление лайками / комментариями /
 * репортами / шером через боковую панель.
 * Пагинация: offset + limit (20 шт.), offline-кэш в SharedPreferences
 * при сетевой ошибке. Поиск по ключевому слову через диалог.
 */
public class MediaFeedActivity extends Activity {

    /* Адаптер RecyclerView, привязывает VideoItem к FeedHolder */
    private FeedAdapter adapter;
    /** ID текущего аккаунта Telegram (для Multi-Account) */
    private final int currentAccount = UserConfig.selectedAccount;
    /** Флаг: Activity уничтожена — все callback'и на UI проверяют его */
    private boolean destroyed;
    /** Текущее смещение пагинации (кол-во уже загруженных элементов) */
    private long feedOffset;
    /** Защита от параллельных запросов: true — запрос уже летит */
    private boolean feedLoading;
    /** Режим поиска: при true автозагрузка при скролле отключается */
    private boolean searchMode;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        setTheme(R.style.Theme_TMessages);
        super.onCreate(savedInstanceState);
        // Экран полностью чёрный — подложка для видео.
        getWindow().getDecorView().setBackgroundColor(0xff000000);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(0xff000000);

        // RecyclerView с вертикальным LinearLayoutManager + PagerSnapHelper.
        // SnapHelper примагничивает список к страницам, чтобы один ролик
        // всегда занимал весь экран.
        RecyclerView list = new RecyclerView(this);
        list.setLayoutManager(new LinearLayoutManager(this, LinearLayoutManager.VERTICAL, false));
        list.setClipToPadding(false);
        PagerSnapHelper pagerSnapHelper = new PagerSnapHelper();
        pagerSnapHelper.attachToRecyclerView(list);

        adapter = new FeedAdapter();
        list.setAdapter(adapter);

        // Спиннер поверх ленты, пока грузится первая страница.
        RadialProgressView loading = new RadialProgressView(this);

        list.addOnScrollListener(new RecyclerView.OnScrollListener() {
            @Override
            public void onScrollStateChanged(@NonNull RecyclerView rv, int state) {
                // Скролл остановился — пересчитываем, какое видео в центре.
                if (state == RecyclerView.SCROLL_STATE_IDLE) {
                    updateActivePosition(rv);
                }
            }

            @Override
            public void onScrolled(@NonNull RecyclerView rv, int dx, int dy) {
                // Доскроллили до последних 3 элементов — догружаем следующую порцию.
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

        // Кнопка поиска в правом верхнем углу.
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

        // Первичная загрузка ленты; по завершении прячем спиннер.
        loadFeed(() -> AndroidUtilities.runOnUIThread(() -> loading.setVisibility(View.GONE)));
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        // При повороте/изменении конфигурации перерисовываем все элементы.
        if (adapter != null) {
            adapter.notifyDataSetChanged();
        }
    }

    /** Определяет, какое видео сейчас ближе всего к центру экрана, и делает его активным.
     *  Цикл по видимым позициям, считается |центр страницы - середина экрана|,
     *  минимум — и есть активная позиция. */
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
            // Расстояние от середины страницы до середины экрана.
            int delta = Math.abs((v.getTop() + v.getHeight() / 2) - center);
            if (delta < bestDelta) {
                bestDelta = delta;
                best = i;
            }
        }
        if (adapter != null) {
            // Прокидываем активную позицию адаптеру: он запустит play/pause.
            adapter.setActivePosition(best);
        }
    }

    /** Полная перезагрузка ленты с нуля (offset = 0, без поиска). */
    private void loadFeed(Runnable onDone) {
        feedOffset = 0;
        searchMode = false;
        loadPage(true, onDone);
    }

    /** Подгрузка следующей страницы при скролле. В режиме поиска отключена. */
    private void loadMore() {
        if (searchMode) {
            return;
        }
        loadPage(false, null);
    }

    /** Диалог ввода поискового запроса. */
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

    /** Поиск видео по запросу: шлём запрос в фоне (stageQueue),
     *  результат — полная замена текущей ленты. */
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
                // Ошибка поиска: сбрасываем флаг и трясём декорацию окна.
                AndroidUtilities.runOnUIThread(() -> {
                    feedLoading = false;
                    AndroidUtilities.shakeView(getWindow().getDecorView());
                });
            }
        });
    }

    /** Загрузка порции видео (offset+limit=20) в фоновом потоке.
     *  При успехе — кэшируем ленту в SharedPreferences, при сетевой ошибке на
     *  первой странице — отдаём сохранённый кэш (offline-режим). */
    private void loadPage(final boolean first, final Runnable onDone) {
        // Предотвращаем повторный вход, пока предыдущая порция не загружена.
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
                // Аутентифицируемся на сервере текущим пользователем Telegram.
                api.auth(userId, phone, name);
                JSONArray videos = api.feed(offset, 20);
                // Пишем результат в кэш — при следующей ошибке сеть покажет его.
                getSharedPreferences("mediafeed_cache", 0).edit()
                        .putString("feed", videos.toString())
                        .apply();
                AndroidUtilities.runOnUIThread(() -> {
                    feedLoading = false;
                    if (destroyed) {
                        return;
                    }
                    if (adapter != null) {
                        // Первая страница заменяет список, следующие — дополняют.
                        if (first) {
                            adapter.setVideos(videos);
                        } else {
                            adapter.addVideos(videos);
                        }
                    }
                    // Сдвигаем offset на число реально добавленных элементов.
                    feedOffset = offset + videos.length();
                    if (onDone != null) {
                        onDone.run();
                    }
                });
            } catch (Exception ignore) {
                // Ошибка сети: для первой страницы пробуем cached-ленту.
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

    /** Обработка неудачной загрузки: сбрасываем флаг, вызываем колбэк,
     *  на первой странице — визуальная индикация ошибки (тряска). */
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
        // Освобождаем все VideoPlayer, чтобы не текли ресурсы.
        if (adapter != null) {
            adapter.releaseAll();
        }
    }

    /** Модель одного видео-ролика в ленте (поля из JSON ответа сервера). */
    private static class VideoItem {
        long id;              // ID видео на сервере
        String fileId;        // файл (для потоковой передачи)
        String caption;       // подпись/описание ролика
        String title;         // заголовок
        int width;            // ширина кадра (для текстурного представления)
        int height;           // высота кадра
        long durationMs;      // длительность в мс
        long likeCount;       // счётчик лайков (из статистики)
        long viewCount;       // счётчик просмотров
        long commentCount;    // счётчик комментариев
        boolean liked;        // поставил ли текущий пользователь лайк
    }

    /** Адаптер ленты: хранит список VideoItem, следит за активной позицией
     *  и держит map живущих холдеров (позиция -> FeedHolder), чтобы
     *  умело управлять play/pause. */
    private class FeedAdapter extends RecyclerView.Adapter<FeedHolder> {

        private final java.util.ArrayList<VideoItem> items = new java.util.ArrayList<>();
        private final android.util.SparseArray<FeedHolder> holders = new android.util.SparseArray<>();
        private int active = -1;   // текущая активная (воспроизводящаяся) позиция

        /** Полная замена списка роликов (первая страница / результат поиска). */
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
            // Если активной позиции ещё нет — активируем первую.
            if (active < 0 && !items.isEmpty()) {
                setActivePosition(0);
            }
        }

        /** Дописывание нового блока видео в конец (подгрузка страниц). */
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

        /** Разбор JSON-объекта ролика в VideoItem. null, если id = 0. */
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

        /** Смена активной позиции: паузим предыдущий ролик, играем новый. */
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

        /** Освобождает все плееры при закрытии экрана. */
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
            // Привязка данных к холдеру; активная позиция играет, остальные пауза.
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
            // Регистрируем холдер в map; если это активная позиция — запускаем.
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
            // Холдер переработан — уничтожаем плеер, освобождаем ресурсы.
            holder.release();
        }

        @Override
        public int getItemCount() {
            return items.size();
        }
    }

    /** Холдер одного ролика: TextureView для видео, спиннер загрузки,
     *  боковая панель с лайком/комментами/репортом/шэром. */
    private class FeedHolder extends RecyclerView.ViewHolder {

        private final FrameLayout container;   // корневой контейнер страницы
        private final android.view.TextureView textureView; // куда рендерится видео
        private final RadialProgressView loader;            // индикатор буферизации первого кадра
        private final ImageView likeButton;    // кнопка лайка
        private final TextView likeCount;      // счётчик лайков
        private final ImageView commentButton; // кнопка комментариев
        private final TextView commentCount;   // счётчик комментариев
        private final ImageView reportButton;  // кнопка репорта (жалоба)
        private VideoPlayer videoPlayer;       // плеер ExoPlayer-обёртка
        private VideoItem item;                // данные текущего ролика
        private boolean playing;               // идёт ли воспроизведение
        private boolean released;              // освобождён ли плеер (после recycle)

        FeedHolder(ViewGroup parent) {
            super(new FrameLayout(parent.getContext()));
            container = (FrameLayout) itemView;
            container.setBackgroundColor(0xff000000);
            container.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT));

            // Поверхность для отрисовки видео на весь экран.
            textureView = new android.view.TextureView(container.getContext());
            container.addView(textureView, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT));

            // Спиннер по центру — виден до первого отрисованного кадра.
            loader = new RadialProgressView(container.getContext());
            loader.setSize(AndroidUtilities.dp(48));
            container.addView(loader, LayoutHelper.createFrame(48, 48, Gravity.CENTER));

            // Боковая вертикальная панель действий (справа снизу).
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
            // Кнопка шэра (без счётчика) — открывает системный диалог "Поделиться".
            ImageView shareButton = createSideButton(R.drawable.msg_share);
            side.addView(shareButton, new LinearLayout.LayoutParams(AndroidUtilities.dp(52), AndroidUtilities.dp(52)));
            container.addView(side, LayoutHelper.createFrame(LayoutHelper.WRAP_CONTENT, LayoutHelper.WRAP_CONTENT, Gravity.BOTTOM | Gravity.RIGHT, 0, 0, 8, 24));

            // Двойной тап по видео = лайк.
            final GestureDetector gd = new GestureDetector(container.getContext(), new GestureDetector.SimpleOnGestureListener() {
                @Override
                public boolean onDoubleTap(MotionEvent e) {
                    onLike();
                    return true;
                }
            });
            // Пропускаем события жестов, сам клик не перехватываем.
            container.setOnTouchListener((v, event) -> {
                gd.onTouchEvent(event);
                return false;
            });

            likeButton.setOnClickListener(v -> onLike());
            commentButton.setOnClickListener(v -> onComment());
            shareButton.setOnClickListener(v -> onShare());
            reportButton.setOnClickListener(v -> onReport());
        }

        /** Создание иконки боковой панели (белая, с отступами). */
        private ImageView createSideButton(int res) {
            ImageView iv = new ImageView(container.getContext());
            iv.setImageResource(res);
            iv.setColorFilter(0xffffffff);
            iv.setScaleType(ImageView.ScaleType.CENTER_INSIDE);
            iv.setPadding(AndroidUtilities.dp(6), AndroidUtilities.dp(6), AndroidUtilities.dp(6), AndroidUtilities.dp(6));
            return iv;
        }

        /** Создание подписи-счётчика под кнопкой. */
        private TextView createCountLabel() {
            TextView tv = new TextView(container.getContext());
            tv.setTextColor(0xffffffff);
            tv.setTextSize(11f);
            tv.setGravity(Gravity.CENTER);
            return tv;
        }

        /** Привязка данных ролика: сброс состояния, показ спиннера,
         *  обновление счётчиков лайков и комментариев. */
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

        /** Старт воспроизведения: создаём плеер при первом вызове, шлём
         *  статистику (просмотр + счётчики), по первому кадру прячем спиннер. */
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
                        // Первый кадр готов — убираем спиннер загрузки.
                        AndroidUtilities.runOnUIThread(() -> {
                            if (loader != null) {
                                loader.setVisibility(View.GONE);
                            }
                        });
                    }
                });
            }
            videoPlayer.setTextureView(textureView);
            // Потоковое воспроизведение по URL от сервера.
            videoPlayer.preparePlayer(Uri.parse(MediaFeedServerApi.getInstance().streamUrl(item.id)), "other");
            videoPlayer.setLooping(true);
            videoPlayer.play();
            // Дополнительный зачёт просмотра в фоне.
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().view(item.id);
                } catch (Exception ignore) {
                }
            });
        }

        /** Пауза воспроизведения (видео ушло из активной позиции). */
        void pause() {
            playing = false;
            if (videoPlayer != null) {
                videoPlayer.pause();
            }
        }

        /** Полное освобождение плеера (переработка холдера / закрытие экрана). */
        void release() {
            released = true;
            playing = false;
            if (videoPlayer != null) {
                videoPlayer.releasePlayer(true);
                videoPlayer = null;
            }
        }

        /** Тоггл лайка: оптимистично меняем UI сразу, затем синхронизируем
         *  на сервере в фоне (like/unlike). */
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

        /** Обновление иконки и счётчика лайков по текущему состоянию. */
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

        /** Открытие диалога комментариев для текущего ролика. */
        private void onComment() {
            if (item == null) {
                return;
            }
            openCommentDialog(item.id);
        }

        /** Шэринг ссылки на видео через системный Intent. */
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

        /** Открытие диалога жалобы на ролик. */
        private void onReport() {
            if (item == null) {
                return;
            }
            openReportDialog(item.id);
        }

        /** Загрузка актуальных счётчиков (лайки/комменты/просмотры) в фоне. */
        private void loadStats() {
            final long id = item.id;
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONObject stats = MediaFeedServerApi.getInstance().videoStats((int) id);
                    final long likes = stats.optLong("likes");
                    final long comments = stats.optLong("comments");
                    final long views = stats.optLong("views");
                    AndroidUtilities.runOnUIThread(() -> {
                        // Проверяем, что холдер не переиспользован под другой ролик.
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

        /** Инкремент счётчика просмотров ролика на сервере. */
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

    /** Диалог комментариев: список строится в дерево по parent_id
     *  (вложенные ответы с отступами), отправка комментария — в фоне. */
    private void openCommentDialog(final long videoId) {
        final AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(AndroidUtilities.dp(8), AndroidUtilities.dp(8), AndroidUtilities.dp(8), 0);

        // TextView-список (упрощённый): все комментарии строкой с отступами.
        final android.widget.TextView list = new android.widget.TextView(this);
        list.setTextSize(13f);
        list.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        list.setLineSpacing(AndroidUtilities.dp(2), 1f);
        content.addView(list, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f));

        // Поле ввода нового комментария.
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
            // Отправка комментария без блокировки UI.
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    MediaFeedServerApi.getInstance().comment(videoId, text);
                } catch (Exception ignore) {
                }
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);

        // Загрузка и построение дерева комментариев в фоне.
        Utilities.stageQueue.postRunnable(() -> {
            final java.util.Map<Long, JSONObject> byId = new java.util.HashMap<>();
            final java.util.List<JSONObject> topLevel = new java.util.ArrayList<>();
            try {
                JSONArray comments = MediaFeedServerApi.getInstance().comments(videoId);
                for (int i = 0; i < comments.length(); i++) {
                    JSONObject c = comments.optJSONObject(i);
                    if (c == null) continue;
                    // Индексируем по id и отдельно собираем корневые (parent_id = 0).
                    byId.put(c.optLong("id"), c);
                    if (c.optLong("parent_id") == 0) {
                        topLevel.add(c);
                    }
                }
            } catch (Exception ignore) {
            }
            // Рекурсивно сериализуем дерево в текст с отступами.
            final StringBuilder sb = new StringBuilder();
            for (JSONObject c : topLevel) {
                appendComment(sb, c, byId, 0);
            }
            final String text = sb.length() == 0 ? getString(R.string.MediaFeedNoComments) : sb.toString();
            AndroidUtilities.runOnUIThread(() -> list.setText(text));
        });

        builder.show();
    }

    /** Рекурсивная отрисовка комментария и его ответов (по parent_id),
     *  глубина задаётся отступами ("└"). */
    private void appendComment(StringBuilder sb, JSONObject c, java.util.Map<Long, JSONObject> byId, int depth) {
        try {
            long uid = c.optLong("user_id");
            String text = c.optString("text");
            String time = c.optString("created_at", "");
            String indent = depth > 0 ? "  ".repeat(depth) + "└ " : "";
            sb.append(indent).append("👤 #").append(uid);
            // Вырезаем из ISO-времени только часы:минуты (11..16 символы).
            if (time.length() > 5) {
                sb.append(" · ").append(time.substring(11, 16));
            }
            sb.append("\n").append(indent).append(text).append("\n");
            // Ищем все ответы на этот комментарий и печатаем их глубже.
            for (java.util.Map.Entry<Long, JSONObject> e : byId.entrySet()) {
                if (e.getValue().optLong("parent_id") == c.optLong("id")) {
                    appendComment(sb, e.getValue(), byId, depth + 1);
                }
            }
        } catch (Exception ignore) {
        }
    }

    /** Диалог жалобы: пользователь вводит причину, отправляем на сервер. */
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
            // Отправка репорта в фоне.
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