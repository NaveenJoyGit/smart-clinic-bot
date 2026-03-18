package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

// AdminUser holds the authenticated admin's identity, extracted from JWT claims.
type AdminUser struct {
	ID       string
	ClinicID *string // nil = super_admin
	Name     string
	Email    string
	Role     string
}

func (u *AdminUser) IsSuperAdmin() bool { return u.Role == "super_admin" }

type contextKey string

const adminUserKey contextKey = "admin_user"

func withAdminUser(ctx context.Context, u *AdminUser) context.Context {
	return context.WithValue(ctx, adminUserKey, u)
}

func adminUserFromCtx(ctx context.Context) (*AdminUser, bool) {
	u, ok := ctx.Value(adminUserKey).(*AdminUser)
	return u, ok
}

// JWTMiddleware verifies the Bearer token and injects AdminUser into the request context.
func JWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "missing or invalid Authorization header")
				return
			}
			claims, err := verifyToken(secret, strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			u := &AdminUser{
				ID:       claims.AdminID,
				ClinicID: claims.ClinicID,
				Email:    claims.Email,
				Role:     claims.Role,
			}
			next.ServeHTTP(w, r.WithContext(withAdminUser(r.Context(), u)))
		})
	}
}

// authorizeClinic returns nil if the caller may act on the given clinicID.
func authorizeClinic(ctx context.Context, clinicID string) error {
	u, ok := adminUserFromCtx(ctx)
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
