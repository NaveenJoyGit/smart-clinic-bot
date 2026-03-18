package providers

import "context"

// Message is a normalized message from any platform.
type Message struct {
	Platform string
	TenantID string
	SenderID string
	Text     string
	Metadata map[string]string
}

// MessagingProvider abstracts a messaging platform.
type MessagingProvider interface {
	Name() string
	ReceiveMessage(body []byte, headers map[string]string) (*Message, error)
	SendMessage(ctx context.Context, recipientID, text string) error
}
