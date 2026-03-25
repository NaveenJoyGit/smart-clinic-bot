package notifications

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

type fakeTelegramResolver struct {
	tokens map[string]string
}

func (r fakeTelegramResolver) TelegramTokenForClinic(_ context.Context, clinicID string) (string, bool, error) {
	tok, ok := r.tokens[clinicID]
	if !ok || tok == "" {
		return "", false, nil
	}
	return tok, true, nil
}

type captureTransport struct {
	lastURL string
}

func (t *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.lastURL = req.URL.String()
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(`ok`)),
		Header:     make(http.Header),
	}, nil
}

func TestNotifier_Telegram_UsesClinicTokenWhenAvailable(t *testing.T) {
	transport := &captureTransport{}
	client := &http.Client{Transport: transport}

	n := NewNotifierWithTelegramResolver(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeTelegramResolver{tokens: map[string]string{"clinic-a": "TOKEN_A"}},
		client,
	)

	if err := n.Send(context.Background(), "telegram", "clinic-a", "123", "hi"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if !strings.Contains(transport.lastURL, "/botTOKEN_A/sendMessage") {
		t.Fatalf("expected clinic token in URL, got %q", transport.lastURL)
	}
}

func TestNotifier_Telegram_ErrorsWhenNoTokenConfigured(t *testing.T) {
	n := NewNotifierWithTelegramResolver(
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeTelegramResolver{tokens: map[string]string{}},
		http.DefaultClient,
	)

	if err := n.Send(context.Background(), "telegram", "clinic-a", "123", "hi"); err == nil {
		t.Fatalf("expected error when no token configured")
	}
}

