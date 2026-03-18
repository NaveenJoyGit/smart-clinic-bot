package ai

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiProvider implements AIProvider using the Google Gemini API.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// newGeminiProvider creates a Gemini-backed AIProvider.
func newGeminiProvider(ctx context.Context, apiKey, model string) (*GeminiProvider, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, err
	}
	return &GeminiProvider{client: client, model: model}, nil
}

// Close releases the underlying client resources.
func (p *GeminiProvider) Close() { p.client.Close() }

// GenerateResponse sends messages and returns the assistant reply.
func (p *GeminiProvider) GenerateResponse(ctx context.Context, messages []Message) (string, error) {
	m := p.client.GenerativeModel(p.model)

	// Collect system messages into SystemInstruction.
	var sysParts []genai.Part
	var chat []Message
	for _, msg := range messages {
		if msg.Role == "system" {
			sysParts = append(sysParts, genai.Text(msg.Content))
		} else {
			chat = append(chat, msg)
		}
	}
	if len(sysParts) > 0 {
		m.SystemInstruction = &genai.Content{Parts: sysParts}
	}

	if len(chat) == 0 {
		return "", fmt.Errorf("no user message provided")
	}

	// All but the last message form the chat history.
	var history []*genai.Content
	for _, msg := range chat[:len(chat)-1] {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		history = append(history, &genai.Content{
			Role:  role,
			Parts: []genai.Part{genai.Text(msg.Content)},
		})
	}

	cs := m.StartChat()
	cs.History = history

	resp, err := cs.SendMessage(ctx, genai.Text(chat[len(chat)-1].Content))
	if err != nil {
		return "", err
	}
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini")
	}
	if txt, ok := resp.Candidates[0].Content.Parts[0].(genai.Text); ok {
		return string(txt), nil
	}
	return "", fmt.Errorf("unexpected response part type from Gemini")
}
