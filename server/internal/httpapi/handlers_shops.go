package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

func parseLimit(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 200 {
		return 50
	}
	return n
}

func parseOffset(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func (s *Server) handleShopCreate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	var req struct {
		TgChatID    int64  `json:"tg_chat_id"` // 0 = создать канал через бота
		Title       string `json:"title"`
		Description string `json:"description"`
		PaymentInfo string `json:"payment_info"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	ctx := r.Context()
	tgChatID := req.TgChatID
	if tgChatID == 0 {
		if s.bot == nil {
			writeErr(w, http.StatusServiceUnavailable, "bot not configured")
			return
		}
		createdID, err := s.bot.CreateChannel(req.Title, req.Description)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "bot api: createNewChannel failed")
			return
		}
		tgChatID = createdID
	}
	id, err := s.repos.Shops.Create(ctx, store.Shop{
		OwnerID:     claims.UserID,
		TgChatID:    tgChatID,
		Title:       req.Title,
		Description: req.Description,
		PaymentInfo: req.PaymentInfo,
		Status:      "active",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// регистрируем канал как shop для индексации товаров
	_, err = s.repos.Channels.EnsureByTgChatID(ctx, tgChatID, "shop", req.Title)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "tg_chat_id": tgChatID})
}

func (s *Server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	category := r.URL.Query().Get("category")
	items, err := s.repos.Products.ListAllActive(r.Context(), limit, category)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleShopsList(w http.ResponseWriter, r *http.Request) {
	shops, err := s.repos.Shops.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
}

func (s *Server) handleShopsSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q required")
		return
	}
	shops, err := s.repos.Shops.Search(r.Context(), q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
}

func (s *Server) handleShopUpdate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), id)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your shop")
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		PaymentInfo string `json:"payment_info"`
		ImageURL    string `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := s.repos.Shops.Update(r.Context(), id, req.Title, req.Description, req.PaymentInfo, req.ImageURL); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (s *Server) handleShopDelete(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), id)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your shop")
		return
	}
	if err := s.repos.Shops.Delete(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleProductCreate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	var req struct {
		ShopID   int64   `json:"shop_id"`
		TgMsgID  int64   `json:"tg_msg_id"`
		FileID   string  `json:"file_id"`
		Title    string  `json:"title"`
		Desc     string  `json:"description"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
		Category string  `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), req.ShopID)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your shop")
		return
	}
	if req.Currency == "" {
		req.Currency = "RUB"
	}
	err = s.repos.Products.Insert(r.Context(), store.Product{
		ShopID:        req.ShopID,
		TgMsgID:       req.TgMsgID,
		FileID:        req.FileID,
		Title:         req.Title,
		Description:   req.Desc,
		PriceAmount:   req.Price,
		PriceCurrency: req.Currency,
		Category:      req.Category,
		Status:        "on_sale",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ok"})
}

func (s *Server) handleOrderCreate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	var req struct {
		ProductID     int64  `json:"product_id"`
		Quantity      int    `json:"quantity"`
		ContactDetails string `json:"contact_details"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.Quantity == 0 {
		req.Quantity = 1
	}
	product, err := s.repos.Products.Get(r.Context(), req.ProductID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "product not found")
		return
	}
	id, err := s.repos.Orders.Create(r.Context(), store.Order{
		ShopID:        product.ShopID,
		BuyerID:       claims.UserID,
		ProductID:     product.ID,
		Quantity:      req.Quantity,
		PriceAmount:   product.PriceAmount * float64(req.Quantity),
		PriceCurrency: product.PriceCurrency,
		PaymentStatus: "pending",
		ContactDetails: req.ContactDetails,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	order := store.Order{
		ID:             id,
		ShopID:         product.ShopID,
		BuyerID:        claims.UserID,
		ProductID:      product.ID,
		Quantity:       req.Quantity,
		PriceAmount:    product.PriceAmount * float64(req.Quantity),
		PriceCurrency:  product.PriceCurrency,
		PaymentStatus:  "pending",
		ContactDetails: req.ContactDetails,
	}
	s.notifyOrder(r.Context(), &order, "created")
	writeJSON(w, http.StatusCreated, map[string]any{"order_id": id})
}

func (s *Server) handleOrderGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	order, err := s.repos.Orders.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	claims := claimsFrom(r.Context())
	shop, err := s.repos.Shops.Get(r.Context(), order.ShopID)
	if err == nil {
		if order.BuyerID != claims.UserID && shop.OwnerID != claims.UserID && claims.Role != "admin" {
			writeErr(w, http.StatusForbidden, "no access")
			return
		}
	}
	writeJSON(w, http.StatusOK, order)
}

func (s *Server) handleOrderConfirm(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	order, err := s.repos.Orders.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), order.ShopID)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your order")
		return
	}
	if err := s.repos.Orders.SetStatus(r.Context(), id, "confirmed"); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Фиксируем канал связи между участниками заказа.
	if shop.TgChatID > 0 {
		_, _ = s.repos.Orders.SetChatLink(r.Context(), id, shop.TgChatID)
	}
	s.notifyOrder(r.Context(), order, "confirmed")
	writeJSON(w, http.StatusOK, map[string]any{"status": "confirmed"})
}

// GET /v1/order/{id}/chat — канал связи заказа (покупатель↔продавец).
func (s *Server) handleOrderChat(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	order, err := s.repos.Orders.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), order.ShopID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if order.BuyerID != claims.UserID && shop.OwnerID != claims.UserID && claims.Role != "admin" {
		writeErr(w, http.StatusForbidden, "no access")
		return
	}

	tgChatID, exists, err := s.repos.Orders.GetChatLink(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if !exists && shop.TgChatID > 0 {
		tgChatID = shop.TgChatID
		exists = true
	}

	// Партнёр для диалога: покупатель видит продавца, продавец — покупателя.
	peerID := order.BuyerID
	peerName := "покупатель"
	if claims.UserID == order.BuyerID {
		peerID = shop.OwnerID
		peerName = "продавец"
	}
	peer, err := s.repos.Users.Get(r.Context(), peerID)
	peerTgID := int64(0)
	peerDisplay := peerName
	if err == nil {
		peerTgID = peer.TgUserID
		if peer.Name != "" {
			peerDisplay = peer.Name
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tg_chat_id":   tgChatID,
		"has_chat":     exists,
		"peer_role":    peerName,
		"peer_name":    peerDisplay,
		"peer_tg_id":   peerTgID,
		"payment_info": shop.PaymentInfo,
		"status":       order.PaymentStatus,
	})
}

func (s *Server) handleOrdersSeller(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	limit := parseLimit(r.URL.Query().Get("limit"))
	offset := parseOffset(r.URL.Query().Get("offset"))
	shops, err := s.repos.Shops.ListForOwner(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := []store.Order{}
	for _, shop := range shops {
		orders, err := s.repos.Orders.ByShop(r.Context(), shop.ID, limit, offset)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		out = append(out, orders...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": out})
}

// GET /v1/orders/me — заказы текущего покупателя.
func (s *Server) handleOrdersMine(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	limit := parseLimit(r.URL.Query().Get("limit"))
	offset := parseOffset(r.URL.Query().Get("offset"))
	orders, err := s.repos.Orders.ByBuyer(r.Context(), claims.UserID, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": orders})
}

// GET /v1/shops/me — мои магазины.
func (s *Server) handleShopsMine(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	shops, err := s.repos.Shops.ListForOwner(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
}

// GET /v1/shop/{id} — витрина магазина с товарами.
func (s *Server) handleShopGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	products, err := s.repos.Products.ListByShop(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	subscribed := false
	if claims := claimsFrom(r.Context()); claims != nil {
		if ch, err := s.repos.Channels.GetByTgChatID(r.Context(), shop.TgChatID); err == nil {
			subscribed, _ = s.repos.Subscriptions.IsSubscribed(r.Context(), claims.UserID, ch.ID)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"shop": shop, "products": products, "subscribed": subscribed})
}

// POST /v1/shops/{id}/subscribe
func (s *Server) handleShopSubscribe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	shopID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), shopID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	channelID, err := s.repos.Channels.EnsureByTgChatID(r.Context(), shop.TgChatID, "shop", shop.Title)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := s.repos.Subscriptions.Subscribe(r.Context(), claims.UserID, channelID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscribed": true})
}

// DELETE /v1/shops/{id}/subscribe
func (s *Server) handleShopUnsubscribe(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	shopID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), shopID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	channelID, err := s.repos.Channels.EnsureByTgChatID(r.Context(), shop.TgChatID, "shop", shop.Title)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if err := s.repos.Subscriptions.Unsubscribe(r.Context(), claims.UserID, channelID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unsubscribed": true})
}

// GET /v1/me/subscriptions — мои подписки на каналы.
func (s *Server) handleMySubscriptions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	channelIDs, err := s.repos.Subscriptions.ByUser(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel_ids": channelIDs})
}

// GET /v1/product/{id}
func (s *Server) handleProductGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	product, err := s.repos.Products.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	views, err := s.repos.Products.CountViews(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"product": product, "views": views})
}

// POST /v1/product/{id}/view
func (s *Server) handleProductView(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Products.RecordView(r.Context(), claims.UserID, id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"viewed": true})
}

func (s *Server) handleProductUpdate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	p, err := s.repos.Products.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no product")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), p.ShopID)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your shop")
		return
	}
	var req struct {
		Title       string  `json:"title"`
		Description string  `json:"description"`
		Price       float64 `json:"price"`
		Currency    string  `json:"currency"`
		Category    string  `json:"category"`
		ImageURL    string  `json:"image_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if err := s.repos.Products.Update(r.Context(), id, req.Title, req.Description, req.Price, req.Currency, req.Category, req.ImageURL); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

func (s *Server) handleProductDelete(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	p, err := s.repos.Products.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no product")
		return
	}
	shop, err := s.repos.Shops.Get(r.Context(), p.ShopID)
	if err != nil || shop.OwnerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your shop")
		return
	}
	if err := s.repos.Products.Delete(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// POST /v1/order/{id}/pay — покупатель отметил оплату.
func (s *Server) handleOrderPay(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	order, err := s.repos.Orders.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if order.BuyerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your order")
		return
	}
	if err := s.repos.Orders.SetStatus(r.Context(), id, "paid"); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	s.notifyOrder(r.Context(), order, "paid")
	writeJSON(w, http.StatusOK, map[string]any{"status": "paid"})
}

// POST /v1/order/{id}/cancel — отмена покупателем.
func (s *Server) handleOrderCancel(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	order, err := s.repos.Orders.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if order.BuyerID != claims.UserID {
		writeErr(w, http.StatusForbidden, "not your order")
		return
	}
	if err := s.repos.Orders.SetStatus(r.Context(), id, "cancelled"); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled"})
}

// notifyOrder — шлёт уведомления продавцу/покупателю через бота (этап 7).
func (s *Server) notifyOrder(ctx context.Context, order *store.Order, status string) {
	if s.bot == nil {
		return
	}
	shop, err := s.repos.Shops.Get(ctx, order.ShopID)
	if err != nil {
		return
	}
	seller, err := s.repos.Users.Get(ctx, shop.OwnerID)
	if err != nil {
		return
	}
	buyer, err := s.repos.Users.Get(ctx, order.BuyerID)
	if err != nil {
		return
	}
	msg := "Заказ #" + strconv.FormatInt(order.ID, 10) + ": " + status
	_ = s.bot.SendMessage(seller.TgUserID, msg)
	_ = s.bot.SendMessage(buyer.TgUserID, msg)
}