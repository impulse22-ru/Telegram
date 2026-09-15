package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type shopsRepo struct{ pg *pgxpool.Pool }

func (r *shopsRepo) Create(ctx context.Context, s Shop) (int64, error) {
	var id int64
	if s.Status == "" {
		s.Status = "active"
	}
	err := r.pg.QueryRow(ctx, `
		INSERT INTO shops (owner_id, tg_chat_id, title, description, payment_info, image_url, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		s.OwnerID, s.TgChatID, s.Title, s.Description, s.PaymentInfo, s.ImageURL, s.Status).Scan(&id)
	return id, err
}

func (r *shopsRepo) List(ctx context.Context) ([]Shop, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, owner_id, tg_chat_id, title, COALESCE(description,''),
		       COALESCE(payment_info,''), COALESCE(image_url,''), status FROM shops WHERE status='active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShops(rows)
}

func (r *shopsRepo) ListForOwner(ctx context.Context, ownerID int64) ([]Shop, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, owner_id, tg_chat_id, title, COALESCE(description,''),
		       		COALESCE(payment_info,''), COALESCE(image_url,''), status FROM shops WHERE owner_id=$1 ORDER BY id`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanShops(rows)
}

func scanShops(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]Shop, error) {
	out := make([]Shop, 0)
	for rows.Next() {
		var s Shop
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title,
			&s.Description, &s.PaymentInfo, &s.ImageURL, &s.Status); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *shopsRepo) Get(ctx context.Context, id int64) (*Shop, error) {
	var s Shop
	err := r.pg.QueryRow(ctx, `
		SELECT id, owner_id, tg_chat_id, title, COALESCE(description,''),
		       COALESCE(payment_info,''), COALESCE(image_url,''), status FROM shops WHERE id=$1`, id).
		Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title, &s.Description, &s.PaymentInfo, &s.ImageURL, &s.Status)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetByTgChatID — магазин по tg_chat_id канала-магазина.
func (r *shopsRepo) GetByTgChatID(ctx context.Context, tgChatID int64) (*Shop, error) {
	var s Shop
	err := r.pg.QueryRow(ctx, `
		SELECT id, owner_id, tg_chat_id, title, COALESCE(description,''),
		       COALESCE(payment_info,''), COALESCE(image_url,''), status FROM shops WHERE tg_chat_id=$1`, tgChatID).
		Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title, &s.Description, &s.PaymentInfo, &s.ImageURL, &s.Status)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *shopsRepo) Suspend(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE shops SET status='suspended' WHERE id=$1`, id)
	return err
}

func (r *shopsRepo) Update(ctx context.Context, id int64, title, description, paymentInfo, imageURL string) error {
	_, err := r.pg.Exec(ctx, `
		UPDATE shops SET
			title = COALESCE(NULLIF($2, ''), title),
			description = COALESCE(NULLIF($3, ''), description),
			payment_info = COALESCE(NULLIF($4, ''), payment_info),
			image_url = NULLIF($5, '')
		WHERE id = $1`, id, title, description, paymentInfo, imageURL)
	return err
}

func (r *shopsRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM shops WHERE id=$1`, id)
	return err
}

func (r *shopsRepo) Search(ctx context.Context, q string) ([]Shop, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, owner_id, tg_chat_id, COALESCE(title,''), COALESCE(description,''),
		       COALESCE(payment_info,''), status
		FROM shops
		WHERE title ILIKE '%'||$1||'%' OR description ILIKE '%'||$1||'%'
		ORDER BY id DESC LIMIT 50`, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Shop
	for rows.Next() {
		var s Shop
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title, &s.Description, &s.PaymentInfo, &s.Status); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

var _ Shops = (*shopsRepo)(nil)

type productsRepo struct{ pg *pgxpool.Pool }

func (r *productsRepo) Insert(ctx context.Context, p Product) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO products (shop_id, tg_msg_id, file_id, title, description, price_amount, price_currency, category, image_url, status, posted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		ON CONFLICT (shop_id, tg_msg_id) DO UPDATE SET
			file_id=EXCLUDED.file_id, title=EXCLUDED.title, description=EXCLUDED.description,
			price_amount=EXCLUDED.price_amount, price_currency=EXCLUDED.price_currency,
			category=EXCLUDED.category, image_url=EXCLUDED.image_url, status=EXCLUDED.status, posted_at=EXCLUDED.posted_at`,
		p.ShopID, p.TgMsgID, p.FileID, p.Title, p.Description, p.PriceAmount, p.PriceCurrency, p.Category, p.ImageURL, p.Status, p.PostedAt)
	return err
}

