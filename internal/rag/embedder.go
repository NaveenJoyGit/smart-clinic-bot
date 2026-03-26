package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/sashabaranov/go-openai"
)

// Embedder generates a vector embedding for a text string.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// GeminiEmbedder calls the Gemini v1beta REST endpoint directly for embeddings.
// gemini-embedding-001 is only available on v1beta; outputDimensionality is
// fixed at 768 to match the vector(768) column in clinic_knowledge_chunks.
type GeminiEmbedder struct {
	apiKey string
	model  string
	hc     *http.Client
	logger *slog.Logger
}

func newGeminiEmbedder(_ context.Context, apiKey, model string, logger *slog.Logger) (*GeminiEmbedder, error) {
	return &GeminiEmbedder{
		apiKey: apiKey,
		model:  model,
		hc:     &http.Client{Timeout: 30 * time.Second},
		logger: logger,
	}, nil
}

func (e *GeminiEmbedder) Close() {}

func (e *GeminiEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	e.logger.DebugContext(ctx, "embedding text", "model", e.model, "text_len", len(text))
	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent?key=%s",
		e.model, e.apiKey,
	)
	payload, _ := json.Marshal(map[string]any{
		"model":                "models/" + e.model,
		"content":              map[string]any{"parts": []map[string]any{{"text": text}}},
		"outputDimensionality": 768,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.hc.Do(req)
	if err != nil {
		e.logger.WarnContext(ctx, "embed http request failed", "model", e.model, "error", err)
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		e.logger.WarnContext(ctx, "embed api error", "model", e.model, "status", resp.StatusCode, "body", string(body))
		return nil, fmt.Errorf("gemini embedContent (model=%s, status=%d): %s", e.model, resp.StatusCode, body)
	}

	var result struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("gemini embed: decode response: %w", err)
	}
	return result.Embedding.Values, nil
}

// OpenAIEmbedder implements Embedder using the OpenAI Embeddings API.
// text-embedding-3-small produces 1536-dimensional vectors.
// NOTE: switching from GeminiEmbedder to OpenAIEmbedder requires a DB migration
// to resize clinic_knowledge_chunks.embedding from vector(768) to vector(1536)
// and re-indexing all existing knowledge chunks.
type OpenAIEmbedder struct {
	oc    *openai.Client
	model openai.EmbeddingModel
}

func newOpenAIEmbedder(apiKey, baseURL string, model openai.EmbeddingModel) *OpenAIEmbedder {
	var oc *openai.Client
	if baseURL != "" {
		cfg := openai.DefaultConfig(apiKey)
		cfg.BaseURL = baseURL
		oc = openai.NewClientWithConfig(cfg)
	} else {
		oc = openai.NewClient(apiKey)
	}
	return &OpenAIEmbedder{oc: oc, model: model}
}

func (e *OpenAIEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	resp, err := e.oc.CreateEmbeddings(ctx, openai.EmbeddingRequestStrings{
		Input: []string{text},
		Model: e.model,
	})
	if err != nil {
		return nil, err
	}
	return resp.Data[0].Embedding, nil
}
