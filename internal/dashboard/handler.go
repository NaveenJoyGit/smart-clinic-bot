package dashboard

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/admin"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
	"github.com/naveenjoy/smart-clinic-bot/web"
	"golang.org/x/crypto/bcrypt"
)

// ─── Handler ─────────────────────────────────────────────────────────────────

// Handler holds all dashboard dependencies.
type Handler struct {
	pool      *pgxpool.Pool
	indexer   *rag.Indexer
	jwtSecret string
	logger    *slog.Logger
	pages     map[string]*template.Template
	frags     *template.Template
}

// PageData is the top-level data passed to every page template.
type PageData struct {
	User     *admin.AdminUser
	ClinicID *string // set for clinic-scoped pages; nil = super_admin on overview
	Data     any
	Error    string
	Flash    string
}

// ─── Page-specific data types ─────────────────────────────────────────────────

type overviewData struct {
	TotalConversations  int
	ActiveConversations int
	TelegramConv        int
	WhatsAppConv        int
	StatusCounts        map[string]int
	RecentAppointments  []recentApptItem
}

type recentApptItem struct {
	PatientName   string
	PreferredDate *time.Time
	PreferredTime *string
	Status        string
	ClinicName    string
	CreatedAt     time.Time
}

type faqPageData struct {
	ClinicID string
	FAQs     []faqItem
}

type faqItem struct {
	ID       string
	ClinicID string
	Category string
	Question string
	Answer   string
}

type servicePageData struct {
	ClinicID string
	Services []serviceItem
}

type serviceItem struct {
	ID          string
	ClinicID    string
	Name        string
	Category    string
	Description *string
	PriceMin    *float64
	PriceMax    *float64
	IsActive    bool
}

type doctorPageData struct {
	ClinicID string
	Doctors  []doctorItem
}

type doctorItem struct {
	ID             string
	ClinicID       string
	Name           string
	Specialization *string
	Bio            *string
	Qualifications []string
	AvailableDays  []string
	Languages      []string
	IsActive       bool
}

type apptPageData struct {
	ClinicID     string
	Appointments []apptItem
	StatusFilter string
}

type apptItem struct {
	ID            string
	ClinicID      string
	PatientName   string
	PatientPhone  string
	PreferredDate *time.Time
	PreferredTime *string
	Status        string
	Notes         *string
	CreatedAt     time.Time
}

type convPageData struct {
	ClinicID      string
	Conversations []convItem
}

type convItem struct {
	ID          string
	Platform    string
	ExternalID  string
	Status      string
	PatientName *string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type clinicPageData struct {
	Clinics []clinicItem
}

type clinicItem struct {
	ID        string
	Name      string
	Slug      string
	Address   *string
	City      *string
	Phone     *string
	Email     *string
	IsActive  bool
	CreatedAt time.Time
}

type userPageData struct {
	Users []userItem
}

type userItem struct {
	ID          string
	Name        string
	Email       string
	Role        string
	ClinicID    *string
	IsActive    bool
	LastLoginAt *time.Time
}

type toastPayload struct {
	Flash string
	Error string
}

// ─── Template functions ────────────────────────────────────────────────────────

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"derefFloat": func(f *float64) float64 {
			if f == nil {
				return 0
			}
			return *f
		},
		"join": func(s []string) string {
			return strings.Join(s, ", ")
		},
		"formatDate": func(t time.Time) string {
			return t.Format("Jan 2, 2006")
		},
		"formatNullDate": func(t *time.Time) string {
			if t == nil {
				return ""
			}
			return t.Format("Jan 2, 2006")
		},
		"statusClass": func(s string) string {
			switch s {
			case "pending":
				return "secondary"
			case "confirmed", "active":
				return "primary"
			case "cancelled", "no_show":
				return "contrast"
			default:
				return ""
			}
		},
	}
}

// ─── Constructor ──────────────────────────────────────────────────────────────