func (r *productsRepo) ListByShop(ctx context.Context, shopID int64) ([]Product, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, shop_id, tg_msg_id, file_id, title, COALESCE(description,''),
		       price_amount, price_currency, COALESCE(category,''), COALESCE(image_url,''), status, posted_at
		FROM products WHERE shop_id=$1 AND status='on_sale' ORDER BY id`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

// ListAllActive — все товары всех активных магазинов (витрина/каталог).
func (r *productsRepo) ListAllActive(ctx context.Context, limit int64, category string) ([]Product, error) {
	if limit <= 0 {
		limit = 50
	}
	if category != "" {
		rows, err := r.pg.Query(ctx, `
		SELECT p.id, p.shop_id, p.tg_msg_id, p.file_id, p.title, COALESCE(p.description,''),
		       p.price_amount, p.price_currency, COALESCE(p.category,''), COALESCE(p.image_url,''), p.status, p.posted_at
		FROM products p JOIN shops s ON s.id=p.shop_id
		WHERE p.status='on_sale' AND s.status='active' AND p.category=$1
		ORDER BY p.id DESC LIMIT $2`, category, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanProducts(rows)
	}
	rows, err := r.pg.Query(ctx, `
		SELECT p.id, p.shop_id, p.tg_msg_id, p.file_id, p.title, COALESCE(p.description,''),
		       p.price_amount, p.price_currency, COALESCE(p.category,''), COALESCE(p.image_url,''), p.status, p.posted_at
		FROM products p JOIN shops s ON s.id=p.shop_id
		WHERE p.status='on_sale' AND s.status='active'
		ORDER BY p.id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanProducts(rows)
}

func scanProducts(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]Product, error) {
	out := make([]Product, 0)
	for rows.Next() {
		var p Product
if err := rows.Scan(&p.ID, &p.ShopID, &p.TgMsgID, &p.FileID, &p.Title, &p.Description,
			&p.PriceAmount, &p.PriceCurrency, &p.Category, &p.ImageURL, &p.Status, &p.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (r *productsRepo) Get(ctx context.Context, id int64) (*Product, error) {
	var p Product
	err := r.pg.QueryRow(ctx, `
		SELECT id, shop_id, tg_msg_id, file_id, title, COALESCE(description,''),
		       price_amount, price_currency, COALESCE(category,''), COALESCE(image_url,''), status
		FROM products WHERE id=$1`, id).
		Scan(&p.ID, &p.ShopID, &p.TgMsgID, &p.FileID, &p.Title,
			&p.Description, &p.PriceAmount, &p.PriceCurrency, &p.Category, &p.ImageURL, &p.Status)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *productsRepo) RecordView(ctx context.Context, userID, productID int64) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO product_views (user_id, product_id) VALUES ($1,$2)`, userID, productID)
	return err
}

func (r *productsRepo) CountViews(ctx context.Context, productID int64) (int64, error) {
	var n int64
	err := r.pg.QueryRow(ctx, `SELECT count(*) FROM product_views WHERE product_id=$1`, productID).Scan(&n)
	return n, err
}

func (r *productsRepo) Hide(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE products SET status='hidden' WHERE id=$1`, id)
	return err
}

func (r *productsRepo) Update(ctx context.Context, id int64, title, description string, price float64, currency, category, imageURL string) error {
	_, err := r.pg.Exec(ctx, `
		UPDATE products SET
			title = COALESCE(NULLIF($2, ''), title),
			description = COALESCE(NULLIF($3, ''), description),
			price_amount = $4,
			price_currency = COALESCE(NULLIF($5, ''), price_currency),
			category = COALESCE(NULLIF($6, ''), category),
			image_url = NULLIF($7, '')
		WHERE id = $1`, id, title, description, price, currency, category, imageURL)
	return err
}

func (r *productsRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM products WHERE id=$1`, id)
return err
}

var _ Products = (*productsRepo)(nil)

type ordersRepo struct{ pg *pgxpool.Pool }

const orderCols = `id, shop_id, buyer_id, product_id, quantity, price_amount, price_currency, payment_status, COALESCE(contact_details,''), created_at, updated_at`

func (r *ordersRepo) Create(ctx context.Context, o Order) (int64, error) {
	var id int64
	if o.PaymentStatus == "" {
		o.PaymentStatus = "pending"
	}
	err := r.pg.QueryRow(ctx, `
		INSERT INTO orders (shop_id, buyer_id, product_id, quantity, price_amount, price_currency, payment_status, contact_details)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		o.ShopID, o.BuyerID, o.ProductID, o.Quantity, o.PriceAmount, o.PriceCurrency, o.PaymentStatus, o.ContactDetails).Scan(&id)
	return id, err
}

