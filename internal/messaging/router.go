package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/telegram"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/whatsapp"
)

// NewRouter wires all HTTP endpoints.
func NewRouter(
	handler *Handler,
	pool *pgxpool.Pool,
	waProvider *whatsapp.Provider,
	adminRouter http.Handler,
	dashboardRouter http.Handler,
	logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	r.Post("/webhook/telegram/{slug}", telegramWebhookBySlug(handler, pool, logger))

	r.Get("/webhook/whatsapp", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("hub.verify_token") != waProvider.VerifyToken() {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(q.Get("hub.challenge")))
	})

	r.Post("/webhook/whatsapp", webhookHandlerFunc(handler, waProvider, logger))

	r.Mount("/admin", adminRouter)
	r.Mount("/dashboard", dashboardRouter)

	return r
}

type clinicBySlugLookupFunc func(ctx context.Context, slug string) (clinicID string, tokenConfigured bool, err error)

func telegramWebhookBySlug(handler *Handler, pool *pgxpool.Pool, logger *slog.Logger) http.HandlerFunc {
	return telegramWebhookBySlugHandler(handler.Handle, clinicBySlugLookupFromDB(pool), logger)
}

func clinicBySlugLookupFromDB(pool *pgxpool.Pool) clinicBySlugLookupFunc {
	return func(ctx context.Context, slug string) (string, bool, error) {
		if pool == nil {
			return "", false, fmt.Errorf("db pool is nil")
		}
		var clinicID string
		var token string
		err := pool.QueryRow(ctx,
			`SELECT id::text, COALESCE(telegram_bot_token,'')
			   FROM clinics
			  WHERE slug = $1
			    AND is_active = TRUE`,
			slug,
		).Scan(&clinicID, &token)
		if err == pgx.ErrNoRows {
			return "", false, pgx.ErrNoRows
		}
		if err != nil {
			return "", false, err
		}
		return clinicID, token != "", nil
	}
}

func telegramWebhookBySlugHandler(
	handle func(ctx context.Context, msg *providers.Message) error,
	lookup clinicBySlugLookupFunc,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := chi.URLParam(r, "slug")
		if slug == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		clinicID, tokenConfigured, err := lookup(r.Context(), slug)
		if err == pgx.ErrNoRows {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil {
			logger.ErrorContext(r.Context(), "lookup clinic by slug failed", "slug", slug, "error", err)
			w.WriteHeader(http.StatusOK) // always 200 to avoid retries
			return
		}
		if !tokenConfigured {
			// Avoid Telegram retry storms; operator needs to set clinics.telegram_bot_token.
			logger.WarnContext(r.Context(), "telegram bot token not configured for clinic", "clinic_id", clinicID, "slug", slug)
			w.WriteHeader(http.StatusOK)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.ErrorContext(r.Context(), "read body failed", "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		senderID, text, ok, err := telegram.ParseUpdate(body)
		if err != nil {
			logger.ErrorContext(r.Context(), "parse telegram webhook failed", "slug", slug, "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}
		if !ok {
			w.WriteHeader(http.StatusOK)
			return
		}

		msg := &providers.Message{
			Platform: "telegram",
			TenantID: clinicID,
			SenderID: senderID,
			Text:     text,
		}

		if err := handle(context.Background(), msg); err != nil {
			logger.ErrorContext(r.Context(), "handle message failed", "platform", "telegram", "slug", slug, "clinic_id", clinicID, "error", err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func webhookHandlerFunc(handler *Handler, p providers.MessagingProvider, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			logger.ErrorContext(r.Context(), "read body failed", "error", err)
			w.WriteHeader(http.StatusOK) // always 200 to avoid retries
			return
		}

		headers := flattenHeaders(r.Header)

		msg, err := p.ReceiveMessage(body, headers)
		if err != nil {
			logger.ErrorContext(r.Context(), "parse webhook failed", "provider", p.Name(), "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}
		if msg == nil {
			// Non-text update — acknowledge silently.
			w.WriteHeader(http.StatusOK)
			return
		}

		if err := handler.Handle(context.Background(), msg); err != nil {
			logger.ErrorContext(r.Context(), "handle message failed", "provider", p.Name(), "error", err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, vs := range h {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}
