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
	Banned    bool      `json:"banned"`
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
	ImageURL    string `json:"image_url"`
	Status      string `json:"status"`
}

type Product struct {
	ID            int64     `json:"id"`
	ShopID        int64     `json:"shop_id"`
	TgMsgID       int64     `json:"tg_msg_id"`
	FileID        string    `json:"file_id"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	PriceAmount   float64   `json:"price_amount"`
	PriceCurrency string    `json:"price_currency"`
	Category      string    `json:"category"`
	ImageURL      string    `json:"image_url"`
	Status        string    `json:"status"`
	PostedAt      time.Time `json:"posted_at"`
}

type Order struct {
	ID             int64     `json:"id"`
	ShopID         int64     `json:"shop_id"`
	BuyerID        int64     `json:"buyer_id"`
	ProductID      int64     `json:"product_id"`
	Quantity       int       `json:"quantity"`
	PriceAmount    float64   `json:"price_amount"`
	PriceCurrency  string    `json:"price_currency"`
	PaymentStatus  string    `json:"payment_status"`
	ContactDetails string    `json:"contact_details"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Comment struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	Text      string    `json:"text"`
	ParentID  int64     `json:"parent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Report struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	Reason    string    `json:"reason"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ViewDay — сводка просмотров за одни сутки.
type ViewDay struct {
	Day     time.Time `json:"day"`
	Views   int64     `json:"views"`
	Uniques int64     `json:"unique_viewers"`
}

// VideoStat — аналитика по одному видео / топ видео.
type VideoStat struct {
	VideoID int64  `json:"video_id"`
	Views   int64  `json:"views"`
	Uniques int64  `json:"unique_viewers"`
	Likes   int64  `json:"likes"`
	AvgWatchSeconds float64 `json:"avg_watch_seconds"`
}

// AdminStats — сводка для дашборда (этап 3).
type AdminStats struct {
	Users        int64   `json:"users"`
	Videos       int64   `json:"videos"`
	VisibleVideos int64  `json:"visible_videos"`
	Views        int64   `json:"views"`
	Likes        int64   `json:"likes"`
	Comments     int64   `json:"comments"`
	ReportsOpen  int64   `json:"reports_open"`
	Shops        int64   `json:"shops"`
	Products     int64   `json:"products"`
	Orders       int64   `json:"orders"`
	Revenue      float64 `json:"revenue"`
}

// SellerStats — аналитика продавца (этап 7).
type SellerStats struct {
	Orders        int64   `json:"orders"`
	Pending       int64   `json:"pending"`
	Confirmed     int64   `json:"confirmed"`
	Revenue       float64 `json:"revenue"`
	ByProduct     []ProductStat `json:"by_product"`
	Views         int64   `json:"product_views"`
}

type ProductStat struct {
	ProductID int64   `json:"product_id"`
	Title     string  `json:"title"`
	Views     int64   `json:"views"`
	Orders    int64   `json:"orders"`
	Revenue   float64 `json:"revenue"`
}

// UserStats — персональная статистика пользователя (/v1/stats/me).
type UserStats struct {
	Views         int64   `json:"views"`
	WatchSeconds  int64   `json:"watch_seconds"`
	LikesGiven    int64   `json:"likes_given"`
	CommentsGiven int64   `json:"comments_given"`
	Subscriptions int64   `json:"subscriptions"`
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
	SetRole(ctx context.Context, id int64, role string) error
	SetBanned(ctx context.Context, id int64, banned bool) error
}

type Videos interface {
	Insert(ctx context.Context, v Video) error
	VisibleFrom(ctx context.Context, channelID, after, limit int64) ([]Video, error)
	Search(ctx context.Context, q string, limit int64) ([]Video, error)
	GetByTgMsg(ctx context.Context, channelID, tgMsgID int64) (*Video, error)
	Ban(ctx context.Context, id int64) error
	Unban(ctx context.Context, id int64) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*Video, error)
	Count(ctx context.Context, status string) (int64, error)
}

type Shops interface {
	Create(ctx context.Context, s Shop) (int64, error)
	List(ctx context.Context) ([]Shop, error)
	ListForOwner(ctx context.Context, ownerID int64) ([]Shop, error)
	Get(ctx context.Context, id int64) (*Shop, error)
	GetByTgChatID(ctx context.Context, tgChatID int64) (*Shop, error)
	Update(ctx context.Context, id int64, title, description, paymentInfo, imageURL string) error
	Delete(ctx context.Context, id int64) error
	Search(ctx context.Context, q string) ([]Shop, error)
	Suspend(ctx context.Context, id int64) error
}

