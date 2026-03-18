package dashboard

import (
	"context"
	"errors"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/naveenjoy/smart-clinic-bot/internal/admin"
)

type ctxKey string

const dashUserKey ctxKey = "dash_user"

func withDashUser(ctx context.Context, u *admin.AdminUser) context.Context {
	return context.WithValue(ctx, dashUserKey, u)
}

func dashUserFromCtx(ctx context.Context) (*admin.AdminUser, bool) {
	u, ok := ctx.Value(dashUserKey).(*admin.AdminUser)
	return u, ok
}

// verifyDashboardToken replicates admin.verifyToken for the dashboard package.
func verifyDashboardToken(secret, tokenStr string) (*admin.Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &admin.Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	c, ok := t.Claims.(*admin.Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return c, nil
}

// DashboardMiddleware reads admin_session cookie, verifies JWT, injects AdminUser.
func (h *Handler) DashboardMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("admin_session")
		if err != nil {
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		claims, err := verifyDashboardToken(h.jwtSecret, cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{
				Name:   "admin_session",
				MaxAge: -1,
				Path:   "/dashboard",
			})
			http.Redirect(w, r, "/dashboard/login", http.StatusSeeOther)
			return
		}
		u := &admin.AdminUser{
			ID:       claims.AdminID,
			ClinicID: claims.ClinicID,
			Email:    claims.Email,
			Role:     claims.Role,
		}
		next.ServeHTTP(w, r.WithContext(withDashUser(r.Context(), u)))
	})
}

// dashAuthorizeClinic mirrors admin.authorizeClinic for use inside dashboard handlers.
func dashAuthorizeClinic(ctx context.Context, clinicID string) error {
	u, ok := dashUserFromCtx(ctx)
	if !ok {
		return errors.New("unauthenticated")
	}
	if u.IsSuperAdmin() {
		return nil
	}
	if u.ClinicID != nil && *u.ClinicID == clinicID {
		return nil
	}
	return errors.New("forbidden")
}
