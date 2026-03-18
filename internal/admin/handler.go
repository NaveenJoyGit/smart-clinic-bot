package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
)

// Handler holds dependencies for all admin HTTP handlers.
type Handler struct {
	pool      *pgxpool.Pool
	indexer   *rag.Indexer
	jwtSecret string
	logger    *slog.Logger
}

// NewHandler constructs a Handler.
func NewHandler(pool *pgxpool.Pool, indexer *rag.Indexer, jwtSecret string, logger *slog.Logger) *Handler {
	return &Handler{pool: pool, indexer: indexer, jwtSecret: jwtSecret, logger: logger}
}

// ─── JSON helpers ────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pgErrCode(err error) string {
	var e *pgconn.PgError
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ─── Auth ────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login handles POST /admin/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	var (
		id           string
		name         string
		email        string
		passwordHash string
		role         string
		clinicID     *string
		isActive     bool
	)
	const q = `SELECT id::text, name, email, password_hash, role, clinic_id::text, is_active
	           FROM admin_users WHERE email = $1`
	err := h.pool.QueryRow(r.Context(), q, req.Email).
		Scan(&id, &name, &email, &passwordHash, &role, &clinicID, &isActive)
	if errors.Is(err, pgx.ErrNoRows) || !isActive {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "login query failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !checkPassword(req.Password, passwordHash) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	_, _ = h.pool.Exec(r.Context(), `UPDATE admin_users SET last_login_at = NOW() WHERE id = $1::uuid`, id)

	u := &AdminUser{ID: id, ClinicID: clinicID, Name: name, Email: email, Role: role}
	token, err := issueToken(h.jwtSecret, u)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "issue token failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"access_token": token, "expires_in": 86400})
}

// ─── Clinics ─────────────────────────────────────────────────────────────────

type clinicRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	Address   *string   `json:"address,omitempty"`
	City      *string   `json:"city,omitempty"`
	Phone     *string   `json:"phone,omitempty"`
	Email     *string   `json:"email,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// ListClinics handles GET /admin/clinics (super_admin only)
func (h *Handler) ListClinics(w http.ResponseWriter, r *http.Request) {
	u, _ := adminUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, name, slug, address, city, phone, email, is_active, created_at
	           FROM clinics ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()

	var list []clinicRow
	for rows.Next() {
		var c clinicRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		list = append(list, c)
	}
	if list == nil {
		list = []clinicRow{}
	}
	writeJSON(w, http.StatusOK, list)
}

type createClinicRequest struct {
	Name    string  `json:"name"`
	Slug    string  `json:"slug"`
	Address *string `json:"address"`
	City    *string `json:"city"`
	Phone   *string `json:"phone"`
	Email   *string `json:"email"`
}

// CreateClinic handles POST /admin/clinics (super_admin only)
func (h *Handler) CreateClinic(w http.ResponseWriter, r *http.Request) {
	u, _ := adminUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req createClinicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}
	const q = `INSERT INTO clinics (name, slug, address, city, phone, email)
	           VALUES ($1, $2, $3, $4, $5, $6)
	           RETURNING id::text, name, slug, address, city, phone, email, is_active, created_at`
	var c clinicRow
	err := h.pool.QueryRow(r.Context(), q, req.Name, req.Slug, req.Address, req.City, req.Phone, req.Email).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt)
	if pgErrCode(err) == "23505" {
		writeError(w, http.StatusConflict, "slug already exists")
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "create clinic failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// GetClinic handles GET /admin/clinics/{clinic_id}
func (h *Handler) GetClinic(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, name, slug, address, city, phone, email, is_active, created_at
	           FROM clinics WHERE id = $1::uuid`
	var c clinicRow
	err := h.pool.QueryRow(r.Context(), q, clinicID).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "clinic not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

type updateClinicRequest struct {
	Name    string  `json:"name"`
	Address *string `json:"address"`
	City    *string `json:"city"`
	Phone   *string `json:"phone"`
	Email   *string `json:"email"`
}

// UpdateClinic handles PUT /admin/clinics/{clinic_id}
func (h *Handler) UpdateClinic(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req updateClinicRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	const q = `UPDATE clinics SET name=$2, address=$3, city=$4, phone=$5, email=$6
	           WHERE id=$1::uuid
	           RETURNING id::text, name, slug, address, city, phone, email, is_active, created_at`
	var c clinicRow
	err := h.pool.QueryRow(r.Context(), q, clinicID, req.Name, req.Address, req.City, req.Phone, req.Email).
		Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "clinic not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, c)
}

// ─── FAQs ────────────────────────────────────────────────────────────────────

