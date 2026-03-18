package engine

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/ai"
	"github.com/naveenjoy/smart-clinic-bot/internal/conversation"
	"github.com/naveenjoy/smart-clinic-bot/internal/providers"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
)

type notifier interface {
	Send(ctx context.Context, platform, recipientID, text string) error
}

// Engine runs the state-machine conversation logic.
type Engine struct {
	pool      *pgxpool.Pool
	conv      *conversation.Manager
	ai        ai.AIProvider
	retriever *rag.Retriever
	notifier  notifier
	logger    *slog.Logger
}

// New constructs an Engine.
func New(pool *pgxpool.Pool, conv *conversation.Manager, aiClient ai.AIProvider,
	retriever *rag.Retriever, n notifier, logger *slog.Logger) *Engine {
	return &Engine{pool: pool, conv: conv, ai: aiClient, retriever: retriever,
		notifier: n, logger: logger}
}

// Process runs the state machine for one incoming message and returns the reply.
func (e *Engine) Process(ctx context.Context, msg *providers.Message) (string, error) {
	data, err := e.conv.GetConvData(ctx, msg.TenantID, msg.Platform, msg.SenderID)
	if err != nil {
		return "", err
	}

	switch data.State {
	case conversation.StateAskTime:
		return e.handleAskTime(ctx, msg, data)
	case conversation.StateBookingIntent:
		return e.handleBookingIntent(ctx, msg, data)
	default: // START, ANSWERING_FAQ, or empty
		return e.handleGeneral(ctx, msg, data)
	}
}

func (e *Engine) handleGeneral(ctx context.Context, msg *providers.Message, data conversation.ConvData) (string, error) {
	intent, err := e.classifyIntent(ctx, msg.Text)
	if err != nil {
		e.logger.WarnContext(ctx, "intent classification failed, defaulting to faq", "error", err)
		intent = "faq"
	}

	if strings.TrimSpace(intent) == "book_appointment" {
		data.State = conversation.StateBookingIntent
		if err := e.conv.SetConvData(ctx, msg.TenantID, msg.Platform, msg.SenderID, data); err != nil {
			e.logger.WarnContext(ctx, "set conv data failed", "error", err)
		}
		return "I'd be happy to help you book an appointment! Could you please share your full name?", nil
	}

	// FAQ: RAG + LLM
	docs, _ := e.retriever.Search(ctx, msg.TenantID, msg.Text, 3)
	history, err := e.conv.GetHistory(ctx, msg.TenantID, msg.Platform, msg.SenderID)
	if err != nil {
		return "", err
	}
	reply, err := e.ai.GenerateResponse(ctx, buildFAQMessages(history, docs, msg.Text))
	if err != nil {
		return "", err
	}
	data.State = conversation.StateAnsweringFAQ
	_ = e.conv.SetConvData(ctx, msg.TenantID, msg.Platform, msg.SenderID, data)
	return reply, nil
}

func (e *Engine) handleBookingIntent(ctx context.Context, msg *providers.Message, data conversation.ConvData) (string, error) {
	data.PatientName = strings.TrimSpace(msg.Text)
	data.State = conversation.StateAskTime
	if err := e.conv.SetConvData(ctx, msg.TenantID, msg.Platform, msg.SenderID, data); err != nil {
		return "", err
	}
	return fmt.Sprintf("Thanks, %s! What date and time works best for your appointment? (e.g. \"Monday morning\" or \"Tuesday at 3pm\")", data.PatientName), nil
}

func (e *Engine) handleAskTime(ctx context.Context, msg *providers.Message, data conversation.ConvData) (string, error) {
	data.PreferredTime = strings.TrimSpace(msg.Text)

	if err := e.createAppointment(ctx, msg, data); err != nil {
		e.logger.ErrorContext(ctx, "create appointment failed", "error", err)
		return "Sorry, I couldn't save your appointment request. Please try again.", nil
	}

	e.notifyReceptionist(ctx, msg, data)

	patientName := data.PatientName
	preferredTime := data.PreferredTime

	// Reset state
	data.State = conversation.StateStart
	data.PatientName = ""
	data.PreferredTime = ""
	_ = e.conv.SetConvData(ctx, msg.TenantID, msg.Platform, msg.SenderID, data)

	return fmt.Sprintf("Your appointment request has been submitted! We'll confirm with you shortly.\n\nSummary:\n• Name: %s\n• Preferred time: %s", patientName, preferredTime), nil
}

func (e *Engine) classifyIntent(ctx context.Context, text string) (string, error) {
	msgs := []ai.Message{
		{Role: "system", Content: `You are an intent classifier for a clinic chatbot.
Classify the user's message as exactly one of:
- book_appointment: the user wants to schedule, book, or make an appointment
- faq: the user is asking a question or seeking information

Reply with ONLY one of those two values and nothing else.`},
		{Role: "user", Content: text},
	}
	result, err := e.ai.GenerateResponse(ctx, msgs)
	return strings.ToLower(strings.TrimSpace(result)), err
}

func (e *Engine) createAppointment(ctx context.Context, msg *providers.Message, data conversation.ConvData) error {
	var convID string
	_ = e.pool.QueryRow(ctx,
		`SELECT id::text FROM conversations WHERE clinic_id = $1 AND platform = $2 AND external_id = $3`,
		msg.TenantID, msg.Platform, msg.SenderID,
	).Scan(&convID)

	_, err := e.pool.Exec(ctx, `
		INSERT INTO appointment_requests
		  (clinic_id, conversation_id, patient_name, patient_phone, preferred_time, status)
		VALUES ($1::uuid, NULLIF($2,'')::uuid, $3, $4, $5, 'pending')`,
		msg.TenantID, convID, data.PatientName, msg.SenderID, data.PreferredTime,
	)
	return err
}

func (e *Engine) notifyReceptionist(ctx context.Context, msg *providers.Message, data conversation.ConvData) {
	var chatID string
	_ = e.pool.QueryRow(ctx,
		`SELECT COALESCE(receptionist_telegram_chat_id,'') FROM clinics WHERE id = $1::uuid`,
		msg.TenantID,
	).Scan(&chatID)

	if chatID == "" {
		e.logger.InfoContext(ctx, "no receptionist telegram chat id configured, skipping notification",
			"clinic_id", msg.TenantID)
		return
	}

	text := fmt.Sprintf("New appointment request\nPatient: %s\nPreferred time: %s\nPlatform: %s\nContact: %s",
		data.PatientName, data.PreferredTime, msg.Platform, msg.SenderID)

	if err := e.notifier.Send(ctx, "telegram", chatID, text); err != nil {
		e.logger.WarnContext(ctx, "receptionist notification failed", "error", err)
	}
}

func buildFAQMessages(history []ai.Message, docs []rag.Document, userText string) []ai.Message {
	const sysPrompt = "You are a helpful clinic assistant. Answer patient questions clearly and compassionately."
	msgs := []ai.Message{{Role: "system", Content: sysPrompt}}
	if len(docs) > 0 {
		var sb strings.Builder
		sb.WriteString("Relevant clinic information:\n")
		for _, d := range docs {
			sb.WriteString(fmt.Sprintf("- [%s] %s\n", d.SourceType, d.Content))
		}
		msgs = append(msgs, ai.Message{Role: "system", Content: sb.String()})
	}
	msgs = append(msgs, history...)
	msgs = append(msgs, ai.Message{Role: "user", Content: userText})
	return msgs
}