// NewHandler builds and template-parses the dashboard handler.
func NewHandler(pool *pgxpool.Pool, indexer *rag.Indexer, jwtSecret string, logger *slog.Logger) (*Handler, error) {
	funcs := templateFuncs()

	// login is standalone — no layout wrapper
	loginTmpl, err := template.New("login.html").Funcs(funcs).ParseFS(web.FS, "templates/login.html")
	if err != nil {
		return nil, fmt.Errorf("parse login template: %w", err)
	}

	// page templates: layout + page content + all partials
	pageNames := []string{"overview", "faqs", "services", "doctors", "clinics", "users", "appointments", "conversations"}
	pages := make(map[string]*template.Template, len(pageNames)+1)
	pages["login"] = loginTmpl

	for _, name := range pageNames {
		t, err := template.New("layout.html").Funcs(funcs).ParseFS(web.FS,
			"templates/layout.html",
			"templates/"+name+".html",
			"templates/partials/*.html",
		)
		if err != nil {
			return nil, fmt.Errorf("parse template %s: %w", name, err)
		}
		pages[name] = t
	}

	// partials-only set for fragment responses
	frags, err := template.New("").Funcs(funcs).ParseFS(web.FS, "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse partials: %w", err)
	}

	return &Handler{
		pool:      pool,
		indexer:   indexer,
		jwtSecret: jwtSecret,
		logger:    logger,
		pages:     pages,
		frags:     frags,
	}, nil
}

// ─── Render helpers ────────────────────────────────────────────────────────────

// renderPage renders a full page or just the {{define "content"}} block for HTMX navigations.
func (h *Handler) renderPage(w http.ResponseWriter, r *http.Request, page string, data PageData) {
	t, ok := h.pages[page]
	if !ok {
		http.Error(w, "template not found: "+page, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var execErr error
	if r.Header.Get("HX-Request") != "" {
		execErr = t.ExecuteTemplate(w, "content", data)
	} else {
		execErr = t.Execute(w, data)
	}
	if execErr != nil {
		h.logger.Error("render page", "page", page, "error", execErr)
	}
}

// renderFrag writes a single named partial template.
func (h *Handler) renderFrag(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.frags.ExecuteTemplate(w, name, data); err != nil {
		h.logger.Error("render frag", "name", name, "error", err)
	}
}

// renderToast appends an OOB toast to the current response (must call after main frag).
func (h *Handler) renderToast(w http.ResponseWriter, flash, errMsg string) {
	if err := h.frags.ExecuteTemplate(w, "toast", toastPayload{Flash: flash, Error: errMsg}); err != nil {
		h.logger.Error("render toast", "error", err)
	}
}

// ─── Auth handlers ────────────────────────────────────────────────────────────

// GetLogin renders GET /dashboard/login.
func (h *Handler) GetLogin(w http.ResponseWriter, r *http.Request) {
	// Redirect already-authenticated users.
	if cookie, err := r.Cookie("admin_session"); err == nil {
		if _, err := verifyDashboardToken(h.jwtSecret, cookie.Value); err == nil {
			http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.pages["login"].Execute(w, PageData{})
}

// PostLogin handles POST /dashboard/login.
func (h *Handler) PostLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.pages["login"].Execute(w, PageData{Error: "Invalid form submission"})
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	renderErr := func(msg string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = h.pages["login"].Execute(w, PageData{Error: msg})
	}

	if email == "" || password == "" {
		renderErr("Email and password are required")
		return
	}

	var (
		id           string
		name         string
		passwordHash string
		role         string
		clinicID     *string
		isActive     bool
	)
	const q = `SELECT id::text, name, email, password_hash, role, clinic_id::text, is_active
	           FROM admin_users WHERE email = $1`
	err := h.pool.QueryRow(r.Context(), q, email).
		Scan(&id, &name, &email, &passwordHash, &role, &clinicID, &isActive)
	if errors.Is(err, pgx.ErrNoRows) || !isActive {
		renderErr("Invalid credentials")
		return
	}
	if err != nil {
		h.logger.Error("login query", "error", err)
		renderErr("Internal error")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)) != nil {
		renderErr("Invalid credentials")
		return
	}

	_, _ = h.pool.Exec(r.Context(), `UPDATE admin_users SET last_login_at = NOW() WHERE id = $1::uuid`, id)

	claims := admin.Claims{
		AdminID:  id,
		Email:    email,
		Role:     role,
		ClinicID: clinicID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.jwtSecret))
	if err != nil {
		h.logger.Error("issue token", "error", err)
		renderErr("Internal error")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "admin_session",
		Value:    token,
		Path:     "/dashboard",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	http.Redirect(w, r, "/dashboard/", http.StatusSeeOther)
}

// PostLogout handles POST /dashboard/logout.
func (h *Handler) PostLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:   "admin_session",
		MaxAge: -1,
		Path:   "/dashboard",
	})
	http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
}

// ─── Overview ─────────────────────────────────────────────────────────────────

