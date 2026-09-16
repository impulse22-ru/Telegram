package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

// parseLimit — парсит query-параметр limit, ограничивая диапазон 1..200.
// При отсутствии/некорректном значении возвращает дефолт 50.
func parseLimit(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 || n > 200 {
		return 50
	}
	return n
}

// parseOffset — парсит query-параметр offset для пагинации.
// Отрицательные и некорректные значения схлопываются в 0.
func parseOffset(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// handleShopCreate — POST /v1/shop. Создание магазина владельцем (authMW).
// tg_chat_id=0 означает, что канал нужно создать через бота (createNewChannel).
// Канал регистрируется в Channels как тип "shop" для индексации товаров.
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
	// Если клиент не передал существующий канал — создаём его через Bot API.
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

// handleCatalog — GET /v1/catalog. Каталог активных товаров (authMW).
// Параметры: limit (дефолт — из БД), category (фильтр по категории, опционален).
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

// handleCatalogCategories — GET /v1/catalog/categories. Уникальные категории
// активных товаров (для чипов каталога на клиенте; authMW).
func (s *Server) handleCatalogCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := s.repos.Products.Categories(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Защита от nil: клиент ждёт JSON-массив.
	if cats == nil {
		cats = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

// handleShopsList — GET /v1/shops. Список всех магазинов (authMW, без пагинации).
func (s *Server) handleShopsList(w http.ResponseWriter, r *http.Request) {
	shops, err := s.repos.Shops.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
}

// handleShopsSearch — GET /v1/shops/search?q=... Поиск магазинов по названию (authMW).
// q обязателен, иначе 400.
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

// handleShopUpdate — PUT /v1/shop/{id}. Обновление полей магазина владельцем
// (authMW; 403 для чужих магазинов — проверка shop.OwnerID против claims.UserID).
func (s *Server) handleShopUpdate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	// Магазин может менять только его владелец: запрос чужого магазина → 403.
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

// handleShopDelete — DELETE /v1/shop/{id}. Удаление магазина владельцем
// (authMW; 403 для чужих магазинов — проверка владельца через shop.OwnerID).
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

// handleProductCreate — POST /v1/product. Создание товара в своём магазине
// (authMW; проверка: магазин должен принадлежать текущему пользователю).
// Валюта по умолчанию RUB, статус on_sale.
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
	// Можно создавать товары только в своих магазинах (проверка владельца).
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

// handleOrderCreate — POST /v1/order. Оформление заказа покупателем (authMW).
// Количество по умолчанию 1; итоговая сумма = цена товара × количество.
// После создания отправляет уведомление продавцу через бота.
func (s *Server) handleOrderCreate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	var req struct {
		ProductID      int64  `json:"product_id"`
		Quantity       int    `json:"quantity"`
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
		ShopID:         product.ShopID,
		BuyerID:        claims.UserID,
		ProductID:      product.ID,
		Quantity:       req.Quantity,
		PriceAmount:    product.PriceAmount * float64(req.Quantity),
		PriceCurrency:  product.PriceCurrency,
		PaymentStatus:  "pending",
		ContactDetails: req.ContactDetails,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Собираем объект заказа повторно, чтобы передать в notifyOrder без повторного чтения из БД.
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

// handleOrderGet — GET /v1/order/{id}. Просмотр заказа (authMW).
// Доступ: покупатель, владелец магазина или админ; прочие → 403.
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
	// Доступ только участникам сделки (покупатель/владелец магазина) или админу.
	shop, err := s.repos.Shops.Get(r.Context(), order.ShopID)
	if err == nil {
		if order.BuyerID != claims.UserID && shop.OwnerID != claims.UserID && claims.Role != "admin" {
			writeErr(w, http.StatusForbidden, "no access")
			return
		}
	}
	writeJSON(w, http.StatusOK, order)
}

// handleOrderConfirm — POST /v1/order/{id}/confirm. Подтверждение заказа продавцом
// (authMW; только владелец магазина). При подтверждении фиксируется канал связи
// (shop.TgChatID) для диалога покупатель↔продавец, затем шлются уведомления.
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
	// Подтверждать может только владелец магазина, которому принадлежит заказ.
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

// handleOrderChat — GET /v1/order/{id}/chat. Канал связи заказа (покупатель↔продавец).
// Доступ только участникам или админу. Если ссылка не была зафиксирована при
// подтверждении — фолбэк на shop.TgChatID. Возвращает данные собеседника и
// платёжную информацию продавца.
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
	// Фолбэк: если заказ ещё не подтверждён, ссылка не зафиксирована —
	// используем канал магазина как витрину для связи.
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
		// Если у партнёра нет имени — показываем его роль.
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

// handleOrdersSeller — GET /v1/orders/seller. Заказы по всем магазинам продавца
// (authMW). Собирает заказы из каждого магазина владельца и объединяет в один список.
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

// handleOrdersMine — GET /v1/orders/me. Заказы текущего покупателя (authMW).
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

// handleShopsMine — GET /v1/shops/me. Мои магазины (authMW).
func (s *Server) handleShopsMine(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	shops, err := s.repos.Shops.ListForOwner(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
}

// handleShopGet — GET /v1/shop/{id}. Витрина магазина с товарами (authMW).
// Дополнительно возвращает флаг subscribed — подписан ли текущий пользователь
// на канал магазина (если канал удалось найти по tg_chat_id).
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

// handleShopSubscribe — POST /v1/shops/{id}/subscribe. Подписка на канал магазина
// (authMW). Канал регистрируется с типом "shop" при необходимости (EnsureByTgChatID).
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

// handleShopUnsubscribe — DELETE /v1/shops/{id}/subscribe. Отписка от канала магазина
// (authMW). Аналогично подписке — убеждается, что канал существует.
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

// handleMySubscriptions — GET /v1/me/subscriptions. Мои подписки на каналы (authMW).
// Возвращает просто список ID каналов (channel_ids), без объектов каналов.
func (s *Server) handleMySubscriptions(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	channelIDs, err := s.repos.Subscriptions.ByUser(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel_ids": channelIDs})
}

// handleProductGet — GET /v1/product/{id}. Получение товара и количества просмотров
// (authMW). Просматривать могут все авторизованные пользователи.
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

// handleProductView — POST /v1/product/{id}/view. Зафиксировать просмотр товара (authMW).
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

// handleProductUpdate — PUT /v1/product/{id}. Обновление товара владельцем магазина
// (authMW; 403 если товар принадлежит не текущему пользователю — проверка через shop.OwnerID).
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
	// Только владелец магазина, в котором находится товар, может его менять.
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

// handleProductDelete — DELETE /v1/product/{id}. Удаление товара владельцем магазина
// (authMW; 403 для продуктов из чужих магазинов).
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

// handleOrderPay — POST /v1/order/{id}/pay. Покупатель отметил оплату заказа
// (authMW; 403 если заказ не принадлежит текущему пользователю). После оплаты шлёт
// уведомления участникам через бота.
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
	// Оплатить может только покупатель (владелец заказа).
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

// handleOrderCancel — POST /v1/order/{id}/cancel. Отмена заказа покупателем
// (authMW; 403 если заказ не принадлежит текущему пользователю).
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
	// Отменять заказ может только его покупатель.
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
// При отсутствии бота или любой ошибке загрузки данных — тихо выходит (best-effort):
// уведомления не должны ломать основной запрос.
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
