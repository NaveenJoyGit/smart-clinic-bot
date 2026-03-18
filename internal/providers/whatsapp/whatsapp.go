package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
)

const graphBase = "https://graph.facebook.com/v19.0"

type Provider struct {
	token       string
	phoneID     string
	verifyToken string
	tenantID    string
	client      *http.Client
}

func New(token, phoneID, verifyToken, tenantID string) *Provider {
	return &Provider{
		token:       token,
		phoneID:     phoneID,
		verifyToken: verifyToken,
		tenantID:    tenantID,
		client:      &http.Client{},
	}
}

func (p *Provider) Name() string        { return "whatsapp" }
func (p *Provider) VerifyToken() string { return p.verifyToken }

// waWebhook is the top-level WhatsApp Cloud API webhook payload.
type waWebhook struct {
	Entry []struct {
		Changes []struct {
			Value struct {
				Messages []struct {
					From string `json:"from"`
					Type string `json:"type"`
					Text struct {
						Body string `json:"body"`
					} `json:"text"`
				} `json:"messages"`
			} `json:"value"`
		} `json:"changes"`
	} `json:"entry"`
}

func (p *Provider) ReceiveMessage(body []byte, _ map[string]string) (*providers.Message, error) {
	var payload waWebhook
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" || msg.Text.Body == "" {
					continue
				}
				return &providers.Message{
					Platform: p.Name(),
					TenantID: p.tenantID,
					SenderID: msg.From,
					Text:     msg.Text.Body,
				}, nil
			}
		}
	}
	return nil, nil // no text message found
}

func (p *Provider) SendMessage(ctx context.Context, recipientID, text string) error {
	payload, err := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                recipientID,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/%s/messages", graphBase, p.phoneID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("whatsapp API error: %s", resp.Status)
	}
	return nil
}