// GetOverview handles GET /dashboard/.
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	u, _ := dashUserFromCtx(r.Context())

	var clinicIDPtr *string
	if !u.IsSuperAdmin() {
		clinicIDPtr = u.ClinicID
	}

	data := overviewData{StatusCounts: make(map[string]int)}

	// Conversation stats
	const convQ = `SELECT
	  COUNT(*)                                      AS total,
	  COUNT(*) FILTER (WHERE status = 'active')     AS active,
	  COUNT(*) FILTER (WHERE platform = 'telegram') AS telegram,
	  COUNT(*) FILTER (WHERE platform = 'whatsapp') AS whatsapp
	FROM conversations
	WHERE ($1::uuid IS NULL OR clinic_id = $1::uuid)`
	_ = h.pool.QueryRow(r.Context(), convQ, clinicIDPtr).
		Scan(&data.TotalConversations, &data.ActiveConversations, &data.TelegramConv, &data.WhatsAppConv)

	// Appointment status counts
	const apptStatusQ = `SELECT status, COUNT(*) AS cnt
	FROM appointment_requests
	WHERE ($1::uuid IS NULL OR clinic_id = $1::uuid)
	GROUP BY status`
	rows, err := h.pool.Query(r.Context(), apptStatusQ, clinicIDPtr)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			var cnt int
			if err := rows.Scan(&status, &cnt); err == nil {
				data.StatusCounts[status] = cnt
			}
		}
	}

	// Recent 10 appointments
	const recentQ = `SELECT ar.patient_name, ar.preferred_date, ar.preferred_time,
	       ar.status, c.name AS clinic_name, ar.created_at
	FROM appointment_requests ar
	JOIN clinics c ON c.id = ar.clinic_id
	WHERE ($1::uuid IS NULL OR ar.clinic_id = $1::uuid)
	ORDER BY ar.created_at DESC LIMIT 10`
	rrows, err := h.pool.Query(r.Context(), recentQ, clinicIDPtr)
	if err == nil {
		defer rrows.Close()
		for rrows.Next() {
			var item recentApptItem
			if err := rrows.Scan(&item.PatientName, &item.PreferredDate, &item.PreferredTime,
				&item.Status, &item.ClinicName, &item.CreatedAt); err == nil {
				data.RecentAppointments = append(data.RecentAppointments, item)
			}
		}
	}

	h.renderPage(w, r, "overview", PageData{
		User:     u,
		ClinicID: clinicIDPtr,
		Data:     data,
	})
}

// ─── FAQs ─────────────────────────────────────────────────────────────────────

// GetFAQs handles GET /dashboard/clinics/{clinic_id}/faqs.
func (h *Handler) GetFAQs(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	u, _ := dashUserFromCtx(r.Context())
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, category, question, answer
	           FROM clinic_faqs WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		h.logger.Error("list faqs", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []faqItem
	for rows.Next() {
		var f faqItem
		if scanErr := rows.Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer); scanErr != nil {
			h.logger.Error("scan faq", "error", scanErr)
			continue
		}
		list = append(list, f)
	}
	h.renderPage(w, r, "faqs", PageData{
		User:     u,
		ClinicID: &clinicID,
		Data:     faqPageData{ClinicID: clinicID, FAQs: list},
	})
}

// PostFAQ handles POST /dashboard/clinics/{clinic_id}/faqs.
func (h *Handler) PostFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	question := strings.TrimSpace(r.FormValue("question"))
	answer := strings.TrimSpace(r.FormValue("answer"))
	if question == "" || answer == "" {
		h.renderToast(w, "", "Question and answer are required")
		return
	}
	if category == "" {
		category = "general"
	}
	const q = `INSERT INTO clinic_faqs (clinic_id, category, question, answer)
	           VALUES ($1::uuid, $2, $3, $4)
	           RETURNING id::text, clinic_id::text, category, question, answer`
	var f faqItem
	if err := h.pool.QueryRow(r.Context(), q, clinicID, category, question, answer).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer); err != nil {
		h.logger.Error("create faq", "error", err)
		h.renderToast(w, "", "Failed to create FAQ")
		return
	}
	content := fmt.Sprintf("Q: %s\nA: %s", f.Question, f.Answer)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "faq", f.ID, content, map[string]any{"category": f.Category}); err != nil {
		h.logger.Warn("faq indexing failed", "faq_id", f.ID, "error", err)
	}
	h.renderFrag(w, "faq_row", f)
	h.renderToast(w, "FAQ created", "")
}

// GetFAQRow handles GET /dashboard/clinics/{clinic_id}/faqs/{id} (for cancel).
func (h *Handler) GetFAQRow(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, category, question, answer
	           FROM clinic_faqs WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var f faqItem
	if err := h.pool.QueryRow(r.Context(), q, faqID, clinicID).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "faq_row", f)
}