type faqRow struct {
	ID        string    `json:"id"`
	ClinicID  string    `json:"clinic_id"`
	Category  string    `json:"category"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListFAQs handles GET /admin/clinics/{clinic_id}/faqs
func (h *Handler) ListFAQs(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, clinic_id::text, category, question, answer, created_at, updated_at
	           FROM clinic_faqs WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	var list []faqRow
	for rows.Next() {
		var f faqRow
		if err := rows.Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer, &f.CreatedAt, &f.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		list = append(list, f)
	}
	if list == nil {
		list = []faqRow{}
	}
	writeJSON(w, http.StatusOK, list)
}

type faqRequest struct {
	Category string `json:"category"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// CreateFAQ handles POST /admin/clinics/{clinic_id}/faqs
func (h *Handler) CreateFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req faqRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Question == "" || req.Answer == "" {
		writeError(w, http.StatusBadRequest, "question and answer are required")
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}
	const q = `INSERT INTO clinic_faqs (clinic_id, category, question, answer)
	           VALUES ($1::uuid, $2, $3, $4)
	           RETURNING id::text, clinic_id::text, category, question, answer, created_at, updated_at`
	var f faqRow
	if err := h.pool.QueryRow(r.Context(), q, clinicID, req.Category, req.Question, req.Answer).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer, &f.CreatedAt, &f.UpdatedAt); err != nil {
		h.logger.ErrorContext(r.Context(), "create faq failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	content := fmt.Sprintf("Q: %s\nA: %s", f.Question, f.Answer)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "faq", f.ID, content, map[string]any{"category": f.Category}); err != nil {
		h.logger.WarnContext(r.Context(), "faq indexing failed", "faq_id", f.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, f)
}

// UpdateFAQ handles PUT /admin/clinics/{clinic_id}/faqs/{id}
func (h *Handler) UpdateFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req faqRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Question == "" || req.Answer == "" {
		writeError(w, http.StatusBadRequest, "question and answer are required")
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_faqs SET category=$2, question=$3, answer=$4, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$5::uuid
	                 RETURNING id::text, clinic_id::text, category, question, answer, created_at, updated_at`
	var f faqRow
	err = tx.QueryRow(r.Context(), updateQ, faqID, req.Category, req.Question, req.Answer, clinicID).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer, &f.CreatedAt, &f.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "faq not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	const deleteChunks = `DELETE FROM clinic_knowledge_chunks
	                      WHERE source_id=$1::uuid AND source_type='faq' AND clinic_id=$2::uuid`
	if _, err := tx.Exec(r.Context(), deleteChunks, faqID, clinicID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	content := fmt.Sprintf("Q: %s\nA: %s", f.Question, f.Answer)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "faq", f.ID, content, map[string]any{"category": f.Category}); err != nil {
		h.logger.WarnContext(r.Context(), "faq re-indexing failed", "faq_id", f.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, f)
}

// DeleteFAQ handles DELETE /admin/clinics/{clinic_id}/faqs/{id}
func (h *Handler) DeleteFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='faq' AND clinic_id=$2::uuid`,
		faqID, clinicID)

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_faqs WHERE id=$1::uuid AND clinic_id=$2::uuid`, faqID, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "faq not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ─── Services ────────────────────────────────────────────────────────────────

type serviceRow struct {
	ID          string    `json:"id"`
	ClinicID    string    `json:"clinic_id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Description *string   `json:"description,omitempty"`
	PriceMin    *float64  `json:"price_min,omitempty"`
	PriceMax    *float64  `json:"price_max,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ListServices handles GET /admin/clinics/{clinic_id}/services
func (h *Handler) ListServices(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, category, description, price_min, price_max, is_active, created_at, updated_at
	           FROM clinic_services WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	var list []serviceRow
	for rows.Next() {
		var s serviceRow
		if err := rows.Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		list = append(list, s)
	}
	if list == nil {
		list = []serviceRow{}
	}
	writeJSON(w, http.StatusOK, list)
}

type serviceRequest struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Description *string  `json:"description"`
	PriceMin    *float64 `json:"price_min"`
	PriceMax    *float64 `json:"price_max"`
}

// CreateService handles POST /admin/clinics/{clinic_id}/services
func (h *Handler) CreateService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req serviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}
	const q = `INSERT INTO clinic_services (clinic_id, name, category, description, price_min, price_max)
	           VALUES ($1::uuid, $2, $3, $4, $5, $6)
	           RETURNING id::text, clinic_id::text, name, category, description, price_min, price_max, is_active, created_at, updated_at`
	var s serviceRow
	if err := h.pool.QueryRow(r.Context(), q, clinicID, req.Name, req.Category, req.Description, req.PriceMin, req.PriceMax).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive, &s.CreatedAt, &s.UpdatedAt); err != nil {
		h.logger.ErrorContext(r.Context(), "create service failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	content := serviceContent(s.Name, nilStr(s.Description), nilFloat(s.PriceMin), nilFloat(s.PriceMax))
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "service", s.ID, content, map[string]any{"category": s.Category}); err != nil {
		h.logger.WarnContext(r.Context(), "service indexing failed", "service_id", s.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, s)
}

// UpdateService handles PUT /admin/clinics/{clinic_id}/services/{id}
func (h *Handler) UpdateService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req serviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Category == "" {
		req.Category = "general"
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_services SET name=$2, category=$3, description=$4, price_min=$5, price_max=$6, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$7::uuid
	                 RETURNING id::text, clinic_id::text, name, category, description, price_min, price_max, is_active, created_at, updated_at`
	var s serviceRow
	err = tx.QueryRow(r.Context(), updateQ, serviceID, req.Name, req.Category, req.Description, req.PriceMin, req.PriceMax, clinicID).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := tx.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='service' AND clinic_id=$2::uuid`,
		serviceID, clinicID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	content := serviceContent(s.Name, nilStr(s.Description), nilFloat(s.PriceMin), nilFloat(s.PriceMax))
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "service", s.ID, content, map[string]any{"category": s.Category}); err != nil {
		h.logger.WarnContext(r.Context(), "service re-indexing failed", "service_id", s.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, s)
}

// DeleteService handles DELETE /admin/clinics/{clinic_id}/services/{id}
func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='service' AND clinic_id=$2::uuid`,
		serviceID, clinicID)

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_services WHERE id=$1::uuid AND clinic_id=$2::uuid`, serviceID, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func serviceContent(name, description string, priceMin, priceMax float64) string {
	return fmt.Sprintf("%s: %s. Price: %.2f–%.2f.", name, description, priceMin, priceMax)
}

// ─── Doctors ─────────────────────────────────────────────────────────────────

type doctorRow struct {
	ID             string    `json:"id"`
	ClinicID       string    `json:"clinic_id"`
	Name           string    `json:"name"`
	Specialization *string   `json:"specialization,omitempty"`
	Qualifications []string  `json:"qualifications"`
	Bio            *string   `json:"bio,omitempty"`
	AvailableDays  []string  `json:"available_days"`
	Languages      []string  `json:"languages"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ListDoctors handles GET /admin/clinics/{clinic_id}/doctors
