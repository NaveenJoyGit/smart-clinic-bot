package messaging

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers/whatsapp"
)

// NewRouter wires all HTTP endpoints.
func NewRouter(
	handler *Handler,
	tgProvider providers.MessagingProvider,
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

	r.Post("/webhook/telegram", webhookHandlerFunc(handler, tgProvider, logger))

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
