// Package auth is the identity contract shared by every service: the JWT
// claims auth-service issues, how they are signed and verified, and the
// Principal handlers read to make authorization decisions.
//
// Tokens are RS256-signed. auth-service holds the private key and publishes
// the public half as a JWKS; the gateway and every service verify tokens
// against it, so no service ever needs a shared secret.
package auth

import (
	"context"
	"crypto/rsa"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Role decides what a user may do.
type Role string

const (
	RoleAdmin     Role = "admin"     // everything
	RoleSales     Role = "sales"     // orders on their own behalf; reads master data
	RoleWarehouse Role = "warehouse" // stock and deliveries
)

// Valid reports whether r is a known role.
func (r Role) Valid() bool {
	return r == RoleAdmin || r == RoleSales || r == RoleWarehouse
}

const (
	// Issuer and Audience are checked on every token.
	Issuer   = "salesflow-auth"
	Audience = "salesflow"
)

// Claims is the JWT payload. SalespersonID links a sales user to the
// master-data salesperson they act as.
type Claims struct {
	jwt.RegisteredClaims
	Email         string `json:"email"`
	Name          string `json:"name"`
	Role          Role   `json:"role"`
	SalespersonID int64  `json:"salesperson_id,omitempty"`
}

// Principal is the authenticated caller of a request.
type Principal struct {
	UserID        int64
	Email         string
	Name          string
	Role          Role
	SalespersonID int64
}

// Is reports whether the caller has one of roles.
func (p Principal) Is(roles ...Role) bool {
	for _, r := range roles {
		if p.Role == r {
			return true
		}
	}
	return false
}

// Sign issues a token for p, valid for ttl, signed with key and tagged with kid
// so verifiers know which public key to use.
func Sign(key *rsa.PrivateKey, kid string, p Principal, now time.Time, ttl time.Duration) (string, error) {
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Audience:  jwt.ClaimStrings{Audience},
			Subject:   fmt.Sprint(p.UserID),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Email:         p.Email,
		Name:          p.Name,
		Role:          p.Role,
		SalespersonID: p.SalespersonID,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

type ctxKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// FromContext returns the caller stored by the auth middleware.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok
}