// GetFAQEdit handles GET /dashboard/clinics/{clinic_id}/faqs/{id}/edit.
func (h *Handler) GetFAQEdit(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, category, question, answer
	           FROM clinic_faqs WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var f faqItem
	if err := h.pool.QueryRow(r.Context(), q, faqID, clinicID).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "faq_edit_row", f)
}

// PutFAQ handles PUT /dashboard/clinics/{clinic_id}/faqs/{id}.
func (h *Handler) PutFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	question := strings.TrimSpace(r.FormValue("question"))
	answer := strings.TrimSpace(r.FormValue("answer"))
	if question == "" || answer == "" {
		http.Error(w, "question and answer are required", http.StatusBadRequest)
		return
	}
	if category == "" {
		category = "general"
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_faqs SET category=$2, question=$3, answer=$4, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$5::uuid
	                 RETURNING id::text, clinic_id::text, category, question, answer`
	var f faqItem
	err = tx.QueryRow(r.Context(), updateQ, faqID, category, question, answer, clinicID).
		Scan(&f.ID, &f.ClinicID, &f.Category, &f.Question, &f.Answer)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("update faq", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = tx.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='faq' AND clinic_id=$2::uuid`,
		faqID, clinicID)
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	content := fmt.Sprintf("Q: %s\nA: %s", f.Question, f.Answer)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "faq", f.ID, content, map[string]any{"category": f.Category}); err != nil {
		h.logger.Warn("faq re-indexing failed", "faq_id", f.ID, "error", err)
	}
	h.renderFrag(w, "faq_row", f)
	h.renderToast(w, "FAQ updated", "")
}

// DeleteFAQ handles DELETE /dashboard/clinics/{clinic_id}/faqs/{id}.
func (h *Handler) DeleteFAQ(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	faqID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='faq' AND clinic_id=$2::uuid`,
		faqID, clinicID)
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_faqs WHERE id=$1::uuid AND clinic_id=$2::uuid`, faqID, clinicID)
	if err != nil {
		h.logger.Error("delete faq", "error", err)
		h.renderToast(w, "", "Failed to delete FAQ")
		return
	}
	if tag.RowsAffected() == 0 {
		h.renderToast(w, "", "FAQ not found")
		return
	}
	h.renderToast(w, "FAQ deleted", "")
}

// ─── Services ─────────────────────────────────────────────────────────────────

// GetServices handles GET /dashboard/clinics/{clinic_id}/services.
func (h *Handler) GetServices(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	u, _ := dashUserFromCtx(r.Context())
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, category, description, price_min, price_max, is_active
	           FROM clinic_services WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []serviceItem
	for rows.Next() {
		var s serviceItem
		if err := rows.Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive); err != nil {
			continue
		}
		list = append(list, s)
	}
	h.renderPage(w, r, "services", PageData{
		User:     u,
		ClinicID: &clinicID,
		Data:     servicePageData{ClinicID: clinicID, Services: list},
	})
}

// PostService handles POST /dashboard/clinics/{clinic_id}/services.
func (h *Handler) PostService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderToast(w, "", "Name is required")
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		category = "general"
	}
	desc := optString(r.FormValue("description"))
	priceMin := parseOptFloat(r.FormValue("price_min"))
	priceMax := parseOptFloat(r.FormValue("price_max"))

	const q = `INSERT INTO clinic_services (clinic_id, name, category, description, price_min, price_max)
	           VALUES ($1::uuid, $2, $3, $4, $5, $6)
	           RETURNING id::text, clinic_id::text, name, category, description, price_min, price_max, is_active`
	var s serviceItem
	if err := h.pool.QueryRow(r.Context(), q, clinicID, name, category, desc, priceMin, priceMax).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive); err != nil {
		h.logger.Error("create service", "error", err)
		h.renderToast(w, "", "Failed to create service")
		return
	}
	content := serviceContent(s.Name, nilStr(s.Description), nilFloat(s.PriceMin), nilFloat(s.PriceMax))
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "service", s.ID, content, map[string]any{"category": s.Category}); err != nil {
		h.logger.Warn("service indexing failed", "service_id", s.ID, "error", err)
	}
	h.renderFrag(w, "service_row", s)
	h.renderToast(w, "Service created", "")
}

