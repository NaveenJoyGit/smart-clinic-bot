package ai

import "context"

// Message is a single chat turn.
type Message struct {
	Role    string
	Content string
}

// AIProvider is the interface for LLM backends.
// Implement this interface to add Anthropic, Gemini, or any other provider.
type AIProvider interface {
	GenerateResponse(ctx context.Context, messages []Message) (string, error)
}
