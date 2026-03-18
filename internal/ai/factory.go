package ai

import (
	"context"

	"github.com/naveenjoy/smart-clinic-bot/internal/config"
)

// New returns the AIProvider selected by cfg.AIProvider ("gemini" or "openai").
// The returned cleanup func is always safe to defer.
func New(ctx context.Context, cfg *config.Config) (AIProvider, func(), error) {
	switch cfg.AIProvider {
	case "openai":
		p := newOpenAIProvider(cfg.OpenAIAPIKey, cfg.OpenAIBaseURL, cfg.OpenAIModel)
		return p, func() {}, nil
	default: // "gemini"
		p, err := newGeminiProvider(ctx, cfg.GeminiAPIKey, cfg.GeminiModel)
		if err != nil {
			return nil, func() {}, err
		}
		return p, p.Close, nil
	}
}