func (r *ordersRepo) Get(ctx context.Context, id int64) (*Order, error) {
	var o Order
	err := r.pg.QueryRow(ctx, `SELECT `+orderCols+` FROM orders WHERE id=$1`, id).
		Scan(&o.ID, &o.ShopID, &o.BuyerID, &o.ProductID, &o.Quantity,
			&o.PriceAmount, &o.PriceCurrency, &o.PaymentStatus, &o.ContactDetails, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *ordersRepo) ByBuyer(ctx context.Context, buyerID int64, limit int64, offset int64) ([]Order, error) {
	return r.list(ctx, `
		SELECT `+orderCols+`
		FROM orders WHERE buyer_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, buyerID, limit, offset)
}

func (r *ordersRepo) ByShop(ctx context.Context, shopID int64, limit int64, offset int64) ([]Order, error) {
	return r.list(ctx, `
		SELECT `+orderCols+`
		FROM orders WHERE shop_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3`, shopID, limit, offset)
}

func (r *ordersRepo) list(ctx context.Context, sql string, args ...int64) ([]Order, error) {
	anyArgs := make([]any, len(args))
	for i, a := range args {
		anyArgs[i] = a
	}
	rows, err := r.pg.Query(ctx, sql, anyArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Order, 0)
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.ShopID, &o.BuyerID, &o.ProductID, &o.Quantity,
			&o.PriceAmount, &o.PriceCurrency, &o.PaymentStatus, &o.ContactDetails, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (r *ordersRepo) SetStatus(ctx context.Context, id int64, status string) error {
	_, err := r.pg.Exec(ctx, `UPDATE orders SET payment_status=$2, updated_at=now() WHERE id=$1`, id, status)
	return err
}

// SetChatLink создаёт/обновляет запись order_chat_link для заказа.
func (r *ordersRepo) SetChatLink(ctx context.Context, orderID, tgChatID int64) (int64, error) {
	var id int64
	err := r.pg.QueryRow(ctx, `
		INSERT INTO order_chat_link (order_id, tg_chat_id)
		VALUES ($1,$2)
		ON CONFLICT (order_id) DO UPDATE SET tg_chat_id=EXCLUDED.tg_chat_id
		RETURNING id`, orderID, tgChatID).Scan(&id)
	return id, err
}

// GetChatLink возвращает (tg_chat_id, exists, error) для заказа.
func (r *ordersRepo) GetChatLink(ctx context.Context, orderID int64) (int64, bool, error) {
	var chatID int64
	err := r.pg.QueryRow(ctx, `SELECT tg_chat_id FROM order_chat_link WHERE order_id=$1`, orderID).Scan(&chatID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return chatID, true, nil
}

var _ Orders = (*ordersRepo)(nil)