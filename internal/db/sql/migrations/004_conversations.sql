-- 004_conversations.sql
-- Chat sessions and message history.

CREATE TABLE IF NOT EXISTS conversations (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id      UUID        NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    platform       TEXT        NOT NULL,              -- telegram | whatsapp
    external_id    TEXT        NOT NULL,              -- sender's platform user-id
    patient_name   TEXT,                              -- extracted from conversation
    patient_phone  TEXT,                              -- extracted from conversation
    status         TEXT        NOT NULL DEFAULT 'active', -- active | closed | escalated
    metadata       JSONB       NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (clinic_id, platform, external_id)
);

CREATE INDEX IF NOT EXISTS idx_conversations_clinic    ON conversations(clinic_id);
CREATE INDEX IF NOT EXISTS idx_conversations_status    ON conversations(clinic_id, status);

CREATE TRIGGER conversations_updated_at
    BEFORE UPDATE ON conversations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  conversations              IS 'One row per unique (clinic, platform, sender) chat session.';
COMMENT ON COLUMN conversations.external_id IS 'Platform-native user identifier (chat_id for Telegram, phone number for WhatsApp).';
COMMENT ON COLUMN conversations.status      IS 'active | closed | escalated';


CREATE TABLE IF NOT EXISTS messages (
    id              UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID        NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT        NOT NULL,   -- user | assistant | system
    content         TEXT        NOT NULL,
    metadata        JSONB       NOT NULL DEFAULT '{}',  -- intent, detected entities, etc.
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id, created_at);

COMMENT ON TABLE  messages          IS 'Individual chat turns within a conversation.';
COMMENT ON COLUMN messages.role     IS 'user | assistant | system';
COMMENT ON COLUMN messages.metadata IS 'Optional bag for intent labels, confidence scores, extracted slots, etc.';