func (h *Handler) ListDoctors(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active, created_at, updated_at
	           FROM clinic_doctors WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	var list []doctorRow
	for rows.Next() {
		var d doctorRow
		if err := rows.Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		list = append(list, d)
	}
	if list == nil {
		list = []doctorRow{}
	}
	writeJSON(w, http.StatusOK, list)
}

type doctorRequest struct {
	Name           string   `json:"name"`
	Specialization *string  `json:"specialization"`
	Qualifications []string `json:"qualifications"`
	Bio            *string  `json:"bio"`
	AvailableDays  []string `json:"available_days"`
	Languages      []string `json:"languages"`
}

// CreateDoctor handles POST /admin/clinics/{clinic_id}/doctors
func (h *Handler) CreateDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req doctorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Qualifications == nil {
		req.Qualifications = []string{}
	}
	if req.AvailableDays == nil {
		req.AvailableDays = []string{}
	}
	if req.Languages == nil {
		req.Languages = []string{"English"}
	}
	const q = `INSERT INTO clinic_doctors (clinic_id, name, specialization, qualifications, bio, available_days, languages)
	           VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
	           RETURNING id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active, created_at, updated_at`
	var d doctorRow
	if err := h.pool.QueryRow(r.Context(), q, clinicID, req.Name, req.Specialization, req.Qualifications, req.Bio, req.AvailableDays, req.Languages).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive, &d.CreatedAt, &d.UpdatedAt); err != nil {
		h.logger.ErrorContext(r.Context(), "create doctor failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	content := doctorContent(d.Name, nilStr(d.Specialization), nilStr(d.Bio), d.AvailableDays)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "doctor", d.ID, content, nil); err != nil {
		h.logger.WarnContext(r.Context(), "doctor indexing failed", "doctor_id", d.ID, "error", err)
	}
	writeJSON(w, http.StatusCreated, d)
}

// UpdateDoctor handles PUT /admin/clinics/{clinic_id}/doctors/{id}
func (h *Handler) UpdateDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req doctorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Qualifications == nil {
		req.Qualifications = []string{}
	}
	if req.AvailableDays == nil {
		req.AvailableDays = []string{}
	}
	if req.Languages == nil {
		req.Languages = []string{"English"}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_doctors SET name=$2, specialization=$3, qualifications=$4, bio=$5, available_days=$6, languages=$7, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$8::uuid
	                 RETURNING id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active, created_at, updated_at`
	var d doctorRow
	err = tx.QueryRow(r.Context(), updateQ, doctorID, req.Name, req.Specialization, req.Qualifications, req.Bio, req.AvailableDays, req.Languages, clinicID).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "doctor not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := tx.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='doctor' AND clinic_id=$2::uuid`,
		doctorID, clinicID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	content := doctorContent(d.Name, nilStr(d.Specialization), nilStr(d.Bio), d.AvailableDays)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "doctor", d.ID, content, nil); err != nil {
		h.logger.WarnContext(r.Context(), "doctor re-indexing failed", "doctor_id", d.ID, "error", err)
	}
	writeJSON(w, http.StatusOK, d)
}

