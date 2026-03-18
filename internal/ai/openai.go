package ai

import (
	"context"
	"fmt"

	"github.com/sashabaranov/go-openai"
)

// OpenAIProvider implements AIProvider using the OpenAI Chat Completions API.
type OpenAIProvider struct {
	oc    *openai.Client
	model string
}

// newOpenAIProvider creates an OpenAI-backed AIProvider.
// baseURL is optional; pass "" to use the default OpenAI endpoint.
func newOpenAIProvider(apiKey, baseURL, model string) *OpenAIProvider {
	var oc *openai.Client
	if baseURL != "" {
		cfg := openai.DefaultConfig(apiKey)
		cfg.BaseURL = baseURL
		oc = openai.NewClientWithConfig(cfg)
	} else {
		oc = openai.NewClient(apiKey)
	}
	return &OpenAIProvider{oc: oc, model: model}
}

// GenerateResponse sends messages and returns the assistant reply.
func (p *OpenAIProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	oaiMessages := make([]openai.ChatCompletionMessage, len(messages))
	for i, m := range messages {
		oaiMessages[i] = openai.ChatCompletionMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	resp, err := p.oc.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:    p.model,
		Messages: oaiMessages,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response from OpenAI")
	}
	return resp.Choices[0].Message.Content, nil
}
