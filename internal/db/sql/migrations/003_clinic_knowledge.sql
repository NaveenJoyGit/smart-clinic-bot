-- 003_clinic_knowledge.sql
-- Per-clinic knowledge: services, doctors, FAQs.

-- ── Services ────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS clinic_services (
    id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id        UUID         NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    name             TEXT         NOT NULL,
    category         TEXT         NOT NULL,   -- e.g. preventive | cosmetic | orthodontics | restorative | surgical
    description      TEXT,
    price_min        NUMERIC(10,2),
    price_max        NUMERIC(10,2),
    price_note       TEXT,                    -- e.g. "varies by complexity"
    duration_minutes INTEGER,
    is_active        BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clinic_services_clinic ON clinic_services(clinic_id);

CREATE TRIGGER clinic_services_updated_at
    BEFORE UPDATE ON clinic_services
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  clinic_services          IS 'Treatments / procedures offered by a clinic with pricing.';
COMMENT ON COLUMN clinic_services.category IS 'preventive | cosmetic | orthodontics | restorative | surgical';


-- ── Doctors ─────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS clinic_doctors (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id         UUID        NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    name              TEXT        NOT NULL,
    title             TEXT        NOT NULL DEFAULT 'Dr.',
    specialization    TEXT,
    qualifications    TEXT[]      NOT NULL DEFAULT '{}',  -- e.g. {"BDS","MDS (Orthodontics)"}
    bio               TEXT,
    experience_years  INTEGER,
    available_days    TEXT[]      NOT NULL DEFAULT '{}',  -- e.g. {"Monday","Wednesday","Friday"}
    consultation_fee  NUMERIC(10,2),
    languages         TEXT[]      NOT NULL DEFAULT '{"English"}',
    is_active         BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clinic_doctors_clinic ON clinic_doctors(clinic_id);

CREATE TRIGGER clinic_doctors_updated_at
    BEFORE UPDATE ON clinic_doctors
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  clinic_doctors                 IS 'Dentists / specialists associated with a clinic.';
COMMENT ON COLUMN clinic_doctors.available_days  IS 'Days of the week the doctor is present; matches timings keys.';
COMMENT ON COLUMN clinic_doctors.qualifications  IS 'Degree list, e.g. {BDS, MDS}.';


-- ── FAQs ────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS clinic_faqs (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id  UUID        NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    category   TEXT        NOT NULL DEFAULT 'general',  -- general | pricing | appointments | insurance
    question   TEXT        NOT NULL,
    answer     TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_clinic_faqs_clinic ON clinic_faqs(clinic_id);

CREATE TRIGGER clinic_faqs_updated_at
    BEFORE UPDATE ON clinic_faqs
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

COMMENT ON TABLE  clinic_faqs          IS 'Frequently asked questions per clinic.';
COMMENT ON COLUMN clinic_faqs.category IS 'general | pricing | appointments | insurance';


-- ── Knowledge chunks (unified RAG search target) ─────────────────────────────
-- Each embeddable record (FAQ answer, service description, doctor bio) produces
-- one row here.  The retriever queries only this table, keeping the search path
-- simple.  The metadata JSONB stores the original record for answer generation.
CREATE TABLE IF NOT EXISTS clinic_knowledge_chunks (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id   UUID        NOT NULL REFERENCES clinics(id) ON DELETE CASCADE,
    source_type TEXT        NOT NULL,   -- faq | service | doctor | general
    source_id   UUID        NOT NULL,   -- FK to the originating row (informational)
    content     TEXT        NOT NULL,   -- text that was embedded
    metadata    JSONB       NOT NULL DEFAULT '{}',
    embedding   vector(1536),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_clinic   ON clinic_knowledge_chunks(clinic_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_source   ON clinic_knowledge_chunks(clinic_id, source_type);
-- HNSW index for fast approximate nearest-neighbour cosine search
CREATE INDEX IF NOT EXISTS idx_knowledge_embedding
    ON clinic_knowledge_chunks
    USING hnsw (embedding vector_cosine_ops)
    WITH (m = 16, ef_construction = 64);

COMMENT ON TABLE  clinic_knowledge_chunks           IS 'Unified vector search table.  One chunk per embeddable knowledge piece.';
COMMENT ON COLUMN clinic_knowledge_chunks.source_type IS 'faq | service | doctor | general';
COMMENT ON COLUMN clinic_knowledge_chunks.metadata    IS 'Full original record stored as JSON so the retriever can return it without extra joins.';
