package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/telegram"
)

type telegramTokenResolver interface {
	TelegramTokenForClinic(ctx context.Context, clinicID string) (string, bool, error)
}

type dbTelegramTokenResolver struct {
	pool *pgxpool.Pool
}

func (r dbTelegramTokenResolver) TelegramTokenForClinic(ctx context.Context, clinicID string) (string, bool, error) {
	if r.pool == nil || clinicID == "" {
		return "", false, nil
	}
	var tok string
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(telegram_bot_token,'') FROM clinics WHERE id = $1::uuid AND is_active = TRUE`,
		clinicID,
	).Scan(&tok)
	if err == nil {
		if tok == "" {
			return "", false, nil
		}
		return tok, true, nil
	}
	if err == pgx.ErrNoRows {
		return "", false, nil
	}
	return "", false, err
}

// Notifier dispatches outbound messages through registered providers.
type Notifier struct {
	logger    *slog.Logger
	providers map[string]providers.MessagingProvider

	telegramDefaultToken string
	telegramClient       *http.Client
	telegramResolver     telegramTokenResolver
}

// NewNotifier registers one or more providers by their Name().
func NewNotifier(
	logger *slog.Logger,
	pool *pgxpool.Pool,
	telegramDefaultToken string,
	telegramClient *http.Client,
	ps ...providers.MessagingProvider,
) *Notifier {
	return NewNotifierWithTelegramResolver(logger, dbTelegramTokenResolver{pool: pool}, telegramDefaultToken, telegramClient, ps...)
}

// NewNotifierWithTelegramResolver allows injecting a token resolver (useful for tests).
func NewNotifierWithTelegramResolver(
	logger *slog.Logger,
	resolver telegramTokenResolver,
	telegramDefaultToken string,
	telegramClient *http.Client,
	ps ...providers.MessagingProvider,
) *Notifier {
	m := make(map[string]providers.MessagingProvider, len(ps))
	for _, p := range ps {
		m[p.Name()] = p
	}
	return &Notifier{
		logger:              logger,
		providers:            m,
		telegramDefaultToken: telegramDefaultToken,
		telegramClient:       telegramClient,
		telegramResolver:     resolver,
	}
}

// Send routes a message to the named platform.
//
// For Telegram, the bot token is chosen by tenantID (clinic UUID) when present,
// falling back to the default token to preserve legacy single-tenant behavior.
func (n *Notifier) Send(ctx context.Context, platform, tenantID, recipientID, text string) error {
	if platform == "telegram" {
		token := n.telegramDefaultToken
		if tenantID != "" && n.telegramResolver != nil {
			if tok, ok, err := n.telegramResolver.TelegramTokenForClinic(ctx, tenantID); err != nil {
				n.logger.ErrorContext(ctx, "telegram token lookup failed", "clinic_id", tenantID, "error", err)
			} else if ok {
				token = tok
			}
		}
		if token == "" {
			return fmt.Errorf("telegram token not configured for clinic_id=%s", tenantID)
		}
		if err := telegram.SendMessageWithToken(ctx, n.telegramClient, token, recipientID, text); err != nil {
			n.logger.ErrorContext(ctx, "send message failed", "platform", platform, "clinic_id", tenantID, "error", err)
			return err
		}
		return nil
	}

	p, ok := n.providers[platform]
	if !ok {
		return fmt.Errorf("unknown platform: %s", platform)
	}
	if err := p.SendMessage(ctx, recipientID, text); err != nil {
		n.logger.ErrorContext(ctx, "send message failed", "platform", platform, "clinic_id", tenantID, "error", err)
		return err
	}
	return nil
}
