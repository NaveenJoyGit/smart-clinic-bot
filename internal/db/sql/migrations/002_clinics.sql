-- 002_clinics.sql
-- Core clinic (tenant) table.  One row = one dental practice.

CREATE TABLE IF NOT EXISTS clinics (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Identity
    name                TEXT        NOT NULL,
    slug                TEXT        NOT NULL UNIQUE,   -- URL-safe handle, e.g. "bright-smile-delhi"
    -- Location
    address             TEXT,
    city                TEXT,
    state               TEXT,
    country             TEXT        NOT NULL DEFAULT 'India',
    -- Contact
    phone               TEXT,
    email               TEXT,
    website             TEXT,
    -- Operating hours
    -- Format: {"monday":{"open":"09:00","close":"18:00"},"sunday":null, ...}
    timings             JSONB       NOT NULL DEFAULT '{}',
    -- Receptionist
    receptionist_name   TEXT,
    receptionist_phone  TEXT,
    receptionist_email  TEXT,
    -- Lifecycle
    is_active           BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TRIGGER clinics_updated_at
    BEFORE UPDATE ON clinics
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  clinics                   IS 'One row per dental clinic (tenant).';
COMMENT ON COLUMN clinics.slug              IS 'URL-safe unique handle used to route webhook traffic to the correct clinic.';
COMMENT ON COLUMN clinics.timings           IS 'JSONB map of weekday → {open, close} in HH:MM 24-hour format; null value means closed that day.';
