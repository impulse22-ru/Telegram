package org.telegram.ui;

import android.app.Activity;
import android.content.Intent;
import android.content.res.Configuration;
import android.graphics.Bitmap;
import android.graphics.BitmapFactory;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Bundle;
import android.text.InputType;
import android.view.Gravity;
import android.view.View;
import android.view.ViewGroup;
import android.view.Window;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.HorizontalScrollView;
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

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.util.ArrayList;

// Витрина/каталог: магазины → товары → заказ (этапы 6–7).
// Экран каталога магазинов и товаров с подписками и заказами.
// Через заголовок доступны: все магазины, подписки пользователя,
// мои магазины (создание/редактирование/удаление, own-флаг владельца),
// заказы покупателя и заказы продавца (подтверждение), поиск магазинов
// и статистика. Заказ товара, отзывы (1–5) и чат по заказу.
public class MediaCatalogActivity extends Activity {

    /** ID текущего аккаунта Telegram (для Multi-Account) */
    private final int currentAccount = UserConfig.selectedAccount;
    /** Флаг: Activity уничтожена — все callback'и на UI проверяют его */
    private boolean destroyed;
    /** Список (RecyclerView), в который выводятся элементы каталога */
    private RecyclerView list;
    /** LayoutManager вертикального списка */
    private LinearLayoutManager layoutManager;
    /** Адаптер, который держит текущий набор элементов (магазины/товары/заказы) */
    private CatalogAdapter adapter;
    /** Контейнер чипов категорий (горизонтальная прокрутка в заголовке); null до загрузки */
    private LinearLayout chipsRow;
    /** Текущая выбранная категория (null = все магазины/товары) */
    private String selectedCategory;
    /** URL изображения, загруженного через picker (перед подтверждением формы товара) */
    private volatile String pendingImageUrl;
    /** Request code для системного picker'а изображений (ACTION_GET_CONTENT) */
    private static final int REQ_IMAGE_PICK = 4242;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        setTheme(R.style.Theme_TMessages);
        super.onCreate(savedInstanceState);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Theme.getColor(Theme.key_windowBackgroundWhite));

        // Заголовок с кнопками: магазины / подписки / мои магазины / заказы.
        LinearLayout header = new LinearLayout(this);
        header.setOrientation(LinearLayout.VERTICAL);
        header.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

        // Клик по заголовку "MediaFeed" показывает статистику пользователя.
        TextView title = new TextView(this);
        title.setText(getString(R.string.MediaFeed));
        title.setTextSize(18f);
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        title.setOnClickListener(v -> showMyStats());
        header.addView(title, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        LinearLayout row = new LinearLayout(this);
        row.setOrientation(LinearLayout.HORIZONTAL);

        // Кнопки-переключатели: подписки, мои магазины, мои заказы,
        // заказы продавца (входящие заказы покупателей).
        TextView subscriptions = button(getString(R.string.MediaFeedSubscriptions), v -> showSubscriptions());
        TextView myShops = button(getString(R.string.MediaFeedMyShops), v -> showMyShops());
        TextView myOrders = button(getString(R.string.MediaFeedMyOrders), v -> showOrders(false));
        TextView mySales = button(getString(R.string.MediaFeedSellerOrders), v -> showOrders(true));
        row.addView(subscriptions);
        row.addView(myShops);
        row.addView(myOrders);
        row.addView(mySales);
        header.addView(row);

        // Создание нового магазина (вызывается только в меню "мои магазины").
        TextView createShopText = button(getString(R.string.MediaFeedCreateShop), v -> promptCreateShop());
        header.addView(createShopText);

        // Поиск магазинов по названию.
        TextView searchText = button("🔍 Search", v -> showShopSearch());
        header.addView(searchText);

        // Чипы категорий товаров: горизонтальная прокрутка, наполняется из API.
        HorizontalScrollView chipsScroll = new HorizontalScrollView(this);
        chipsScroll.setHorizontalScrollBarEnabled(false);
        chipsRow = new LinearLayout(this);
        chipsRow.setOrientation(LinearLayout.HORIZONTAL);
        chipsScroll.addView(chipsRow);
        header.addView(chipsScroll);

        root.addView(header, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.TOP));

        // Список: высота оставляет место под фиксированный заголовок.
        list = new RecyclerView(this);
        layoutManager = new LinearLayoutManager(this);
        list.setLayoutManager(layoutManager);
        adapter = new CatalogAdapter();
        list.setAdapter(adapter);
        root.addView(list, LayoutHelper.createFrameMarginPx(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT, Gravity.TOP, 0, AndroidUtilities.dp(140), 0, 0));

        // Спиннер поверх списка, скрывается после первой загрузки.
        RadialProgressView loading = new RadialProgressView(this);
        root.addView(loading, LayoutHelper.createFrame(48, 48, Gravity.CENTER));
        setContentView(root);

        // Стартовая загрузка: показываем все магазины.
        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONArray shops = MediaFeedServerApi.getInstance().shops();
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed) {
                        return;
                    }
                    adapter.setShops(shops);
                    loading.setVisibility(View.GONE);
                });
            } catch (Exception ignore) {
                AndroidUtilities.runOnUIThread(() -> loading.setVisibility(View.GONE));
            }
        });

        // Чипы категорий: "Все" + список категорий, по клику — фильтр каталога.
        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONArray cats = MediaFeedServerApi.getInstance().categories();
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed || chipsRow == null) {
                        return;
                    }
                    buildChips(cats);
                });
            } catch (Exception ignore) {
                // Сеть/сервер недоступны — оставляем заголовок без чипов.
            }
        });
    }

    @Override
    public void onConfigurationChanged(Configuration newConfig) {
        super.onConfigurationChanged(newConfig);
        // При повороте/изменении конфигурации перерисовываем список.
        if (adapter != null) {
            adapter.notifyDataSetChanged();
        }
    }

    /** Создание текстовой кнопки заголовка (стиль "ссылки" с цветом темы). */
    private TextView button(String text, View.OnClickListener onClick) {
        TextView tv = new TextView(this);
        tv.setText(text);
        tv.setTextSize(14f);
        tv.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
        tv.setPadding(AndroidUtilities.dp(8), AndroidUtilities.dp(4), AndroidUtilities.dp(8), AndroidUtilities.dp(4));
        tv.setOnClickListener(onClick);
        return tv;
    }

    /** Построение ряда чипов категорий: всегда "Все" (null/без фильтра)
     *  + категории с сервера. Клик по чипу фильтрует каталог. */
    private void buildChips(JSONArray cats) {
        chipsRow.removeAllViews();
        // Чип "Все": возврат к полному каталогу (без фильтра).
        TextView all = chip("Все", selectedCategory == null);
        all.setOnClickListener(v -> selectChip(all, null));
        chipsRow.addView(all);

        if (cats != null) {
            for (int i = 0; i < cats.length(); i++) {
                final String cat = cats.optString(i);
                if (cat.isEmpty()) {
                    continue;
                }
                TextView c = chip(cat, cat.equals(selectedCategory));
                c.setOnClickListener(v -> selectChip(c, cat));
                chipsRow.addView(c);
            }
        }
    }

    /** Создание одного чипа категории: скруглённый фон, цвет темы; для выбранного — заливка. */
    private TextView chip(String label, boolean selected) {
        GradientDrawable bg = new GradientDrawable();
        bg.setShape(GradientDrawable.RECTANGLE);
        bg.setCornerRadius(AndroidUtilities.dp(16));
        if (selected) {
            bg.setColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
        } else {
            bg.setColor(Theme.getColor(Theme.key_windowBackgroundWhite));
            bg.setStroke(AndroidUtilities.dp(1), Theme.getColor(Theme.key_dialogGrayLine));
        }
        TextView tv = new TextView(this);
        tv.setText(label);
        tv.setTextSize(13f);
        tv.setTextColor(selected
                ? Theme.getColor(Theme.key_windowBackgroundWhite)
                : Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
        tv.setPadding(AndroidUtilities.dp(14), AndroidUtilities.dp(7), AndroidUtilities.dp(14), AndroidUtilities.dp(7));
        tv.setBackground(bg);
        LinearLayout.LayoutParams lp = new LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.WRAP_CONTENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        lp.rightMargin = AndroidUtilities.dp(6);
        tv.setLayoutParams(lp);
        return tv;
    }

    /** Выбор чипа: перекрашиваем все чипы, запоминаем категорию, грузим каталог. */
    private void selectChip(TextView chosen, String category) {
        if (chipsRow == null) {
            return;
        }
        selectedCategory = category;
        for (int i = 0; i < chipsRow.getChildCount(); i++) {
            TextView chip = (TextView) chipsRow.getChildAt(i);
            boolean sel = chip == chosen;
            GradientDrawable bg = new GradientDrawable();
            bg.setShape(GradientDrawable.RECTANGLE);
            bg.setCornerRadius(AndroidUtilities.dp(16));
            if (sel) {
                bg.setColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
            } else {
                bg.setColor(Theme.getColor(Theme.key_windowBackgroundWhite));
                bg.setStroke(AndroidUtilities.dp(1), Theme.getColor(Theme.key_dialogGrayLine));
            }
            chip.setBackground(bg);
            chip.setTextColor(sel
                    ? Theme.getColor(Theme.key_windowBackgroundWhite)
                    : Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
        }
        adapter.showCatalogCategory(category);
    }

    /** Запуск системного picker'а изображений (ACTION_GET_CONTENT). Результат в onActivityResult. */
    private void pickImage() {
        try {
            Intent i = new Intent(Intent.ACTION_GET_CONTENT);
            i.setType("image/*");
            startActivityForResult(i, REQ_IMAGE_PICK);
        } catch (Exception ignore) {
            AndroidUtilities.shakeView(getWindow().getDecorView());
        }
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        if (requestCode == REQ_IMAGE_PICK && resultCode == RESULT_OK && data != null && data.getData() != null) {
            uploadPickedImage(data.getData());
            return;
        }
        super.onActivityResult(requestCode, resultCode, data);
    }

    /** Чтение, сжатие и загрузка выбранного изображения в фоне (stageQueue);
     *  результат (URL) кладётся в pendingImageUrl для формы товара/магазина. */
    private void uploadPickedImage(final Uri uri) {
        Utilities.stageQueue.postRunnable(() -> {
            try (InputStream in = getContentResolver().openInputStream(uri)) {
                if (in == null) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    return;
                }
                // Декодируем с понижением размера: сжатие до ~1280px по большей стороне.
                Bitmap bmp = BitmapFactory.decodeStream(in);
                if (bmp == null) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    return;
                }
                Bitmap scaled = bmp;
                int maxSide = 1280;
                int w = bmp.getWidth();
                int h = bmp.getHeight();
                int side = Math.max(w, h);
                if (side > maxSide) {
                    float k = (float) maxSide / side;
                    scaled = Bitmap.createScaledBitmap(bmp, (int) (w * k), (int) (h * k), true);
                    if (scaled != bmp) {
                        bmp.recycle();
                    }
                }
                ByteArrayOutputStream bos = new ByteArrayOutputStream();
                scaled.compress(Bitmap.CompressFormat.JPEG, 82, bos);
                if (scaled != bmp) {
                    scaled.recycle();
                }
                byte[] jpeg = bos.toByteArray();

                // Загружаем на сервер; URL сохраняем для последующего create/update.
                final String url = MediaFeedServerApi.getInstance().uploadImage(jpeg);
                pendingImageUrl = url;
                AndroidUtilities.runOnUIThread(() -> {
                    if (!destroyed && url != null && !url.isEmpty()) {
                        AlertDialog.Builder b = new AlertDialog.Builder(MediaCatalogActivity.this);
                        b.setTitle("🖼");
                        b.setMessage(getString(R.string.MediaFeedImageReady));
                        b.setPositiveButton(getString(R.string.OK), null);
                        b.show();
                    }
                });
            } catch (Exception e) {
                AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
            }
        });
    }

    /** Показ личной статистики: просмотры, время просмотра, лайки, комменты,
     *  подписки — плюс продажи продавца (заказы, подтверждённые, выручка). */
    private void showMyStats() {
        Utilities.stageQueue.postRunnable(() -> {
            final StringBuilder sb = new StringBuilder();
            try {
                JSONObject st = MediaFeedServerApi.getInstance().myStats();
                sb.append("👀 ").append(st.optLong("views"))
                        .append("  ⏱ ").append(st.optLong("watch_seconds"))
                        .append("s\n❤ ").append(st.optLong("likes_given"))
                        .append("  💬 ").append(st.optLong("comments_given"))
                        .append("  🔔 ").append(st.optLong("subscriptions")).append("\n");
            } catch (Exception ignore) {
                sb.append("stats unavailable\n");
            }
            try {
                JSONObject s = MediaFeedServerApi.getInstance().salesStats();
                sb.append("\nПродажи:\n📦 ").append(s.optLong("orders"))
                        .append("  ⏳ ").append(s.optLong("pending"))
                        .append("  ✅ ").append(s.optLong("confirmed"))
                        .append("\n💰 ").append(s.optDouble("revenue", 0))
                        .append("  👁 ").append(s.optLong("product_views"));
            } catch (Exception ignore) {
                sb.append("Пока нет продаж");
            }
            final String text = sb.toString();
            AndroidUtilities.runOnUIThread(() -> {
                if (destroyed) return;
                AlertDialog.Builder b = new AlertDialog.Builder(MediaCatalogActivity.this);
                b.setTitle(getString(R.string.MediaFeedStatsMe));
                b.setMessage(text);
                b.setPositiveButton(getString(R.string.OK), null);
                b.show();
            });
        });
    }

    /** Показ заказов: mine=false — мои покупки, mine=true — заказы продавца.
     *  Во втором случае у заказа появляется кнопка подтверждения (extra). */
    private void showOrders(final boolean mine) {
        Utilities.stageQueue.postRunnable(() -> {
            JSONArray arr;
            try {
                arr = mine
                        ? MediaFeedServerApi.getInstance().sellerOrders()
                        : MediaFeedServerApi.getInstance().myOrders();
            } catch (Exception e) {
                AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                return;
            }
            final JSONArray orders = arr;
            AndroidUtilities.runOnUIThread(() -> {
                if (destroyed) {
                    return;
                }
                // Передаём флаг mine адаптеру: он помечает "это заказ продавца".
                adapter.setOrders(orders, mine);
            });
        });
    }

    /** Показ магазинов, на которые подписан пользователь: берём все магазины
     *  и фильтруем по списку id подписок (tg_chat_id). */
    private void showSubscriptions() {
        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONArray channelIds = MediaFeedServerApi.getInstance().mySubscriptions();
                JSONArray allShops = MediaFeedServerApi.getInstance().shops();
                // Соберём ids каналов, на которые подписаны.
                ArrayList<Long> subs = new ArrayList<>();
                for (int i = 0; i < channelIds.length(); i++) {
                    subs.add(channelIds.optLong(i));
                }
                // Фильтруем магазины: оставляем только те, чей канал в подписках.
                JSONArray filtered = new JSONArray();
                if (allShops != null) {
                    for (int i = 0; i < allShops.length(); i++) {
                        JSONObject s = allShops.optJSONObject(i);
                        if (s != null && subs.contains(s.optLong("tg_chat_id"))) {
                            filtered.put(s);
                        }
                    }
                }
                final JSONArray result = filtered;
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed) return;
                    adapter.setShops(result);
                });
            } catch (Exception e) {
                AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
            }
        });
    }

    /** Показ магазинов текущего пользователя (через API myShops). */
    private void showMyShops() {
        Utilities.stageQueue.postRunnable(() -> {
            try {
                JSONArray shops = MediaFeedServerApi.getInstance().myShops();
                AndroidUtilities.runOnUIThread(() -> {
                    if (destroyed) return;
                    adapter.setShops(shops);
                });
            } catch (Exception e) {
                AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
            }
        });
    }

    /** Диалог поиска магазинов по названию. */
    private void showShopSearch() {
        AlertDialog.Builder builder = new AlertDialog.Builder(this);
        final EditText input = new EditText(this);
        input.setHint(getString(R.string.MediaFeedSearchHint));
        input.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        input.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        FrameLayout frame = new FrameLayout(this);
        frame.addView(input, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.CENTER, 16, 16, 16, 16));
        builder.setTitle("🔍");
        builder.setView(frame);
        builder.setPositiveButton(getString(R.string.Search), (dialog, which) -> {
            final String q = input.getText().toString().trim();
            if (q.isEmpty()) {
                return;
            }
            // Отправляем запрос в фоне, результат — список магазинов.
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONArray found = MediaFeedServerApi.getInstance().searchShops(q);
                    AndroidUtilities.runOnUIThread(() -> {
                        if (destroyed) {
                            return;
                        }
                        adapter.setShops(found);
                    });
                } catch (Exception ignore) {
                }
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);
        builder.show();
    }

    /** Диалог создания нового магазина: название, описание, платёжные реквизиты.
     *  После создания автоматически переходим в "мои магазины". */
    private void promptCreateShop() {
        AlertDialog.Builder builder = new AlertDialog.Builder(this);
        LinearLayout content = new LinearLayout(this);
        content.setOrientation(LinearLayout.VERTICAL);
        content.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

        final EditText titleInput = new EditText(this);
        titleInput.setHint(getString(R.string.MediaFeedShopTitleHint));
        titleInput.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        titleInput.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        content.addView(titleInput, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        final EditText descInput = new EditText(this);
        descInput.setHint(getString(R.string.MediaFeedShopDescHint));
        descInput.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        descInput.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        content.addView(descInput, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        final EditText paymentInput = new EditText(this);
        paymentInput.setHint(getString(R.string.MediaFeedShopPaymentHint));
        paymentInput.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        paymentInput.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
        content.addView(paymentInput, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

        builder.setTitle(getString(R.string.MediaFeedCreateShop));
        builder.setView(content);
        builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
            final String title = titleInput.getText().toString().trim();
            if (title.isEmpty()) return;
            final String desc = descInput.getText().toString();
            final String pay = paymentInput.getText().toString();
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    // Автосоздание магазина (канал создаётся на сервере).
                    MediaFeedServerApi.getInstance().createShopAuto(title, desc, pay, null);
                    JSONArray shops = MediaFeedServerApi.getInstance().myShops();
                    AndroidUtilities.runOnUIThread(() -> {
                        if (!destroyed) adapter.setShops(shops);
                    });
                } catch (Exception e) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                }
            });
        });
        builder.setNegativeButton(getString(R.string.Cancel), null);
        builder.show();
    }

    /** Адаптер каталога: хранит текущий режим отображения (shops / products /
     *  orders) в полях shops/products и строит из них список ListItem.
     *  Занимается открытием магазина/товара, CRUD магазинов и товаров,
     *  подписками и созданием заказов. */
    private class CatalogAdapter extends RecyclerView.Adapter<CatalogHolder> {

        /** Текущий список элементов для RecyclerView (пересобирается rebuild'ами) */
        private final ArrayList<ListItem> items = new ArrayList<>();
        /** Актуальный список магазинов (режим "каталог магазинов") */
        private JSONArray shops;
        /** Актуальный список товаров (режим "магазин с товарами") */
        private JSONArray products;

        /** Показ списка магазинов: очищает товары, пересобирает items. */
        void setShops(JSONArray list) {
            this.shops = list;
            this.products = null;
            rebuild();
        }

        /** Показ каталога товаров по категории (или без фильтра, если category null).
         *  items строятся из ответа /v1/catalog без заголовка магазина. */
        void setCatalogProducts(JSONArray itemsArray) {
            shops = null;
            items.clear();
            if (itemsArray != null) {
                for (int i = 0; i < itemsArray.length(); i++) {
                    JSONObject p = itemsArray.optJSONObject(i);
                    if (p == null) {
                        continue;
                    }
                    ListItem it = new ListItem();
                    it.productId = p.optLong("id");
                    it.shopId = p.optLong("shop_id");
                    it.title = p.optString("title");
                    it.imageUrl = p.optString("image_url", null);
                    it.subtitle = "💰 " + p.optDouble("price_amount", 0) + " "
                            + p.optString("price_currency")
                            + "  ·  " + p.optString("category");
                    if (it.imageUrl != null && !it.imageUrl.isEmpty()) {
                        // Отмечаем товары с картинкой эмодзи-индикатором.
                        it.subtitle += "  🖼";
                    }
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        /** Загрузка каталога по категории в фоне; null → все товары. */
        void showCatalogCategory(final String category) {
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONArray arr = MediaFeedServerApi.getInstance().catalog(category);
                    AndroidUtilities.runOnUIThread(() -> {
                        if (!destroyed) {
                            setCatalogProducts(arr);
                        }
                    });
                } catch (Exception e) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                }
            });
        }

        /** Показ всех магазинов (кнопка "магазины") с сервера. */
        void showShops() {
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONArray list = MediaFeedServerApi.getInstance().shops();
                    AndroidUtilities.runOnUIThread(() -> {
                        if (!destroyed) {
                            setShops(list);
                        }
                    });
                } catch (Exception ignore) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                }
            });
        }

        /** Переход в режим "товары магазина": первым элементом идёт заголовок
         *  магазина (head=true), далее его товары. Считаем own-флаг —
         *  владелец ли магазина текущий пользователь. */
        void setShopProducts(JSONObject shopResp) {
            shops = null;
            products = shopResp.optJSONArray("products");
            // Заголовок магазина вверху списка товаров.
            ListItem shopItem = new ListItem();
            shopItem.head = true;
            shopItem.shopId = shopResp.optJSONObject("shop") != null
                    ? shopResp.optJSONObject("shop").optLong("id") : 0;
            shopItem.title = shopResp.optJSONObject("shop") != null
                    ? shopResp.optJSONObject("shop").optString("title") : "Магазин";
            shopItem.subtitle = "👁 " + (shopResp.optJSONObject("shop") != null
                    ? shopResp.optJSONObject("shop").optString("payment_info") : "");
            shopItem.subscribed = shopResp.optBoolean("subscribed");
            // Сравниваем owner_id магазина с ID текущего пользователя.
            if (shopResp.optJSONObject("shop") != null) {
                JSONObject shop = shopResp.optJSONObject("shop");
                final long ownerId = shop.optLong("owner_id");
                final long myId = UserConfig.getInstance(currentAccount).getClientUserId();
                shopItem.owned = ownerId == myId;
            }
            rebuildFromShop(shopItem);
        }

        /** Диалог редактирования магазина (название/описание/реквизиты) —
         *  доступен только владельцу (own). После сохранения открываем магазин. */
        void promptEditShop(final ListItem shopItem) {
            if (shopItem.shopId == 0) {
                return;
            }
            final long shopId = shopItem.shopId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);
            content.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

            final EditText titleIn = new EditText(MediaCatalogActivity.this);
            titleIn.setHint(getString(R.string.MediaFeedShopTitleHint));
            titleIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            titleIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(titleIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText descIn = new EditText(MediaCatalogActivity.this);
            descIn.setHint(getString(R.string.MediaFeedShopDescHint));
            descIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            descIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(descIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText payIn = new EditText(MediaCatalogActivity.this);
            payIn.setHint(getString(R.string.MediaFeedShopPaymentHint));
            payIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            payIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(payIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            builder.setTitle(getString(R.string.MediaFeedEditShop));
            builder.setView(content);
            builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
                final String title = titleIn.getText().toString().trim();
                if (title.isEmpty()) {
                    return;
                }
                final String desc = descIn.getText().toString();
                final String pay = payIn.getText().toString();
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        MediaFeedServerApi.getInstance().updateShop((int) shopId, title, desc, pay, null);
                        JSONObject resp = MediaFeedServerApi.getInstance().shop((int) shopId);
                        AndroidUtilities.runOnUIThread(() -> {
                            if (!destroyed) {
                                setShopProducts(resp);
                            }
                        });
                    } catch (Exception ignore) {
                        AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Подтверждение удаления магазина; после удаления показываем список магазинов. */
        void confirmDeleteShop(final ListItem shopItem) {
            if (shopItem.shopId == 0) {
                return;
            }
            final long shopId = shopItem.shopId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            builder.setTitle(getString(R.string.MediaFeedDeleteShop));
            builder.setMessage(shopItem.title);
            builder.setPositiveButton(getString(R.string.Delete), (dialog, which) ->
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().deleteShop((int) shopId);
                            AndroidUtilities.runOnUIThread(() -> {
                                if (!destroyed) {
                                    showShops();
                                }
                            });
                        } catch (Exception ignore) {
                            AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                        }
                    }));
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Диалог редактирования товара (название/описание/цена) — own-tовар. */
        void promptEditProduct(final ListItem p) {
            if (p.productId == 0) {
                return;
            }
            final long pid = p.productId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);
            content.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

            final EditText titleIn = new EditText(MediaCatalogActivity.this);
            titleIn.setHint(getString(R.string.MediaFeedProductTitle));
            titleIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            titleIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(titleIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText descIn = new EditText(MediaCatalogActivity.this);
            descIn.setHint(getString(R.string.MediaFeedShopDescHint));
            descIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            descIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(descIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText priceIn = new EditText(MediaCatalogActivity.this);
            priceIn.setHint(getString(R.string.MediaFeedProductPrice));
            priceIn.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
            priceIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            priceIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(priceIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Выбор нового изображения товара (перезапишет старое при сохранении).
            content.addView(button("🖼 " + getString(R.string.MediaFeedPickImage), v -> pickImage()),
                    new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            builder.setTitle(getString(R.string.MediaFeedEditProduct));
            builder.setView(content);
            builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
                final String title = titleIn.getText().toString().trim();
                if (title.isEmpty()) {
                    return;
                }
                final String desc = descIn.getText().toString();
                final double price;
                try {
                    price = Double.parseDouble(priceIn.getText().toString());
                } catch (Exception ignore) {
                    return;
                }
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        final String imgUrl = pendingImageUrl;
                        MediaFeedServerApi.getInstance().updateProduct((int) pid, title, desc, price, "RUB", null, imgUrl);
                        AndroidUtilities.runOnUIThread(() -> {
                            if (!destroyed && p.shopId != 0) {
                                pendingImageUrl = null;
                                adapter.openShop(p.shopId);
                            }
                        });
                    } catch (Exception ignore) {
                        AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Подтверждение удаления товара; затем переоткрываем магазин. */
        void confirmDeleteProduct(final ListItem p) {
            if (p.productId == 0) {
                return;
            }
            final long pid = p.productId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            builder.setTitle(getString(R.string.MediaFeedDeleteProduct));
            builder.setMessage(p.title);
            builder.setPositiveButton(getString(R.string.Delete), (dialog, which) ->
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().deleteProduct((int) pid);
                            AndroidUtilities.runOnUIThread(() -> {
                                if (!destroyed && p.shopId != 0) {
                                    adapter.openShop(p.shopId);
                                }
                            });
                        } catch (Exception ignore) {
                            AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                        }
                    }));
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Диалог создания нового товара в своём магазине (own). */
        void promptCreateProduct(final ListItem shopItem) {
            if (shopItem.shopId == 0) {
                return;
            }
            final long shopId = shopItem.shopId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);
            content.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

            final EditText titleIn = new EditText(MediaCatalogActivity.this);
            titleIn.setHint(getString(R.string.MediaFeedProductTitle));
            titleIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            titleIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(titleIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText descIn = new EditText(MediaCatalogActivity.this);
            descIn.setHint(getString(R.string.MediaFeedShopDescHint));
            descIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            descIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(descIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText priceIn = new EditText(MediaCatalogActivity.this);
            priceIn.setHint(getString(R.string.MediaFeedProductPrice));
            priceIn.setInputType(InputType.TYPE_CLASS_NUMBER | InputType.TYPE_NUMBER_FLAG_DECIMAL);
            priceIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            priceIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(priceIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText curIn = new EditText(MediaCatalogActivity.this);
            curIn.setHint("RUB");
            curIn.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            curIn.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(curIn, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Выбор изображения товара: перед созданием можно загрузить картинку.
            content.addView(button("🖼 " + getString(R.string.MediaFeedPickImage), v -> pickImage()),
                    new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            builder.setTitle(getString(R.string.MediaFeedNewProduct));
            builder.setView(content);
            builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
                final String title = titleIn.getText().toString().trim();
                if (title.isEmpty()) {
                    return;
                }
                final String description = descIn.getText().toString();
                final double price;
                try {
                    price = Double.parseDouble(priceIn.getText().toString());
                } catch (Exception ignore) {
                    return;
                }
                // Валюта по умолчанию RUB, если поле пустое.
                final String currency = curIn.getText().toString().trim().isEmpty() ? "RUB" : curIn.getText().toString().trim();
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        final String imgUrl = pendingImageUrl;
                        MediaFeedServerApi.getInstance().createProduct(shopId, title, description, price, currency, null, imgUrl);
                        JSONObject resp = MediaFeedServerApi.getInstance().shop((int) shopId);
                        AndroidUtilities.runOnUIThread(() -> {
                            if (!destroyed) {
                                pendingImageUrl = null;
                                setShopProducts(resp);
                            }
                        });
                    } catch (Exception ignore) {
                        AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Тоггл подписки на магазин: шлём subscribe/unsubscribe на сервер,
         *  после успеха обновляем подпись магазина в списке. */
        void toggleSubscription(final ListItem shopItem) {
            if (shopItem.shopId == 0) {
                return;
            }
            final int id = (int) shopItem.shopId;
            final boolean wasSubscribed = shopItem.subscribed;
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    if (wasSubscribed) {
                        MediaFeedServerApi.getInstance().unsubscribe(id);
                    } else {
                        MediaFeedServerApi.getInstance().subscribe(id);
                    }
                    AndroidUtilities.runOnUIThread(() -> {
                        if (destroyed) return;
                        // Меняем состояние уже после успешного ответа сервера.
                        shopItem.subscribed = !wasSubscribed;
                        updateShopSubtitle(shopItem);
                    });
                } catch (Exception e) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                }
            });
        }

        /** Обновление второй строки магазина: добавляем/убираем статус
         *  "подписан / отписаться". */
        private void updateShopSubtitle(ListItem shopItem) {
            String status = shopItem.subscribed ? getString(R.string.MediaFeedSubscribed)
                    : getString(R.string.MediaFeedUnsubscribe);
            if (shopItem.subtitle != null && shopItem.subtitle.length() > 2) {
                // Берём первую строку подписи и дописываем статус новой строкой.
                String base = shopItem.subtitle;
                int idx = base.indexOf("\n");
                if (idx > 0) {
                    base = base.substring(0, idx);
                }
                shopItem.subtitle = base + "\n" + status;
            } else {
                shopItem.subtitle = status;
            }
            notifyDataSetChanged();
        }

        /** Показ заказов: mine=false — покупки пользователя (кнопка "оплатить"),
         *  mine=true — заказы продавца (кнопка "подтвердить"). Формируем ListItem
         *  с бейджем статуса (paid/confirmed/cancelled/новый). */
        void setOrders(JSONArray orders, boolean mine) {
            this.shops = null;
            this.products = null;
            items.clear();
            if (orders != null) {
                for (int i = 0; i < orders.length(); i++) {
                    JSONObject o = orders.optJSONObject(i);
                    if (o == null) continue;
                    ListItem it = new ListItem();
                    it.orderId = o.optLong("id");
                    it.title = "Заказ #" + it.orderId;
                    String status = o.optString("payment_status");
                    // Бейдж статуса заказа: цветная эмодзи + текст статуса.
                    String badge;
                    switch (status == null ? "" : status) {
                        case "paid": badge = "💳 " + status; break;
                        case "confirmed": badge = "✅ " + status; break;
                        case "cancelled": badge = "❌ " + status; break;
                        default: badge = "🆕 " + status; break;
                    }
                    it.subtitle = badge
                            + " — " + o.optDouble("price_amount", 0) + " "
                            + o.optString("price_currency");
                    // extra=true означает "заказ продавца" (можно подтвердить).
                    it.extra = mine;
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        /** Сборка списка из shops: каждый магазин — ListItem с head=true. */
        void rebuild() {
            items.clear();
            if (shops != null) {
                for (int i = 0; i < shops.length(); i++) {
                    JSONObject s = shops.optJSONObject(i);
                    if (s == null) continue;
                    ListItem it = new ListItem();
                    it.shopId = s.optLong("id");
                    it.title = s.optString("title");
                    // Подпись: платёжная информация + статус магазина.
                    it.subtitle = "💰 " + s.optString("payment_info") + "\n" + s.optString("status");
                    it.head = true;
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        /** Сборка списка: первый элемент — заголовок магазина, далее товары.
         *  own-флаг пробрасывается на все товары магазина. */
        void rebuildFromShop(ListItem shopItem) {
            items.clear();
            items.add(shopItem);
            if (products != null) {
                for (int i = 0; i < products.length(); i++) {
                    JSONObject p = products.optJSONObject(i);
                    if (p == null) continue;
                    ListItem it = new ListItem();
                    it.productId = p.optLong("id");
                    it.shopId = shopItem.shopId;
                    it.owned = shopItem.owned;
                    it.title = p.optString("title");
                    // Подпись товара: цена + валюта + описание.
                    it.subtitle = "💰 " + p.optDouble("price_amount", 0) + " "
                            + p.optString("price_currency") + "\n" + p.optString("description");
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        /** Открытие магазина по id: грузим shop()-ответ и показываем товары. */
        void openShop(long shopId) {
            Utilities.stageQueue.postRunnable(() -> {
                try {
                    JSONObject resp = MediaFeedServerApi.getInstance().shop((int) shopId);
                    AndroidUtilities.runOnUIThread(() -> {
                        if (destroyed) return;
                        setShopProducts(resp);
                    });
                } catch (Exception e) {
                    AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                }
            });
        }

        /** Диалог заказа товара: просмотр информации, счётчик просмотра товара,
         *  поле контакта и количества, кнопки "купить" и "отзыв". */
        void openProduct(ListItem p) {
            // Увеличиваем счётчик просмотров товара на сервере.
            if (p.productId != 0) {
                final int pid = (int) p.productId;
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        MediaFeedServerApi.getInstance().productView(pid);
                    } catch (Exception ignore) {
                    }
                });
            }
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);
            // Информация о товаре: название + подпись (цена/описание).
            TextView info = new TextView(MediaCatalogActivity.this);
            info.setTextSize(15f);
            info.setText(p.title + "\n" + p.subtitle);
            info.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            content.addView(info, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Контакт покупателя (передаётся в заказ).
            final EditText contact = new EditText(MediaCatalogActivity.this);
            contact.setHint(getString(R.string.MediaFeedContact));
            contact.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            contact.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(contact, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Количество единиц товара (по умолчанию 1).
            final EditText qty = new EditText(MediaCatalogActivity.this);
            qty.setInputType(InputType.TYPE_CLASS_NUMBER);
            qty.setText("1");
            qty.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            content.addView(qty, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            builder.setTitle(getString(R.string.MediaFeedBuy));
            builder.setView(content);
            builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
                int quantity = 1;
                try {
                    quantity = Math.max(1, Integer.parseInt(qty.getText().toString()));
                } catch (Exception ignore) {
                }
                final int q = quantity;
                final String contactText = contact.getText().toString();
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        // Создаём заказ; цена берётся сервером, клиент передаёт 0.
                        MediaFeedServerApi.getInstance().createOrder((int) p.productId, q,
                                priceOf(p), contactText);
                        AndroidUtilities.runOnUIThread(() -> {
                            if (!destroyed) {
                                // После заказа показываем "мои заказы" (покупательская вкладка).
                                showOrders(false);
                            }
                        });
                    } catch (Exception ignore) {
                        AndroidUtilities.runOnUIThread(() -> AndroidUtilities.shakeView(getWindow().getDecorView()));
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            // Отдельная кнопка "отзыв" на товар (звезда).
            builder.setNeutralButton("⭐ " + getString(R.string.MediaFeedReview), (dialog, which) -> {
                promptReview(p);
            });
            builder.show();
        }

        /** Диалог отзыва на товар: оценка 1–5 и текст. */
        private void promptReview(final ListItem p) {
            final int pid = (int) p.productId;
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);

            final EditText rating = new EditText(MediaCatalogActivity.this);
            rating.setHint("1-5");
            rating.setInputType(InputType.TYPE_CLASS_NUMBER);
            rating.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            rating.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(rating, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText text = new EditText(MediaCatalogActivity.this);
            text.setHint(getString(R.string.MediaFeedReviewTextHint));
            text.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            text.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(text, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            builder.setTitle("⭐ " + getString(R.string.MediaFeedReview));
            builder.setView(content);
            builder.setPositiveButton(getString(R.string.OK), (dialog, which) -> {
                final int r;
                try {
                    r = Integer.parseInt(rating.getText().toString());
                } catch (Exception ignore) {
                    return;
                }
                // Валидация оценки: только 1..5.
                if (r < 1 || r > 5) {
                    return;
                }
                final String t = text.getText().toString();
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        MediaFeedServerApi.getInstance().review(pid, r, t);
                    } catch (Exception ignore) {
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Цена передаётся 0: сервер сам подставляет цену товара. */
        private double priceOf(ListItem p) {
            return 0; // цена берётся сервером из товара
        }

        @NonNull
        @Override
        public CatalogHolder onCreateViewHolder(@NonNull ViewGroup parent, int viewType) {
            return new CatalogHolder(parent);
        }

        @Override
        public void onBindViewHolder(@NonNull CatalogHolder holder, int position) {
            // Привязка: title/subtitle текущего элемента к TextView холдера.
            holder.bind(items.get(position));
        }

        @Override
        public int getItemCount() {
            return items.size();
        }
    }

    /** Модель одного элемента списка каталога (магазин / товар / заказ).
     *  Тип определяется по заполненным id: shopId / productId / orderId. */
    private class ListItem {
        long shopId;        // id магазина (магазин или заголовок в товарах)
        long productId;     // id товара
        long orderId;       // id заказа
        boolean head;       // true = это заголовок магазина в списке товаров
        boolean subscribed; // подписан ли пользователь на магазин
        boolean owned;      // владелец ли магазина текущий пользователь
        String title;       // заголовок строки
        String subtitle;    // вторая строка (статус/цена/описание)
        String imageUrl;    // URL картинки товара (если задана)
        boolean extra; // true = заказ продавца (можно подтвердить)
    }

    /** Холдер строки каталога: две строки (title + subtitle).
     *  Обработка кликов: что открыть в зависимости от типа элемента. */
    private class CatalogHolder extends RecyclerView.ViewHolder {

        private final TextView title;
        private final TextView subtitle;

        CatalogHolder(ViewGroup parent) {
            super(new LinearLayout(parent.getContext()));
            LinearLayout row = (LinearLayout) itemView;
            row.setOrientation(LinearLayout.VERTICAL);
            row.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(10), AndroidUtilities.dp(12), AndroidUtilities.dp(10));
            row.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Первая строка — заголовок элемента (название магазина/товара/заказа).
            title = new TextView(row.getContext());
            title.setTextSize(16f);
            title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            row.addView(title, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            // Вторая строка — подпись (платёжные данные, статус, цена и т.п.).
            subtitle = new TextView(row.getContext());
            subtitle.setTextSize(13f);
            subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
            row.addView(subtitle, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            row.setOnClickListener(v -> onClick());
        }

        /** Маршрутизация клика по типу элемента:
         *  заголовок магазина в товарах — меню подписки/управления;
         *  магазин — открыть его товары; товар — купить или меню (если own);
         *  заказ — действия по заказу. */
        private void onClick() {
            int pos = getAdapterPosition();
            if (pos < 0 || pos >= adapter.items.size()) {
                return;
            }
            ListItem it = adapter.items.get(pos);
            if (it.shopId != 0 && it.head && adapter.products != null) {
                // Заголовок магазина в деталях: показать подписку/отписку.
                openShopSubscribeDialog(it);
            } else if (it.shopId != 0) {
                // Обычный магазин в каталоге: открыть список товаров.
                adapter.openShop(it.shopId);
            } else if (it.productId != 0) {
                // Свой товар — меню (купить/редактировать/удалить), чужой — купить.
                if (it.owned) {
                    openOwnedProductMenu(it);
                } else {
                    adapter.openProduct(it);
                }
            } else if (it.orderId != 0) {
                // Заказ: диалог действий (оплатить/подтвердить/чат/отмена).
                openOrderActions(it);
            }
        }

        /** Меню действий для собственного товара (own): купить,
         *  редактировать или удалить. */
        private void openOwnedProductMenu(final ListItem it) {
            final java.util.ArrayList<String> actions = new java.util.ArrayList<>();
            final java.util.ArrayList<Runnable> runs = new java.util.ArrayList<>();
            actions.add(getString(R.string.MediaFeedBuy));
            runs.add(() -> adapter.openProduct(it));
            actions.add("✏ " + getString(R.string.MediaFeedEditProduct));
            runs.add(() -> adapter.promptEditProduct(it));
            actions.add("🗑 " + getString(R.string.MediaFeedDeleteProduct));
            runs.add(() -> adapter.confirmDeleteProduct(it));

            // setItems: список действий (строки) → соответствующие Runnable.
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            builder.setTitle(it.title);
            builder.setItems(actions.toArray(new CharSequence[0]), (dialog, which) -> {
                if (which >= 0 && which < runs.size()) {
                    runs.get(which).run();
                }
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Меню заголовка магазина: подписка/отписка, создание товара;
         *  для владельца (own) добавляются редактирование и удаление магазина. */
        private void openShopSubscribeDialog(final ListItem it) {
            final java.util.ArrayList<String> actions = new java.util.ArrayList<>();
            final java.util.ArrayList<Runnable> runs = new java.util.ArrayList<>();
            actions.add(it.subscribed
                    ? getString(R.string.MediaFeedUnsubscribe)
                    : getString(R.string.MediaFeedSubscribe));
            runs.add(() -> adapter.toggleSubscription(it));
            actions.add("+ " + getString(R.string.MediaFeedNewProduct));
            runs.add(() -> adapter.promptCreateProduct(it));
            // Пункты управления магазином доступны только его владельцу.
            if (it.owned) {
                actions.add("✏ " + getString(R.string.MediaFeedEditShop));
                runs.add(() -> adapter.promptEditShop(it));
                actions.add("🗑 " + getString(R.string.MediaFeedDeleteShop));
                runs.add(() -> adapter.confirmDeleteShop(it));
            }

            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            builder.setTitle(it.title);
            builder.setItems(actions.toArray(new CharSequence[0]), (dialog, which) -> {
                if (which >= 0 && which < runs.size()) {
                    runs.get(which).run();
                }
            });
            builder.setNegativeButton(getString(R.string.Cancel), null);
            builder.show();
        }

        /** Действия по заказу: для заказа продавца (extra) — "подтвердить",
         *  для покупки — "оплатил", плюс общий "чат с покупателем/продавцом"
         *  и "отменить заказ". */
        private void openOrderActions(final ListItem it) {
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            TextView info = new TextView(MediaCatalogActivity.this);
            info.setTextSize(15f);
            info.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            info.setText(it.title + "\n" + it.subtitle);
            builder.setView(info);
            builder.setTitle(getString(R.string.MediaFeedOrders));

            if (it.extra) {
                // Заказ пришёл продавцу — он подтверждает выполнение.
                builder.setPositiveButton(getString(R.string.MediaFeedConfirm), (dialog, which) -> {
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().confirmOrder((int) it.orderId);
                        } catch (Exception ignore) {
                        }
                    });
                });
            } else {
                // Покупка покупателя — он отмечает, что оплатил.
                builder.setPositiveButton(getString(R.string.MediaFeedPaid), (dialog, which) -> {
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().payOrder((int) it.orderId);
                        } catch (Exception ignore) {
                        }
                    });
                });
            }
            // Получение ссылки на чат по заказу (tg_chat_id / peer).
            builder.setNeutralButton(getString(R.string.MediaFeedChat), (dialog, which) -> {
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        JSONObject chat = MediaFeedServerApi.getInstance().orderChat((int) it.orderId);
                        final String link = chat.optString("tg_chat_id", "0");
                        final String peer = chat.optString("peer_name", "");
                        AndroidUtilities.runOnUIThread(() -> {
                            if (destroyed) {
                                return;
                            }
                            AlertDialog.Builder cb = new AlertDialog.Builder(MediaCatalogActivity.this);
                            cb.setTitle(getString(R.string.MediaFeedChat));
                            cb.setMessage("@" + peer + "\ntg_chat_id=" + link);
                            cb.setPositiveButton(getString(R.string.OK), null);
                            cb.show();
                        });
                    } catch (Exception ignore) {
                    }
                });
            });
            builder.setNegativeButton(getString(R.string.MediaFeedCancel), (dialog, which) -> {
                Utilities.stageQueue.postRunnable(() -> {
                    try {
                        MediaFeedServerApi.getInstance().cancelOrder((int) it.orderId);
                    } catch (Exception ignore) {
                    }
                });
            });
            builder.show();
        }

        /** Привязка данных: заголовок в первую строку, подпись — во вторую. */
        void bind(ListItem it) {
            title.setText(it.title);
            subtitle.setText(it.subtitle != null ? it.subtitle : "");
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        // Помечаем Activity уничтоженной, чтобы отбросить поздние UI-вызовы.
        destroyed = true;
    }
}