// GetServiceRow handles GET /dashboard/clinics/{clinic_id}/services/{id} (cancel).
func (h *Handler) GetServiceRow(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, category, description, price_min, price_max, is_active
	           FROM clinic_services WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var s serviceItem
	if err := h.pool.QueryRow(r.Context(), q, serviceID, clinicID).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "service_row", s)
}

// GetServiceEdit handles GET /dashboard/clinics/{clinic_id}/services/{id}/edit.
func (h *Handler) GetServiceEdit(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, category, description, price_min, price_max, is_active
	           FROM clinic_services WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var s serviceItem
	if err := h.pool.QueryRow(r.Context(), q, serviceID, clinicID).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "service_edit_row", s)
}

// PutService handles PUT /dashboard/clinics/{clinic_id}/services/{id}.
func (h *Handler) PutService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	category := strings.TrimSpace(r.FormValue("category"))
	if category == "" {
		category = "general"
	}
	desc := optString(r.FormValue("description"))
	priceMin := parseOptFloat(r.FormValue("price_min"))
	priceMax := parseOptFloat(r.FormValue("price_max"))

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_services SET name=$2, category=$3, description=$4, price_min=$5, price_max=$6, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$7::uuid
	                 RETURNING id::text, clinic_id::text, name, category, description, price_min, price_max, is_active`
	var s serviceItem
	err = tx.QueryRow(r.Context(), updateQ, serviceID, name, category, desc, priceMin, priceMax, clinicID).
		Scan(&s.ID, &s.ClinicID, &s.Name, &s.Category, &s.Description, &s.PriceMin, &s.PriceMax, &s.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("update service", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = tx.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='service' AND clinic_id=$2::uuid`,
		serviceID, clinicID)
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	content := serviceContent(s.Name, nilStr(s.Description), nilFloat(s.PriceMin), nilFloat(s.PriceMax))
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "service", s.ID, content, map[string]any{"category": s.Category}); err != nil {
		h.logger.Warn("service re-indexing failed", "service_id", s.ID, "error", err)
	}
	h.renderFrag(w, "service_row", s)
	h.renderToast(w, "Service updated", "")
}

// DeleteService handles DELETE /dashboard/clinics/{clinic_id}/services/{id}.
func (h *Handler) DeleteService(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	serviceID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='service' AND clinic_id=$2::uuid`,
		serviceID, clinicID)
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_services WHERE id=$1::uuid AND clinic_id=$2::uuid`, serviceID, clinicID)
	if err != nil {
		h.logger.Error("delete service", "error", err)
		h.renderToast(w, "", "Failed to delete service")
		return
	}
	if tag.RowsAffected() == 0 {
		h.renderToast(w, "", "Service not found")
		return
	}
	h.renderToast(w, "Service deleted", "")
}

// ─── Doctors ──────────────────────────────────────────────────────────────────

// GetDoctors handles GET /dashboard/clinics/{clinic_id}/doctors.
func (h *Handler) GetDoctors(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	u, _ := dashUserFromCtx(r.Context())
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active
	           FROM clinic_doctors WHERE clinic_id = $1::uuid ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []doctorItem
	for rows.Next() {
		var d doctorItem
		if err := rows.Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive); err != nil {
			continue
		}
		list = append(list, d)
	}
	h.renderPage(w, r, "doctors", PageData{
		User:     u,
		ClinicID: &clinicID,
		Data:     doctorPageData{ClinicID: clinicID, Doctors: list},
	})
}

// PostDoctor handles POST /dashboard/clinics/{clinic_id}/doctors.
func (h *Handler) PostDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		h.renderToast(w, "", "Name is required")
		return
	}
	spec := optString(r.FormValue("specialization"))
	bio := optString(r.FormValue("bio"))
	qualifications := parseCSV(r.FormValue("qualifications"))
	availableDays := parseCSV(r.FormValue("available_days"))
	languages := parseCSV(r.FormValue("languages"))
	if len(languages) == 0 {
		languages = []string{"English"}
	}

	const q = `INSERT INTO clinic_doctors (clinic_id, name, specialization, qualifications, bio, available_days, languages)
	           VALUES ($1::uuid, $2, $3, $4, $5, $6, $7)
	           RETURNING id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active`
	var d doctorItem
	if err := h.pool.QueryRow(r.Context(), q, clinicID, name, spec, qualifications, bio, availableDays, languages).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive); err != nil {
		h.logger.Error("create doctor", "error", err)
		h.renderToast(w, "", "Failed to create doctor")
		return
	}
	content := doctorContent(d.Name, nilStr(d.Specialization), nilStr(d.Bio), d.AvailableDays)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "doctor", d.ID, content, nil); err != nil {
		h.logger.Warn("doctor indexing failed", "doctor_id", d.ID, "error", err)
	}
	h.renderFrag(w, "doctor_row", d)
	h.renderToast(w, "Doctor added", "")
}

