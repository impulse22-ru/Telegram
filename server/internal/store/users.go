package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// usersRepo — реализация Users на pgxpool.
type usersRepo struct{ pg *pgxpool.Pool }

// upsertUserSQL — SQL шаблон для upsert пользователя: создаёт нового или обновляет phone/name у существующего.
const upsertUserSQL = `
INSERT INTO users (tg_user_id, phone, name, role)
VALUES ($1, $2, $3, 'user')
ON CONFLICT (tg_user_id) DO UPDATE SET
	phone = EXCLUDED.phone,
	name  = EXCLUDED.name
RETURNING id`

// Upsert — создаёт/обновляет пользователя по tg_user_id; role по умолчанию "user".
func (r *usersRepo) Upsert(ctx context.Context, u User) (int64, error) {
	var id int64
	if u.Role == "" {
		u.Role = "user"
	}
	err := r.pg.QueryRow(ctx, upsertUserSQL, u.TgUserID, u.Phone, u.Name).Scan(&id)
	return id, err
}

// GetByTgID — возвращает пользователя по Telegram user ID.
func (r *usersRepo) GetByTgID(ctx context.Context, tgID int64) (*User, error) {
	row := r.pg.QueryRow(ctx, `
		SELECT id, tg_user_id, COALESCE(phone,''), COALESCE(name,''), role, banned, created_at
		FROM users WHERE tg_user_id = $1`, tgID)
	var u User
	if err := row.Scan(&u.ID, &u.TgUserID, &u.Phone, &u.Name, &u.Role, &u.Banned, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// Get — возвращает пользователя по внутреннему ID.
func (r *usersRepo) Get(ctx context.Context, id int64) (*User, error) {
	row := r.pg.QueryRow(ctx, `
		SELECT id, tg_user_id, COALESCE(phone,''), COALESCE(name,''), role, banned, created_at
		FROM users WHERE id = $1`, id)
	var u User
	if err := row.Scan(&u.ID, &u.TgUserID, &u.Phone, &u.Name, &u.Role, &u.Banned, &u.CreatedAt); err != nil {
		return nil, err
	}
	return &u, nil
}

// SetRole — устанавливает роль пользователя (user / admin / seller).
func (r *usersRepo) SetRole(ctx context.Context, id int64, role string) error {
	_, err := r.pg.Exec(ctx, `UPDATE users SET role=$2 WHERE id=$1`, id, role)
	return err
}

// SetBanned — бан/разбан пользователя.
func (r *usersRepo) SetBanned(ctx context.Context, id int64, banned bool) error {
	_, err := r.pg.Exec(ctx, `UPDATE users SET banned=$2 WHERE id=$1`, id, banned)
	return err
}

var _ Users = (*usersRepo)(nil)
