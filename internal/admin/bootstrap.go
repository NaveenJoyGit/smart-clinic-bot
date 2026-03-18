package admin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BootstrapSuperAdmin upserts a super_admin with the given email/password.
// If the email already exists the password_hash is updated, so rotating the
// password env var takes effect on the next server restart.
func BootstrapSuperAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string, logger *slog.Logger) error {
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	const q = `
INSERT INTO admin_users (name, email, password_hash, role)
VALUES ('Bootstrap Admin', $1, $2, 'super_admin')
ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash`
	if _, err = pool.Exec(ctx, q, email, hash); err != nil {
		return fmt.Errorf("upsert bootstrap admin: %w", err)
	}
	logger.Info("bootstrap admin upserted", "email", email)
	return nil
}
