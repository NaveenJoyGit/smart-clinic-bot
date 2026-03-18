-- 005_appointments.sql
-- Patient appointment requests captured by the chatbot.

CREATE TABLE IF NOT EXISTS appointment_requests (
    id                  UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    -- Links
    clinic_id           UUID        NOT NULL REFERENCES clinics(id),
    conversation_id     UUID        REFERENCES conversations(id),
    doctor_id           UUID        REFERENCES clinic_doctors(id),    -- optional preference
    service_id          UUID        REFERENCES clinic_services(id),   -- optional preference
    -- Patient details
    patient_name        TEXT        NOT NULL,
    patient_phone       TEXT        NOT NULL,
    patient_email       TEXT,
    -- Scheduling preference
    preferred_date      DATE,
    preferred_time      TEXT,       -- free-text slot, e.g. "Morning" or "10:00 AM"
    notes               TEXT,
    -- Lifecycle
    status              TEXT        NOT NULL DEFAULT 'pending',
    -- pending | confirmed | cancelled | completed | no_show
    confirmed_at        TIMESTAMPTZ,
    cancelled_at        TIMESTAMPTZ,
    cancellation_reason TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_appt_clinic  ON appointment_requests(clinic_id, status);
CREATE INDEX IF NOT EXISTS idx_appt_date    ON appointment_requests(clinic_id, preferred_date);
CREATE INDEX IF NOT EXISTS idx_appt_doctor  ON appointment_requests(doctor_id);

CREATE TRIGGER appointment_requests_updated_at
    BEFORE UPDATE ON appointment_requests
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  appointment_requests        IS 'Appointment booking requests collected by the chatbot.';
COMMENT ON COLUMN appointment_requests.status IS 'pending | confirmed | cancelled | completed | no_show';
COMMENT ON COLUMN appointment_requests.preferred_time IS 'Free-text slot as stated by the patient; clinic staff resolves the exact time.';