type Products interface {
	Insert(ctx context.Context, p Product) error
	ListByShop(ctx context.Context, shopID int64) ([]Product, error)
	ListAllActive(ctx context.Context, limit int64, category string) ([]Product, error)
	Get(ctx context.Context, id int64) (*Product, error)
	Update(ctx context.Context, id int64, title, description string, price float64, currency, category, imageURL string) error
	Delete(ctx context.Context, id int64) error
	RecordView(ctx context.Context, userID, productID int64) error
	CountViews(ctx context.Context, productID int64) (int64, error)
	Hide(ctx context.Context, id int64) error
}

type Orders interface {
	Create(ctx context.Context, o Order) (int64, error)
	Get(ctx context.Context, id int64) (*Order, error)
	ByBuyer(ctx context.Context, buyerID int64, limit, offset int64) ([]Order, error)
	ByShop(ctx context.Context, shopID int64, limit, offset int64) ([]Order, error)
	SetStatus(ctx context.Context, id int64, status string) error
	SetChatLink(ctx context.Context, orderID, tgChatID int64) (int64, error)
	GetChatLink(ctx context.Context, orderID int64) (int64, bool, error)
}

type Engagements interface {
	Like(ctx context.Context, userID, videoID int64) error
	Unlike(ctx context.Context, userID, videoID int64) error
	IsLiked(ctx context.Context, userID, videoID int64) (bool, error)
	CountLikes(ctx context.Context, videoID int64) (int64, error)
	Comment(ctx context.Context, userID, videoID int64, text string, parentID int64) (int64, error)
	Comments(ctx context.Context, videoID int64, limit int64) ([]Comment, error)
	CommentsAll(ctx context.Context, limit int64) ([]Comment, error)
	CountComments(ctx context.Context, videoID int64) (int64, error)
	DeleteComment(ctx context.Context, commentID int64) error
	Report(ctx context.Context, userID, videoID int64, reason string) error
	Reports(ctx context.Context, status string, limit int64) ([]Report, error)
	ReportResolve(ctx context.Context, id int64, status string) error
	RecordView(ctx context.Context, userID, videoID int64, watchSeconds int) error
	CountViews(ctx context.Context, videoID int64) (int64, error)
}

type Feed interface {
	AddVideo(ctx context.Context, channelID, videoID int64, score float64) error
	Top(ctx context.Context, channelID, n int64) ([]int64, error)
	ScoredFeed(ctx context.Context, channelID, offset, limit int64) ([]int64, error)
	RemoveVideo(ctx context.Context, channelID, videoID int64) error
}

type Stats interface {
	VideoViewsDay(ctx context.Context, videoID int64, days int) ([]ViewDay, error)
	VideoStat(ctx context.Context, videoID int64) (*VideoStat, error)
	TopVideos(ctx context.Context, limit int64) ([]VideoStat, error)
	AdminStats(ctx context.Context) (*AdminStats, error)
	SellerStats(ctx context.Context, ownerID int64) (*SellerStats, error)
	UserStats(ctx context.Context, userID int64) (*UserStats, error)
}

type Subscriptions interface {
	Subscribe(ctx context.Context, userID, channelID int64) error
	Unsubscribe(ctx context.Context, userID, channelID int64) error
	IsSubscribed(ctx context.Context, userID, channelID int64) (bool, error)
	ByUser(ctx context.Context, userID int64) ([]int64, error)
}

type Filter interface {
	Add(ctx context.Context, word string) error
	List(ctx context.Context) ([]string, error)
	Remove(ctx context.Context, word string) error
}

type Review struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	ProductID int64     `json:"product_id"`
	Rating    int       `json:"rating"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}

type Reviews interface {
	Add(ctx context.Context, r Review) error
	ByProduct(ctx context.Context, productID int64) ([]Review, error)
	AvgRating(ctx context.Context, productID int64) (float64, int64, error)
}

type Repos struct {
	Channels      Channels
	Users         Users
	Videos        Videos
	Shops         Shops
	Products      Products
	Orders        Orders
	Engagements   Engagements
	Feed          Feed
	Stats         Stats
	Subscriptions Subscriptions
	Filter        Filter
	Reviews       Reviews
}

func NewRepos(s *Store) *Repos {
	return &Repos{
		Channels:      &channelsRepo{pg: s.PG},
		Users:         &usersRepo{pg: s.PG},
		Videos:        &videosRepo{pg: s.PG},
		Shops:         &shopsRepo{pg: s.PG},
		Products:      &productsRepo{pg: s.PG},
		Orders:        &ordersRepo{pg: s.PG},
		Engagements:   &engagementsRepo{pg: s.PG},
		Feed:          &feedRepo{rdb: s.Redis, pg: s.PG},
		Stats:         &statsRepo{pg: s.PG},
		Subscriptions: &subscriptionsRepo{pg: s.PG},
		Filter:        &filterRepo{pg: s.PG},
		Reviews:       &reviewsRepo{pg: s.PG},
	}
}