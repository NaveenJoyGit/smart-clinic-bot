package dashboard

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
	"github.com/naveenjoy/smart-clinic-bot/web"
)

// NewRouter builds the /dashboard sub-router, including static file serving.
// Returns an error if template parsing fails.
func NewRouter(pool *pgxpool.Pool, indexer *rag.Indexer, jwtSecret string, logger *slog.Logger) (http.Handler, error) {
	h, err := NewHandler(pool, indexer, jwtSecret, logger)
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()

	// Static assets (no auth).
	staticFS, _ := fs.Sub(web.FS, "static")
	fileServer(r, "/static", http.FS(staticFS))

	// Public routes (no auth)
	r.Get("/login", h.GetLogin)
	r.Post("/login", h.PostLogin)
	r.Post("/logout", h.PostLogout)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(h.DashboardMiddleware)

		r.Get("/", h.GetOverview)

		// Super-admin only
		r.Get("/clinics", h.GetClinics)
		r.Post("/clinics", h.PostClinic)
		r.Get("/users", h.GetUsers)
		r.Post("/users", h.PostUser)
		r.Delete("/users/{id}", h.DeleteUser)

		// Clinic-scoped resources
		r.Get("/clinics/{clinic_id}/faqs", h.GetFAQs)
		r.Post("/clinics/{clinic_id}/faqs", h.PostFAQ)
		r.Get("/clinics/{clinic_id}/faqs/{id}", h.GetFAQRow)
		r.Get("/clinics/{clinic_id}/faqs/{id}/edit", h.GetFAQEdit)
		r.Put("/clinics/{clinic_id}/faqs/{id}", h.PutFAQ)
		r.Delete("/clinics/{clinic_id}/faqs/{id}", h.DeleteFAQ)

		r.Get("/clinics/{clinic_id}/services", h.GetServices)
		r.Post("/clinics/{clinic_id}/services", h.PostService)
		r.Get("/clinics/{clinic_id}/services/{id}", h.GetServiceRow)
		r.Get("/clinics/{clinic_id}/services/{id}/edit", h.GetServiceEdit)
		r.Put("/clinics/{clinic_id}/services/{id}", h.PutService)
		r.Delete("/clinics/{clinic_id}/services/{id}", h.DeleteService)

		r.Get("/clinics/{clinic_id}/doctors", h.GetDoctors)
		r.Post("/clinics/{clinic_id}/doctors", h.PostDoctor)
		r.Get("/clinics/{clinic_id}/doctors/{id}", h.GetDoctorRow)
		r.Get("/clinics/{clinic_id}/doctors/{id}/edit", h.GetDoctorEdit)
		r.Put("/clinics/{clinic_id}/doctors/{id}", h.PutDoctor)
		r.Delete("/clinics/{clinic_id}/doctors/{id}", h.DeleteDoctor)

		r.Get("/clinics/{clinic_id}/appointments", h.GetAppointments)
		r.Put("/clinics/{clinic_id}/appointments/{id}/status", h.PutAppointmentStatus)

		r.Get("/clinics/{clinic_id}/conversations", h.GetConversations)
	})

	return r, nil
}

// fileServer sets up a http.FileServer handler to serve static files from root.
// Adapted from the official go-chi/chi FileServer example.
func fileServer(r chi.Router, path string, root http.FileSystem) {
	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
