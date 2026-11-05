// Package domain holds user accounts and their rules.
package domain

import (
	"net/mail"
	"strings"
	"time"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
)

// bcrypt only reads the first 72 bytes, so longer passwords would silently
// collide; reject them instead.
const (
	MinPasswordLength = 8
	MaxPasswordLength = 72
)

type User struct {
	ID            int64     `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Role          auth.Role `json:"role"`
	SalespersonID int64     `json:"salesperson_id,omitempty"`
	Active        bool      `json:"active"`
	CreatedAt     time.Time `json:"created_at"`
}

// Principal is what the user's token says about them.
func (u User) Principal() auth.Principal {
	return auth.Principal{UserID: u.ID, Email: u.Email, Name: u.Name, Role: u.Role, SalespersonID: u.SalespersonID}
}

// UserInput is the editable part of an account. Password is required on
// create and optional on update (empty keeps the current one).
type UserInput struct {
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	Password      string    `json:"password"`
	Role          auth.Role `json:"role"`
	SalespersonID int64     `json:"salesperson_id"`
	Active        *bool     `json:"active"`
}

// NormalizeEmail lower-cases and trims, so logins are case-insensitive.
func NormalizeEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Validate checks in, normalising it in place.
func (in *UserInput) Validate(creating bool) error {
	in.Email, in.Name = NormalizeEmail(in.Email), strings.TrimSpace(in.Name)
	if creating {
		if _, err := mail.ParseAddress(in.Email); err != nil || !strings.Contains(in.Email, "@") {
			return apperr.Invalid("invalid_email", "%q is not a valid email address", in.Email)
		}
	}
	if in.Name == "" {
		return apperr.Invalid("invalid_user", "name is required")
	}
	if !in.Role.Valid() {
		return apperr.Invalid("invalid_role", "role must be admin, sales or warehouse")
	}
	// A sales login must say which salesperson it is; the others must not.
	if in.Role == auth.RoleSales && in.SalespersonID <= 0 {
		return apperr.Invalid("salesperson_required", "a sales user must be linked to a salesperson")
	}
	if in.Role != auth.RoleSales {
		in.SalespersonID = 0
	}
	if creating || in.Password != "" {
		if err := ValidatePassword(in.Password); err != nil {
			return err
		}
	}
	return nil
}

func ValidatePassword(pw string) error {
	if len(pw) < MinPasswordLength || len(pw) > MaxPasswordLength {
		return apperr.Invalid("invalid_password", "password must be %d to %d characters", MinPasswordLength, MaxPasswordLength)
	}
	return nil
}
