package admin

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const tokenTTL = 24 * time.Hour

// Claims is the JWT payload.
type Claims struct {
	AdminID  string  `json:"admin_id"`
	Email    string  `json:"email"`
	Role     string  `json:"role"`
	ClinicID *string `json:"clinic_id,omitempty"` // nil for super_admin
	jwt.RegisteredClaims
}

// issueToken creates a signed 24h JWT for the given admin user.
func issueToken(secret string, u *AdminUser) (string, error) {
	claims := Claims{
		AdminID:  u.ID,
		Email:    u.Email,
		Role:     u.Role,
		ClinicID: u.ClinicID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(tokenTTL)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// verifyToken parses and validates a signed JWT, returning the claims.
func verifyToken(secret, tokenStr string) (*Claims, error) {
	t, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil || !t.Valid {
		return nil, errors.New("invalid token")
	}
	c, ok := t.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims")
	}
	return c, nil
}

// hashPassword returns a bcrypt hash of the plaintext password.
func hashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// checkPassword compares plaintext against a bcrypt hash.
func checkPassword(plain, hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