// GetDoctorRow handles GET /dashboard/clinics/{clinic_id}/doctors/{id} (cancel).
func (h *Handler) GetDoctorRow(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active
	           FROM clinic_doctors WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var d doctorItem
	if err := h.pool.QueryRow(r.Context(), q, doctorID, clinicID).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "doctor_row", d)
}

// GetDoctorEdit handles GET /dashboard/clinics/{clinic_id}/doctors/{id}/edit.
func (h *Handler) GetDoctorEdit(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active
	           FROM clinic_doctors WHERE id = $1::uuid AND clinic_id = $2::uuid`
	var d doctorItem
	if err := h.pool.QueryRow(r.Context(), q, doctorID, clinicID).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderFrag(w, "doctor_edit_row", d)
}

// PutDoctor handles PUT /dashboard/clinics/{clinic_id}/doctors/{id}.
func (h *Handler) PutDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	spec := optString(r.FormValue("specialization"))
	bio := optString(r.FormValue("bio"))
	qualifications := parseCSV(r.FormValue("qualifications"))
	availableDays := parseCSV(r.FormValue("available_days"))
	languages := parseCSV(r.FormValue("languages"))
	if len(languages) == 0 {
		languages = []string{"English"}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	const updateQ = `UPDATE clinic_doctors SET name=$2, specialization=$3, qualifications=$4, bio=$5, available_days=$6, languages=$7, updated_at=NOW()
	                 WHERE id=$1::uuid AND clinic_id=$8::uuid
	                 RETURNING id::text, clinic_id::text, name, specialization, qualifications, bio, available_days, languages, is_active`
	var d doctorItem
	err = tx.QueryRow(r.Context(), updateQ, doctorID, name, spec, qualifications, bio, availableDays, languages, clinicID).
		Scan(&d.ID, &d.ClinicID, &d.Name, &d.Specialization, &d.Qualifications, &d.Bio, &d.AvailableDays, &d.Languages, &d.IsActive)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("update doctor", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	_, _ = tx.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='doctor' AND clinic_id=$2::uuid`,
		doctorID, clinicID)
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	content := doctorContent(d.Name, nilStr(d.Specialization), nilStr(d.Bio), d.AvailableDays)
	if err := h.indexer.IndexDocument(r.Context(), clinicID, "doctor", d.ID, content, nil); err != nil {
		h.logger.Warn("doctor re-indexing failed", "doctor_id", d.ID, "error", err)
	}
	h.renderFrag(w, "doctor_row", d)
	h.renderToast(w, "Doctor updated", "")
}

// DeleteDoctor handles DELETE /dashboard/clinics/{clinic_id}/doctors/{id}.
func (h *Handler) DeleteDoctor(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	doctorID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	_, _ = h.pool.Exec(r.Context(),
		`DELETE FROM clinic_knowledge_chunks WHERE source_id=$1::uuid AND source_type='doctor' AND clinic_id=$2::uuid`,
		doctorID, clinicID)
	tag, err := h.pool.Exec(r.Context(),
		`DELETE FROM clinic_doctors WHERE id=$1::uuid AND clinic_id=$2::uuid`, doctorID, clinicID)
	if err != nil {
		h.logger.Error("delete doctor", "error", err)
		h.renderToast(w, "", "Failed to delete doctor")
		return
	}
	if tag.RowsAffected() == 0 {
		h.renderToast(w, "", "Doctor not found")
		return
	}
	h.renderToast(w, "Doctor deleted", "")
}

// ─── Appointments ─────────────────────────────────────────────────────────────

