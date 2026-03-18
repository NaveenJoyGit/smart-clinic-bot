package messaging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/naveenjoy/smart-clinic-bot/internal/conversation"
	"github.com/naveenjoy/smart-clinic-bot/internal/engine"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
)

// Handler orchestrates the state-machine pipeline for an incoming message.
type Handler struct {
	conv     *conversation.Manager
	engine   *engine.Engine
	notifier notifierIface
	logger   *slog.Logger
}

type notifierIface interface {
	Send(ctx context.Context, platform, recipientID, text string) error
}

// NewHandler constructs a Handler.
func NewHandler(conv *conversation.Manager, eng *engine.Engine, notifier notifierIface, logger *slog.Logger) *Handler {
	return &Handler{conv: conv, engine: eng, notifier: notifier, logger: logger}
}

// Handle processes a single incoming message through the full pipeline.
func (h *Handler) Handle(ctx context.Context, msg *providers.Message) error {
	// 1. Persist user message (also upserts the conversations row).
	if err := h.conv.AppendMessage(ctx, msg.TenantID, msg.Platform, msg.SenderID, "user", msg.Text); err != nil {
		return fmt.Errorf("append user message: %w", err)
	}

	// 2. Run state machine.
	reply, err := h.engine.Process(ctx, msg)
	if err != nil {
		return fmt.Errorf("engine: %w", err)
	}

	// 3. Persist assistant reply.
	if err := h.conv.AppendMessage(ctx, msg.TenantID, msg.Platform, msg.SenderID, "assistant", reply); err != nil {
		h.logger.WarnContext(ctx, "persist assistant message failed", "error", err)
	}

	// 4. Send to user.
	return h.notifier.Send(ctx, msg.Platform, msg.SenderID, reply)
}