// DeleteDoctor handles DELETE /admin/clinics/{clinic_id}/doctors/{id}
func (h *Handler) DeleteDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := authorizeClinic(r.Context(), clinicID); err != nil {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='doctor' AND clinic_id=$2::uuid`,
		doctorID, clinicID)

	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_doctors WHERE id=$1::uuid AND clinic_id=$2::uuid`, doctorID, clinicID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "doctor not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func doctorContent(name, specialization, bio string, availableDays []string) string {
	return fmt.Sprintf("Dr. %s (%s). %s. Available: %s.", name, specialization, bio, strings.Join(availableDays, ", "))
}

// ─── Admin users ─────────────────────────────────────────────────────────────

type createAdminUserRequest struct {
	Name     string  `json:"name"`
	Email    string  `json:"email"`
	Password string  `json:"password"`
	Role     string  `json:"role"`
	ClinicID *string `json:"clinic_id"`
}

// CreateAdminUser handles POST /admin/users (super_admin only)
func (h *Handler) CreateAdminUser(w http.ResponseWriter, r *http.Request) {
	caller, _ := adminUserFromCtx(r.Context())
	if !caller.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	var req createAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" || req.Password == "" || req.Role == "" {
		writeError(w, http.StatusBadRequest, "name, email, password, and role are required")
		return
	}
	if req.Role != "super_admin" && req.Role != "clinic_admin" {
		writeError(w, http.StatusBadRequest, "role must be super_admin or clinic_admin")
		return
	}
	if req.Role == "clinic_admin" && req.ClinicID == nil {
		writeError(w, http.StatusBadRequest, "clinic_id is required for clinic_admin")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	const q = `INSERT INTO admin_users (clinic_id, name, email, password_hash, role)
	           VALUES ($1::uuid, $2, $3, $4, $5)
	           RETURNING id::text`
	var id string
	err = h.pool.QueryRow(r.Context(), q, req.ClinicID, req.Name, req.Email, hash, req.Role).Scan(&id)
	if pgErrCode(err) == "23505" {
		writeError(w, http.StatusConflict, "email already exists")
		return
	}
	if err != nil {
		h.logger.ErrorContext(r.Context(), "create admin user failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": req.Name, "email": req.Email, "role": req.Role, "clinic_id": req.ClinicID,
	})
}

type adminUserRow struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	ClinicID    *string    `json:"clinic_id,omitempty"`
	IsActive    bool       `json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
}

// ListAdminUsers handles GET /admin/users (super_admin only)
func (h *Handler) ListAdminUsers(w http.ResponseWriter, r *http.Request) {
	caller, _ := adminUserFromCtx(r.Context())
	if !caller.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	const q = `SELECT id::text, name, email, role, clinic_id::text, is_active, last_login_at
	           FROM admin_users ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	defer rows.Close()
	var list []adminUserRow
	for rows.Next() {
		var u adminUserRow
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.ClinicID, &u.IsActive, &u.LastLoginAt); err != nil {
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		list = append(list, u)
	}
	if list == nil {
		list = []adminUserRow{}
	}
	writeJSON(w, http.StatusOK, list)
}

// DeactivateAdminUser handles DELETE /admin/users/{id} (super_admin only)
func (h *Handler) DeactivateAdminUser(w http.ResponseWriter, r *http.Request) {
	caller, _ := adminUserFromCtx(r.Context())
	if !caller.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE admin_users SET is_active=FALSE WHERE id=$1::uuid`, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deactivated": true})
}

type resetPasswordRequest struct {
	Password string `json:"password"`
}

// ResetAdminPassword handles POST /admin/users/{id}/reset-password (super_admin only)
func (h *Handler) ResetAdminPassword(w http.ResponseWriter, r *http.Request) {
	caller, _ := adminUserFromCtx(r.Context())
	if !caller.IsSuperAdmin() {
		writeError(w, http.StatusForbidden, "forbidden")
		return
	}
	userID := chi.URLParam(r, "id")
	var req resetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE admin_users SET password_hash=$2 WHERE id=$1::uuid`, userID, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"reset": true})
}

// ─── nil helpers ─────────────────────────────────────────────────────────────

func nilStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func nilFloat(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
