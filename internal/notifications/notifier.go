package notifications

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
)

// Notifier dispatches outbound messages through registered providers.
type Notifier struct {
	logger    *slog.Logger
	providers map[string]providers.MessagingProvider
}

// NewNotifier registers one or more providers by their Name().
func NewNotifier(logger *slog.Logger, ps ...providers.MessagingProvider) *Notifier {
	m := make(map[string]providers.MessagingProvider, len(ps))
	for _, p := range ps {
		m[p.Name()] = p
	}
	return &Notifier{logger: logger, providers: m}
}

// Send routes a message to the named platform provider.
func (n *Notifier) Send(ctx context.Context, platform, recipientID, text string) error {
	p, ok := n.providers[platform]
	if !ok {
		return fmt.Errorf("unknown platform: %s", platform)
	}
	if err := p.SendMessage(ctx, recipientID, text); err != nil {
		n.logger.ErrorContext(ctx, "send message failed", "platform", platform, "error", err)
		return err
	}
	return nil
}
