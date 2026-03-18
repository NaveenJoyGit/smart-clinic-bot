CREATE TABLE IF NOT EXISTS admin_users (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    clinic_id      UUID        REFERENCES clinics(id) ON DELETE CASCADE,  -- NULL = super_admin
    name           TEXT        NOT NULL,
    email          TEXT        NOT NULL UNIQUE,
    password_hash  TEXT        NOT NULL,  -- bcrypt hash
    role           TEXT        NOT NULL CHECK (role IN ('super_admin', 'clinic_admin')),
    is_active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_admin_users_email  ON admin_users(email);
CREATE INDEX IF NOT EXISTS idx_admin_users_clinic ON admin_users(clinic_id);

COMMENT ON TABLE  admin_users               IS 'Admin users for the management API.';
COMMENT ON COLUMN admin_users.clinic_id     IS 'NULL for super_admin; clinic UUID for clinic_admin.';
COMMENT ON COLUMN admin_users.password_hash IS 'bcrypt hash of the admin password.';
