package store

import (
	"context"
	"time"
)

type User struct {
	ID        int64     `json:"id"`
	TgUserID  int64     `json:"tg_user_id"`
	Phone     string    `json:"phone"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

type Channel struct {
	ID        int64     `json:"id"`
	TgChatID  int64     `json:"tg_chat_id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	IsPrivate bool      `json:"is_private"`
	OwnerID   int64     `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Video struct {
	ID         int64    `json:"id"`
	TgMsgID    int64    `json:"tg_msg_id"`
	FileID     string   `json:"file_id"`
	Caption    string   `json:"caption"`
	DurationMs int      `json:"duration_ms"`
	Width      int      `json:"width"`
	Height     int      `json:"height"`
	Title      string   `json:"title"`
	Tags       []string `json:"tags"`
	Status     string   `json:"status"`
	ChannelID  int64    `json:"channel_id"`
	PostedAt   time.Time `json:"posted_at"`
}

type Shop struct {
	ID          int64  `json:"id"`
	OwnerID     int64  `json:"owner_id"`
	TgChatID    int64  `json:"tg_chat_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	PaymentInfo string `json:"payment_info"`
	Status      string `json:"status"`
}

type Product struct {
	ID            int64   `json:"id"`
	ShopID        int64   `json:"shop_id"`
	TgMsgID       int64   `json:"tg_msg_id"`
	FileID        string  `json:"file_id"`
	Title         string  `json:"title"`
	Description   string  `json:"description"`
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	Category      string  `json:"category"`
	Status        string  `json:"status"`
}

type Order struct {
	ID            int64   `json:"id"`
	ShopID        int64   `json:"shop_id"`
	BuyerID       int64   `json:"buyer_id"`
	ProductID     int64   `json:"product_id"`
	Quantity      int     `json:"quantity"`
	PriceAmount   float64 `json:"price_amount"`
	PriceCurrency string  `json:"price_currency"`
	PaymentStatus string  `json:"payment_status"`
	ContactDetails string `json:"contact_details"`
}

// --- Репозитории (интерфейсы + реализация на pgxpool) ---

type Channels interface {
	EnsureByTgChatID(ctx context.Context, tgChatID int64, kind, title string) (int64, error)
	GetByTgChatID(ctx context.Context, tgChatID int64) (*Channel, error)
}

type Users interface {
	Upsert(ctx context.Context, u User) (int64, error)
	GetByTgID(ctx context.Context, tgID int64) (*User, error)
	Get(ctx context.Context, id int64) (*User, error)
}

type Videos interface {
	Insert(ctx context.Context, v Video) error
	VisibleFrom(ctx context.Context, channelID, after, limit int64) ([]Video, error)
	Ban(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*Video, error)
}

type Shops interface {
	Create(ctx context.Context, s Shop) (int64, error)
	List(ctx context.Context) ([]Shop, error)
	Get(ctx context.Context, id int64) (*Shop, error)
	Suspend(ctx context.Context, id int64) error
}

type Products interface {
	Insert(ctx context.Context, p Product) error
	ListByShop(ctx context.Context, shopID int64) ([]Product, error)
	Get(ctx context.Context, id int64) (*Product, error)
}

type Orders interface {
	Create(ctx context.Context, o Order) (int64, error)
	Get(ctx context.Context, id int64) (*Order, error)
	ByBuyer(ctx context.Context, buyerID int64) ([]Order, error)
	ByShop(ctx context.Context, shopID int64) ([]Order, error)
	SetStatus(ctx context.Context, id int64, status string) error
}

type Repos struct {
	Channels Channels
	Users    Users
	Videos   Videos
	Shops    Shops
	Products Products
	Orders   Orders
}

func NewRepos(s *Store) *Repos {
	return &Repos{
		Channels: &channelsRepo{pg: s.PG},
		Users:    &usersRepo{pg: s.PG},
		Videos:   &videosRepo{pg: s.PG},
		Shops:    &shopsRepo{pg: s.PG},
		Products: &productsRepo{pg: s.PG},
		Orders:   &ordersRepo{pg: s.PG},
	}
}