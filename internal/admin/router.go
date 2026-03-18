package admin

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/rag"
)

// NewRouter builds the /admin sub-router.
func NewRouter(pool *pgxpool.Pool, indexer *rag.Indexer, jwtSecret string, logger *slog.Logger) http.Handler {
	h := NewHandler(pool, indexer, jwtSecret, logger)
	r := chi.NewRouter()

	// Public — login does not require a token.
	r.Post("/auth/login", h.Login)

	// All routes below require a valid JWT.
	r.Group(func(r chi.Router) {
		r.Use(JWTMiddleware(jwtSecret))

		r.Get("/clinics", h.ListClinics)
		r.Post("/clinics", h.CreateClinic)
		r.Get("/clinics/{clinic_id}", h.GetClinic)
		r.Put("/clinics/{clinic_id}", h.UpdateClinic)

		r.Get("/clinics/{clinic_id}/faqs", h.ListFAQs)
		r.Post("/clinics/{clinic_id}/faqs", h.CreateFAQ)
		r.Put("/clinics/{clinic_id}/faqs/{id}", h.UpdateFAQ)
		r.Delete("/clinics/{clinic_id}/faqs/{id}", h.DeleteFAQ)

		r.Get("/clinics/{clinic_id}/services", h.ListServices)
		r.Post("/clinics/{clinic_id}/services", h.CreateService)
		r.Put("/clinics/{clinic_id}/services/{id}", h.UpdateService)
		r.Delete("/clinics/{clinic_id}/services/{id}", h.DeleteService)

		r.Get("/clinics/{clinic_id}/doctors", h.ListDoctors)
		r.Post("/clinics/{clinic_id}/doctors", h.CreateDoctor)
		r.Put("/clinics/{clinic_id}/doctors/{id}", h.UpdateDoctor)
		r.Delete("/clinics/{clinic_id}/doctors/{id}", h.DeleteDoctor)

		r.Post("/users", h.CreateAdminUser)
		r.Get("/users", h.ListAdminUsers)
		r.Delete("/users/{id}", h.DeactivateAdminUser)
		r.Post("/users/{id}/reset-password", h.ResetAdminPassword)
	})

	return r
}
