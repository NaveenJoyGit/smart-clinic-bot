package admin

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DefaultClinicSlug is the URL path suffix for the primary Telegram webhook:
// POST /webhook/telegram/{DefaultClinicSlug}
const DefaultClinicSlug = "default"

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

// EnsureDefaultClinic upserts a clinic row with slug DefaultClinicSlug so a single deployment
// has a stable Telegram webhook path and optional bot token from TELEGRAM_TOKEN.
//
// When telegramBotToken is empty, an existing clinics.telegram_bot_token is left unchanged.
func EnsureDefaultClinic(ctx context.Context, pool *pgxpool.Pool, telegramBotToken string, logger *slog.Logger) error {
	token := strings.TrimSpace(telegramBotToken)
	_, err := pool.Exec(ctx, `
INSERT INTO clinics (name, slug, country, telegram_bot_token)
VALUES ('Primary clinic', $1, 'India', NULLIF($2, ''))
ON CONFLICT (slug) DO UPDATE SET
  telegram_bot_token = COALESCE(NULLIF(EXCLUDED.telegram_bot_token, ''), clinics.telegram_bot_token)
`, DefaultClinicSlug, token)
	if err != nil {
		return fmt.Errorf("ensure default clinic: %w", err)
	}
	if logger != nil {
		logger.Info("default clinic ensured", "slug", DefaultClinicSlug)
	}
	return nil
}
