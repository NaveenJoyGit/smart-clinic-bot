# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Build
go build ./...

# Run locally (requires env vars)
go run ./cmd/server

# Run a single test
go test ./internal/engine/... -run TestFunctionName -v

# Docker development environment
docker compose up --build
```

## Architecture

This is a multi-tenant clinic chatbot that receives messages via webhooks (Telegram, WhatsApp), runs them through a conversation state machine, and responds using RAG + LLM. Each clinic is a tenant identified by a UUID (`clinic_id` / `TenantID`).

### Request flow

```
Webhook (POST /webhook/{platform})
  → messaging.Handler  (parse, persist user message)
  → engine.Engine.Process  (state machine)
  → ai.AIProvider / rag.Retriever  (LLM + vector search for FAQ)
  → notifications.Notifier.Send  (reply to user; alert receptionist)
  → messaging.Handler  (persist assistant reply)
```

### State machine (`internal/conversation/state.go`, `internal/engine/engine.go`)

Three conversation states drive the booking flow:

```
StateStart / StateAnsweringFAQ
  → (intent == "book_appointment") → StateBookingIntent  [ask for name]
  → StateAskTime                                          [ask for time]
  → StateStart                                            [create appointment_request, notify receptionist]
```

Conversation state and collected data (`PatientName`, `PreferredTime`) are stored as JSON in `conversations.metadata` and cached in Redis (24 h TTL).

### Key interfaces

| Interface | Location | Purpose |
|---|---|---|
| `ai.AIProvider` | `internal/ai/provider.go` | LLM text generation |
| `providers.MessagingProvider` | `internal/providers/provider.go` | Per-platform parse + send |
| `engine.notifier` | `internal/engine/engine.go` | Route outbound messages by platform name |
| `rag.Embedder` | `internal/rag/embedder.go` | Text → vector |

Adding a new messaging platform: implement `MessagingProvider`, register with `notifications.NewNotifier`, add a webhook route in `internal/messaging/router.go`.

### Database & migrations

Migrations live in `internal/db/sql/migrations/` as numbered SQL files and are embedded via `//go:embed` in `internal/db/migrations.go`. They run automatically on startup. To add a migration, create the next numbered `.sql` file — no other registration needed.

pgvector is used for clinic knowledge chunks (`clinic_knowledge_chunks.embedding`). Similarity search uses the `<=>` cosine distance operator.

### RAG pipeline

`rag.Indexer.IndexDocument` embeds content and stores it in `clinic_knowledge_chunks`. On each FAQ request, `rag.Retriever.Search` fetches the top-K chunks for the clinic and injects them as a system message before the LLM call.

### Multi-tenancy

All tables are partitioned by `clinic_id`. The `DEFAULT_TENANT_ID` env var is used when a platform message carries no explicit tenant — useful for single-clinic deployments.

### Receptionist notification

After an appointment is saved, `engine.notifyReceptionist` reads `clinics.receptionist_telegram_chat_id` and sends via `notifier.Send(ctx, "telegram", chatID, text)`. If the column is empty the notification is skipped with an info log.

## Environment variables

| Variable | Required | Default |
|---|---|---|
| `DATABASE_URL` | yes | — |
| `GEMINI_API_KEY` | yes | — |
| `PORT` | no | `8080` |
| `REDIS_ADDR` | no | `localhost:6379` |
| `TELEGRAM_TOKEN` | no | — |
| `WHATSAPP_TOKEN` / `WHATSAPP_PHONE_ID` / `WHATSAPP_VERIFY_TOKEN` | no | — |
| `GEMINI_MODEL` | no | `gemini-1.5-flash` |
| `GEMINI_EMBEDDING_MODEL` | no | `text-embedding-004` |
| `DEFAULT_TENANT_ID` | no | `default` |
| `ADMIN_JWT_SECRET` | yes (for admin API) | — |
| `ADMIN_BOOTSTRAP_EMAIL` | no | — |
| `ADMIN_BOOTSTRAP_PASSWORD` | no | — |
