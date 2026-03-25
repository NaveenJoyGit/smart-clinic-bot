-- 009_clinics_telegram_bot_token.sql
-- Per-clinic Telegram bot token for multi-bot, single-deployment routing.
--
-- Security note:
-- - Treat this value as a secret. Limit DB access accordingly.
-- - Consider encrypting at rest if your threat model requires it.

ALTER TABLE clinics
  ADD COLUMN IF NOT EXISTS telegram_bot_token TEXT;

COMMENT ON COLUMN clinics.telegram_bot_token IS 'Telegram bot token used for this clinic when receiving/sending messages via Telegram. Stored for multi-tenant single-deployment setups.';

