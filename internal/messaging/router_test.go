package messaging

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
)

func TestTelegramWebhookBySlugHandler_StampsTenantIDAndCallsHandle(t *testing.T) {
	var got *providers.Message
	handle := func(_ context.Context, msg *providers.Message) error {
		got = msg
		return nil
	}
	lookup := func(_ context.Context, slug string) (string, bool, error) {
		if slug != "clinic-a" {
			t.Fatalf("unexpected slug %q", slug)
		}
		return "clinic-uuid-a", true, nil
	}

	r := chi.NewRouter()
	r.Post("/webhook/telegram/{slug}", telegramWebhookBySlugHandler(handle, lookup, slog.New(slog.NewTextHandler(io.Discard, nil))))

	body := `{"message":{"from":{"id":123},"text":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram/clinic-a", strings.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got == nil {
		t.Fatalf("expected handle to be called")
	}
	if got.TenantID != "clinic-uuid-a" {
		t.Fatalf("TenantID = %q, want %q", got.TenantID, "clinic-uuid-a")
	}
	if got.Platform != "telegram" {
		t.Fatalf("Platform = %q, want %q", got.Platform, "telegram")
	}
	if got.SenderID != "123" {
		t.Fatalf("SenderID = %q, want %q", got.SenderID, "123")
	}
	if got.Text != "hello" {
		t.Fatalf("Text = %q, want %q", got.Text, "hello")
	}
}

func TestTelegramWebhookBySlugHandler_SkipsWhenTokenNotConfigured(t *testing.T) {
	called := false
	handle := func(_ context.Context, _ *providers.Message) error {
		called = true
		return nil
	}
	lookup := func(_ context.Context, _ string) (string, bool, error) {
		return "clinic-uuid-a", false, nil
	}

	r := chi.NewRouter()
	r.Post("/webhook/telegram/{slug}", telegramWebhookBySlugHandler(handle, lookup, slog.New(slog.NewTextHandler(io.Discard, nil))))

	body := `{"message":{"from":{"id":123},"text":"hello"}}`
	req := httptest.NewRequest(http.MethodPost, "/webhook/telegram/clinic-a", strings.NewReader(body))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if called {
		t.Fatalf("expected handle not to be called when token not configured")
	}
}

