package org.telegram.ui;

import android.app.Activity;
import android.os.Bundle;
import android.text.InputType;
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

// Витрина/каталог: магазины → товары → заказ (этапы 6–7).
public class MediaCatalogActivity extends Activity {

    private final int currentAccount = UserConfig.selectedAccount;
    private boolean destroyed;
    private RecyclerView list;
    private LinearLayoutManager layoutManager;
    private CatalogAdapter adapter;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        requestWindowFeature(Window.FEATURE_NO_TITLE);
        setTheme(R.style.Theme_TMessages);
        super.onCreate(savedInstanceState);

        FrameLayout root = new FrameLayout(this);
        root.setBackgroundColor(Theme.getColor(Theme.key_windowBackgroundWhite));

        // Заголовок с кнопками: магазины / мои заказы.
        LinearLayout header = new LinearLayout(this);
        header.setOrientation(LinearLayout.HORIZONTAL);
        header.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(8), AndroidUtilities.dp(12), AndroidUtilities.dp(8));

        TextView title = new TextView(this);
        title.setText(getString(R.string.MediaFeed));
        title.setTextSize(18f);
        title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
        header.addView(title, new LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f));

        TextView myOrders = button("Мои заказы", v -> showOrders(false));
        TextView mySales = button("Мои продажи", v -> showOrders(true));
        header.addView(myOrders);
        header.addView(mySales);

        root.addView(header, LayoutHelper.createFrame(LayoutHelper.MATCH_PARENT, LayoutHelper.WRAP_CONTENT, Gravity.TOP));

        list = new RecyclerView(this);
        layoutManager = new LinearLayoutManager(this);
        list.setLayoutManager(layoutManager);
        adapter = new CatalogAdapter();
        list.setAdapter(adapter);
        root.addView(list, LayoutHelper.createFrameMarginPx(LayoutHelper.MATCH_PARENT, LayoutHelper.MATCH_PARENT, Gravity.TOP, 0, AndroidUtilities.dp(52), 0, 0));

        RadialProgressView loading = new RadialProgressView(this);
        root.addView(loading, LayoutHelper.createFrame(48, 48, Gravity.CENTER));
        setContentView(root);

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
    }

    private TextView button(String text, View.OnClickListener onClick) {
        TextView tv = new TextView(this);
        tv.setText(text);
        tv.setTextSize(14f);
        tv.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlueText4));
        tv.setPadding(AndroidUtilities.dp(8), AndroidUtilities.dp(4), AndroidUtilities.dp(8), AndroidUtilities.dp(4));
        tv.setOnClickListener(onClick);
        return tv;
    }

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
                adapter.setOrders(orders, mine);
            });
        });
    }

    private class CatalogAdapter extends RecyclerView.Adapter<CatalogHolder> {

        private final ArrayList<ListItem> items = new ArrayList<>();
        private JSONArray shops;
        private JSONArray products;

        void setShops(JSONArray list) {
            this.shops = list;
            this.products = null;
            rebuild();
        }

        void setShopProducts(JSONObject shopResp) {
            shops = null;
            products = shopResp.optJSONArray("products");
            ListItem shopItem = new ListItem();
            shopItem.head = true;
            shopItem.title = shopResp.optJSONObject("shop") != null
                    ? shopResp.optJSONObject("shop").optString("title") : "Магазин";
            shopItem.subtitle = "👁 " + (shopResp.optJSONObject("shop") != null
                    ? shopResp.optJSONObject("shop").optString("payment_info") : "");
            rebuildFromShop(shopItem);
        }

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
                    it.subtitle = o.optString("payment_status")
                            + " — " + o.optDouble("price_amount", 0) + " "
                            + o.optString("price_currency");
                    it.extra = mine;
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        void rebuild() {
            items.clear();
            if (shops != null) {
                for (int i = 0; i < shops.length(); i++) {
                    JSONObject s = shops.optJSONObject(i);
                    if (s == null) continue;
                    ListItem it = new ListItem();
                    it.shopId = s.optLong("id");
                    it.title = s.optString("title");
                    it.subtitle = "💰 " + s.optString("payment_info") + "\n" + s.optString("status");
                    it.head = true;
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

        void rebuildFromShop(ListItem shopItem) {
            items.clear();
            items.add(shopItem);
            if (products != null) {
                for (int i = 0; i < products.length(); i++) {
                    JSONObject p = products.optJSONObject(i);
                    if (p == null) continue;
                    ListItem it = new ListItem();
                    it.productId = p.optLong("id");
                    it.title = p.optString("title");
                    it.subtitle = "💰 " + p.optDouble("price_amount", 0) + " "
                            + p.optString("price_currency") + "\n" + p.optString("description");
                    items.add(it);
                }
            }
            notifyDataSetChanged();
        }

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

        void openProduct(ListItem p) {
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            LinearLayout content = new LinearLayout(MediaCatalogActivity.this);
            content.setOrientation(LinearLayout.VERTICAL);
            TextView info = new TextView(MediaCatalogActivity.this);
            info.setTextSize(15f);
            info.setText(p.title + "\n" + p.subtitle);
            info.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            content.addView(info, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            final EditText contact = new EditText(MediaCatalogActivity.this);
            contact.setHint(getString(R.string.MediaFeedContact));
            contact.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            contact.setHintTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteHintText));
            content.addView(contact, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

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
                        MediaFeedServerApi.getInstance().createOrder((int) p.productId, q,
                                priceOf(p), contactText);
                        AndroidUtilities.runOnUIThread(() -> {
                            if (!destroyed) {
                                showOrders(false);
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
            holder.bind(items.get(position));
        }

        @Override
        public int getItemCount() {
            return items.size();
        }
    }

    private class ListItem {
        long shopId;
        long productId;
        long orderId;
        boolean head;
        String title;
        String subtitle;
        boolean extra; // true = заказ продавца (можно подтвердить)
    }

    private class CatalogHolder extends RecyclerView.ViewHolder {

        private final TextView title;
        private final TextView subtitle;

        CatalogHolder(ViewGroup parent) {
            super(new LinearLayout(parent.getContext()));
            LinearLayout row = (LinearLayout) itemView;
            row.setOrientation(LinearLayout.VERTICAL);
            row.setPadding(AndroidUtilities.dp(12), AndroidUtilities.dp(10), AndroidUtilities.dp(12), AndroidUtilities.dp(10));
            row.setLayoutParams(new RecyclerView.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            title = new TextView(row.getContext());
            title.setTextSize(16f);
            title.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            row.addView(title, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            subtitle = new TextView(row.getContext());
            subtitle.setTextSize(13f);
            subtitle.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteGrayText));
            row.addView(subtitle, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT));

            row.setOnClickListener(v -> onClick());
        }

        private void onClick() {
            int pos = getAdapterPosition();
            if (pos < 0 || pos >= adapter.items.size()) {
                return;
            }
            ListItem it = adapter.items.get(pos);
            if (it.shopId != 0) {
                adapter.openShop(it.shopId);
            } else if (it.productId != 0) {
                adapter.openProduct(it);
            } else if (it.orderId != 0) {
                openOrderActions(it);
            }
        }

        private void openOrderActions(final ListItem it) {
            final AlertDialog.Builder builder = new AlertDialog.Builder(MediaCatalogActivity.this);
            TextView info = new TextView(MediaCatalogActivity.this);
            info.setTextSize(15f);
            info.setTextColor(Theme.getColor(Theme.key_windowBackgroundWhiteBlackText));
            info.setText(it.title + "\n" + it.subtitle);
            builder.setView(info);
            builder.setTitle(getString(R.string.MediaFeedOrders));

            if (it.extra) {
                builder.setPositiveButton(getString(R.string.MediaFeedConfirm), (dialog, which) -> {
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().confirmOrder((int) it.orderId);
                        } catch (Exception ignore) {
                        }
                    });
                });
            } else {
                builder.setPositiveButton(getString(R.string.MediaFeedPaid), (dialog, which) -> {
                    Utilities.stageQueue.postRunnable(() -> {
                        try {
                            MediaFeedServerApi.getInstance().payOrder((int) it.orderId);
                        } catch (Exception ignore) {
                        }
                    });
                });
            }
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

        void bind(ListItem it) {
            title.setText(it.title);
            subtitle.setText(it.subtitle != null ? it.subtitle : "");
        }
    }

    @Override
    protected void onDestroy() {
        super.onDestroy();
        destroyed = true;
    }
}