// GetAppointments handles GET /dashboard/clinics/{clinic_id}/appointments.
func (h *Handler) GetAppointments(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	u, _ := dashUserFromCtx(r.Context())
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	statusFilter := r.URL.Query().Get("status")

	list, err := h.queryAppointments(r, clinicID, statusFilter)
	if err != nil {
		h.logger.Error("list appointments", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderPage(w, r, "appointments", PageData{
		User:     u,
		ClinicID: &clinicID,
		Data:     apptPageData{ClinicID: clinicID, Appointments: list, StatusFilter: statusFilter},
	})
}

func (h *Handler) queryAppointments(r *http.Request, clinicID, statusFilter string) ([]apptItem, error) {
	const baseQ = `SELECT id::text, patient_name, patient_phone, preferred_date, preferred_time, status, notes, created_at
	               FROM appointment_requests WHERE clinic_id = $1::uuid`
	var (
		rows pgx.Rows
		err  error
	)
	if statusFilter != "" {
		rows, err = h.pool.Query(r.Context(), baseQ+" AND status = $2 ORDER BY created_at DESC", clinicID, statusFilter)
	} else {
		rows, err = h.pool.Query(r.Context(), baseQ+" ORDER BY created_at DESC", clinicID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []apptItem
	for rows.Next() {
		var a apptItem
		a.ClinicID = clinicID
		if scanErr := rows.Scan(&a.ID, &a.PatientName, &a.PatientPhone, &a.PreferredDate, &a.PreferredTime, &a.Status, &a.Notes, &a.CreatedAt); scanErr != nil {
			continue
		}
		list = append(list, a)
	}
	return list, rows.Err()
}

// PutAppointmentStatus handles PUT /dashboard/clinics/{clinic_id}/appointments/{id}/status.
func (h *Handler) PutAppointmentStatus(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	apptID := chi.URLParam(r, "id")
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := r.FormValue("status")
	valid := map[string]bool{"pending": true, "confirmed": true, "cancelled": true, "completed": true, "no_show": true}
	if !valid[status] {
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	const q = `UPDATE appointment_requests SET status=$2, updated_at=NOW()
	           WHERE id=$1::uuid AND clinic_id=$3::uuid
	           RETURNING id::text, patient_name, patient_phone, preferred_date, preferred_time, status, notes, created_at`
	var a apptItem
	a.ClinicID = clinicID
	err := h.pool.QueryRow(r.Context(), q, apptID, status, clinicID).
		Scan(&a.ID, &a.PatientName, &a.PatientPhone, &a.PreferredDate, &a.PreferredTime, &a.Status, &a.Notes, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		h.logger.Error("update appt status", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderFrag(w, "appt_row", a)
	h.renderToast(w, "Status updated", "")
}

// ─── Conversations ────────────────────────────────────────────────────────────

// GetConversations handles GET /dashboard/clinics/{clinic_id}/conversations.
func (h *Handler) GetConversations(w http.ResponseWriter, r *http.Request) {
	clinicID := chi.URLParam(r, "clinic_id")
	u, _ := dashUserFromCtx(r.Context())
	if err := dashAuthorizeClinic(r.Context(), clinicID); err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, platform, external_id, status, patient_name, created_at, updated_at
	           FROM conversations WHERE clinic_id = $1::uuid ORDER BY updated_at DESC LIMIT 100`
	rows, err := h.pool.Query(r.Context(), q, clinicID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []convItem
	for rows.Next() {
		var c convItem
		if err := rows.Scan(&c.ID, &c.Platform, &c.ExternalID, &c.Status, &c.PatientName, &c.CreatedAt, &c.UpdatedAt); err != nil {
			continue
		}
		list = append(list, c)
	}
	h.renderPage(w, r, "conversations", PageData{
		User:     u,
		ClinicID: &clinicID,
		Data:     convPageData{ClinicID: clinicID, Conversations: list},
	})
}

// ─── Clinics (super_admin) ────────────────────────────────────────────────────

// GetClinics handles GET /dashboard/clinics.
func (h *Handler) GetClinics(w http.ResponseWriter, r *http.Request) {
	u, _ := dashUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, name, slug, address, city, phone, email, is_active, created_at
	           FROM clinics ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []clinicItem
	for rows.Next() {
		var c clinicItem
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt); err != nil {
			continue
		}
		list = append(list, c)
	}
	h.renderPage(w, r, "clinics", PageData{
		User: u,
		Data: clinicPageData{Clinics: list},
	})
}

// PostClinic handles POST /dashboard/clinics.
func (h *Handler) PostClinic(w http.ResponseWriter, r *http.Request) {
	u, _ := dashUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(r.FormValue("slug"))
	if name == "" || slug == "" {
		h.renderToast(w, "", "Name and slug are required")
		return
	}
	const q = `INSERT INTO clinics (name, slug, address, city, phone, email)
	           VALUES ($1, $2, $3, $4, $5, $6)
	           RETURNING id::text, name, slug, address, city, phone, email, is_active, created_at`
	var c clinicItem
	err := h.pool.QueryRow(r.Context(), q,
		name, slug, optString(r.FormValue("address")), optString(r.FormValue("city")),
		optString(r.FormValue("phone")), optString(r.FormValue("email")),
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Address, &c.City, &c.Phone, &c.Email, &c.IsActive, &c.CreatedAt)
	if err != nil {
		h.logger.Error("create clinic", "error", err)
		h.renderToast(w, "", "Failed to create clinic (slug may already exist)")
		return
	}
	h.renderFrag(w, "clinic_row", c)
	h.renderToast(w, "Clinic created", "")
}

// ─── Users (super_admin) ──────────────────────────────────────────────────────

// GetUsers handles GET /dashboard/users.
func (h *Handler) GetUsers(w http.ResponseWriter, r *http.Request) {
	u, _ := dashUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	const q = `SELECT id::text, name, email, role, clinic_id::text, is_active, last_login_at
	           FROM admin_users ORDER BY created_at DESC`
	rows, err := h.pool.Query(r.Context(), q)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	var list []userItem
	for rows.Next() {
		var ui userItem
		if err := rows.Scan(&ui.ID, &ui.Name, &ui.Email, &ui.Role, &ui.ClinicID, &ui.IsActive, &ui.LastLoginAt); err != nil {
			continue
		}
		list = append(list, ui)
	}
	h.renderPage(w, r, "users", PageData{
		User: u,
		Data: userPageData{Users: list},
	})
}

// PostUser handles POST /dashboard/users.
func (h *Handler) PostUser(w http.ResponseWriter, r *http.Request) {
	u, _ := dashUserFromCtx(r.Context())
	if !u.IsSuperAdmin() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	role := r.FormValue("role")
	clinicIDStr := strings.TrimSpace(r.FormValue("clinic_id"))

	if name == "" || email == "" || password == "" || role == "" {
		h.renderToast(w, "", "Name, email, password and role are required")
		return
	}
	if role != "super_admin" && role != "clinic_admin" {
		h.renderToast(w, "", "Role must be super_admin or clinic_admin")
		return
	}
	if role == "clinic_admin" && clinicIDStr == "" {
		h.renderToast(w, "", "clinic_id is required for clinic_admin")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		h.renderToast(w, "", "Internal error")
		return
	}
	var clinicIDPtr *string
	if clinicIDStr != "" {
		clinicIDPtr = &clinicIDStr
	}
	const q = `INSERT INTO admin_users (clinic_id, name, email, password_hash, role)
	           VALUES ($1::uuid, $2, $3, $4, $5)
	           RETURNING id::text, name, email, role, clinic_id::text, is_active, last_login_at`
	var ui userItem
	err = h.pool.QueryRow(r.Context(), q, clinicIDPtr, name, email, string(hash), role).
		Scan(&ui.ID, &ui.Name, &ui.Email, &ui.Role, &ui.ClinicID, &ui.IsActive, &ui.LastLoginAt)
	if err != nil {
		h.logger.Error("create user", "error", err)
		h.renderToast(w, "", "Failed to create user (email may already exist)")
		return
	}
	h.renderFrag(w, "user_row", ui)
	h.renderToast(w, "User created", "")
}

// DeleteUser handles DELETE /dashboard/users/{id} (deactivates the user).
func (h *Handler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	caller, _ := dashUserFromCtx(r.Context())
	if !caller.IsSuperAdmin() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	userID := chi.URLParam(r, "id")
	const q = `UPDATE admin_users SET is_active=FALSE WHERE id=$1::uuid
	           RETURNING id::text, name, email, role, clinic_id::text, is_active, last_login_at`
	var ui userItem
	err := h.pool.QueryRow(r.Context(), q, userID).
		Scan(&ui.ID, &ui.Name, &ui.Email, &ui.Role, &ui.ClinicID, &ui.IsActive, &ui.LastLoginAt)
	if errors.Is(err, pgx.ErrNoRows) {
		h.renderToast(w, "", "User not found")
		return
	}
	if err != nil {
		h.logger.Error("deactivate user", "error", err)
		h.renderToast(w, "", "Failed to deactivate user")
		return
	}
	h.renderFrag(w, "user_row", ui)
	h.renderToast(w, "User deactivated", "")
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func optString(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func parseOptFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseCSV(s string) []string {
	if s = strings.TrimSpace(s); s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

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

func serviceContent(name, description string, priceMin, priceMax float64) string {
	return fmt.Sprintf("%s: %s. Price: %.2f–%.2f.", name, description, priceMin, priceMax)
}

func doctorContent(name, specialization, bio string, availableDays []string) string {
	return fmt.Sprintf("Dr. %s (%s). %s. Available: %s.", name, specialization, bio, strings.Join(availableDays, ", "))
}
