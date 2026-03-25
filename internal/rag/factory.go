package rag

import (
	"context"
	"log/slog"

	"github.com/naveenjoy/smart-clinic-bot/internal/config"
	"github.com/sashabaranov/go-openai"
)

// NewEmbedder returns the Embedder selected by cfg.AIProvider ("gemini" or "openai").
// The returned cleanup func is always safe to defer.
func NewEmbedder(ctx context.Context, cfg *config.Config, logger *slog.Logger) (Embedder, func(), error) {
	switch cfg.AIProvider {
	case "openai":
		e := newOpenAIEmbedder(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL,
			openai.EmbeddingModel(cfg.OpenAIEmbeddingModel))
		return e, func() {}, nil
	default: // "gemini"
		e, err := newGeminiEmbedder(ctx, cfg.GeminiAPIKey, cfg.GeminiEmbeddingModel, logger)
		if err != nil {
			return nil, func() {}, err
		}
		return e, e.Close, nil
	}
}
