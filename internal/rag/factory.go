package rag

import (
	"context"

	"github.com/naveenjoy/smart-clinic-bot/internal/config"
	"github.com/sashabaranov/go-openai"
)

// NewEmbedder returns the Embedder selected by cfg.AIProvider ("gemini" or "openai").
// The returned cleanup func is always safe to defer.
func NewEmbedder(ctx context.Context, cfg *config.Config) (Embedder, func(), error) {
	switch cfg.AIProvider {
	case "openai":
		e := newOpenAIEmbedder(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL,
			openai.EmbeddingModel(cfg.OpenAIEmbeddingModel))
		return e, func() {}, nil
	default: // "gemini"
		e, err := newGeminiEmbedder(ctx, cfg.GeminiAPIKey, cfg.GeminiEmbeddingModel)
		if err != nil {
			return nil, func() {}, err
		}
		return e, e.Close, nil
	}
}
