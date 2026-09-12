package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

func (s *Server) handleShopCreate(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	var req struct {
		TgChatID    int64  `json:"tg_chat_id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		PaymentInfo string `json:"payment_info"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.TgChatID == 0 || req.Title == "" {
		writeErr(w, http.StatusBadRequest, "tg_chat_id and title required")
		return
	}
	id, err := s.repos.Shops.Create(r.Context(), store.Shop{
		OwnerID:     claims.UserID,
		TgChatID:    req.TgChatID,
		Title:       req.Title,
		Description: req.Description,
		PaymentInfo: req.PaymentInfo,
		Status:      "active",
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func (s *Server) handleShopsList(w http.ResponseWriter, r *http.Request) {
	shops, err := s.repos.Shops.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"shops": shops})
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
	writeJSON(w, http.StatusOK, map[string]any{"status": "confirmed"})
}

func (s *Server) handleOrdersSeller(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())

	// Все заказы магазинов, где продавец — текущий пользователь.
	shops, err := s.repos.Shops.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	out := []store.Order{}
	for _, shop := range shops {
		if shop.OwnerID != claims.UserID {
			continue
		}
		orders, err := s.repos.Orders.ByShop(r.Context(), shop.ID)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "db error")
			return
		}
		out = append(out, orders...)
	}
	writeJSON(w, http.StatusOK, map[string]any{"orders": out})
}