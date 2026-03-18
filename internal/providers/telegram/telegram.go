package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
)

const apiBase = "https://api.telegram.org"

type Provider struct {
	token    string
	tenantID string
	client   *http.Client
}

func New(token, tenantID string) *Provider {
	return &Provider{
		token:    token,
		tenantID: tenantID,
		client:   &http.Client{},
	}
}

func (p *Provider) Name() string { return "telegram" }

// update is a minimal Telegram Update payload.
type update struct {
	Message *struct {
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
		Text string `json:"text"`
	} `json:"message"`
}

func (p *Provider) ReceiveMessage(body []byte, _ map[string]string) (*providers.Message, error) {
	var u update
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, err
	}
	if u.Message == nil || u.Message.Text == "" {
		return nil, nil // not a text message
	}
	return &providers.Message{
		Platform: p.Name(),
		TenantID: p.tenantID,
		SenderID: fmt.Sprintf("%d", u.Message.From.ID),
		Text:     u.Message.Text,
	}, nil
}

func (p *Provider) SendMessage(ctx context.Context, recipientID, text string) error {
	payload, err := json.Marshal(map[string]any{
		"chat_id": recipientID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", apiBase, p.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram API error: %s", resp.Status)
	}
	return nil
}
