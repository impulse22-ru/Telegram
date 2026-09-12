package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type shopsRepo struct{ pg *pgxpool.Pool }

func (r *shopsRepo) Create(ctx context.Context, s Shop) (int64, error) {
	var id int64
	if s.Status == "" {
		s.Status = "active"
	}
	err := r.pg.QueryRow(ctx, `
		INSERT INTO shops (owner_id, tg_chat_id, title, description, payment_info, status)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		s.OwnerID, s.TgChatID, s.Title, s.Description, s.PaymentInfo, s.Status).Scan(&id)
	return id, err
}

func (r *shopsRepo) List(ctx context.Context) ([]Shop, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, owner_id, tg_chat_id, title, COALESCE(description,''),
		       COALESCE(payment_info,''), status FROM shops WHERE status='active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Shop, 0)
	for rows.Next() {
		var s Shop
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title,
			&s.Description, &s.PaymentInfo, &s.Status); err != nil {
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
		       COALESCE(payment_info,''), status FROM shops WHERE id=$1`, id).
		Scan(&s.ID, &s.OwnerID, &s.TgChatID, &s.Title, &s.Description, &s.PaymentInfo, &s.Status)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *shopsRepo) Suspend(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE shops SET status='suspended' WHERE id=$1`, id)
	return err
}

var _ Shops = (*shopsRepo)(nil)

type productsRepo struct{ pg *pgxpool.Pool }

func (r *productsRepo) Insert(ctx context.Context, p Product) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO products (shop_id, tg_msg_id, file_id, title, description, price_amount, price_currency, category, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		ON CONFLICT (shop_id, tg_msg_id) DO UPDATE SET
			file_id=EXCLUDED.file_id, title=EXCLUDED.title, description=EXCLUDED.description,
			price_amount=EXCLUDED.price_amount, price_currency=EXCLUDED.price_currency,
			category=EXCLUDED.category, status=EXCLUDED.status`,
		p.ShopID, p.TgMsgID, p.FileID, p.Title, p.Description, p.PriceAmount, p.PriceCurrency, p.Category, p.Status)
	return err
}

func (r *productsRepo) ListByShop(ctx context.Context, shopID int64) ([]Product, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, shop_id, tg_msg_id, file_id, title, COALESCE(description,''),
		       price_amount, price_currency, COALESCE(category,''), status
		FROM products WHERE shop_id=$1 AND status='on_sale' ORDER BY id`, shopID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Product, 0)
	for rows.Next() {
		var p Product
		if err := rows.Scan(&p.ID, &p.ShopID, &p.TgMsgID, &p.FileID, &p.Title,
			&p.Description, &p.PriceAmount, &p.PriceCurrency, &p.Category, &p.Status); err != nil {
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
		       price_amount, price_currency, COALESCE(category,''), status
		FROM products WHERE id=$1`, id).
		Scan(&p.ID, &p.ShopID, &p.TgMsgID, &p.FileID, &p.Title,
			&p.Description, &p.PriceAmount, &p.PriceCurrency, &p.Category, &p.Status)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

var _ Products = (*productsRepo)(nil)

type ordersRepo struct{ pg *pgxpool.Pool }

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
	err := r.pg.QueryRow(ctx, `
		SELECT id, shop_id, buyer_id, product_id, quantity, price_amount, price_currency, payment_status, COALESCE(contact_details,'')
		FROM orders WHERE id=$1`, id).
		Scan(&o.ID, &o.ShopID, &o.BuyerID, &o.ProductID, &o.Quantity,
			&o.PriceAmount, &o.PriceCurrency, &o.PaymentStatus, &o.ContactDetails)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *ordersRepo) ByBuyer(ctx context.Context, buyerID int64) ([]Order, error) {
	return r.list(ctx, `
		SELECT id, shop_id, buyer_id, product_id, quantity, price_amount, price_currency, payment_status, COALESCE(contact_details,'')
		FROM orders WHERE buyer_id=$1 ORDER BY id DESC`, buyerID)
}

func (r *ordersRepo) ByShop(ctx context.Context, shopID int64) ([]Order, error) {
	return r.list(ctx, `
		SELECT id, shop_id, buyer_id, product_id, quantity, price_amount, price_currency, payment_status, COALESCE(contact_details,'')
		FROM orders WHERE shop_id=$1 ORDER BY id DESC`, shopID)
}

func (r *ordersRepo) list(ctx context.Context, sql string, arg int64) ([]Order, error) {
	rows, err := r.pg.Query(ctx, sql, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Order, 0)
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.ShopID, &o.BuyerID, &o.ProductID, &o.Quantity,
			&o.PriceAmount, &o.PriceCurrency, &o.PaymentStatus, &o.ContactDetails); err != nil {
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

var _ Orders = (*ordersRepo)(nil)