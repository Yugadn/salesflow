// Package usecase implements sign-in, token issuing and user management.
package usecase

import (
	"context"
	"crypto/rsa"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/services/auth/internal/domain"
)

// StoredUser is a user with their password hash, which never leaves this package.
type StoredUser struct {
	domain.User
	PasswordHash string
}

// Repository is the persistence port. Missing rows are apperr NotFound;
// duplicate emails and salesperson links are apperr Conflict.
type Repository interface {
	CreateUser(ctx context.Context, in domain.UserInput, passwordHash string) (domain.User, error)
	GetUser(ctx context.Context, id int64) (StoredUser, error)
	GetUserByEmail(ctx context.Context, email string) (StoredUser, error)
	ListUsers(ctx context.Context) ([]domain.User, error)
	UpdateUser(ctx context.Context, id int64, in domain.UserInput, active bool) (domain.User, error)
	UpdatePassword(ctx context.Context, id int64, passwordHash string) error
	CountUsers(ctx context.Context) (int64, error)
}

// Keys gives the current signing key and every published public key.
type Keys interface {
	Current() (kid string, key *rsa.PrivateKey)
	Public() auth.JWKS
}

type Service struct {
	repo     Repository
	keys     Keys
	tokenTTL time.Duration
	now      func() time.Time

	// dummyHash is compared against when the email is unknown, so a failed
	// sign-in takes the same time whether or not the account exists.
	dummyHash []byte
}

func NewService(repo Repository, keys Keys, tokenTTL time.Duration) *Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("timing-equaliser"), bcrypt.DefaultCost)
	return &Service{repo: repo, keys: keys, tokenTTL: tokenTTL, now: time.Now, dummyHash: dummy}
}

type Session struct {
	Token     string      `json:"token"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      domain.User `json:"user"`
}

// errBadCredentials is deliberately vague: it must not reveal whether the
// email exists or the account is inactive.
var errBadCredentials = apperr.Unauthorized("invalid_credentials", "email or password is incorrect")

// Login checks credentials and issues a signed token.
func (s *Service) Login(ctx context.Context, email, password string) (Session, error) {
	u, err := s.repo.GetUserByEmail(ctx, domain.NormalizeEmail(email))
	if apperr.IsNotFound(err) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return Session{}, errBadCredentials
	}
	if err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil || !u.Active {
		return Session{}, errBadCredentials
	}

	now := s.now()
	kid, key := s.keys.Current()
	token, err := auth.Sign(key, kid, u.Principal(), now, s.tokenTTL)
	if err != nil {
		return Session{}, err
	}
	return Session{Token: token, ExpiresAt: now.Add(s.tokenTTL).UTC(), User: u.User}, nil
}

// JWKS is the public key set verifiers fetch.
func (s *Service) JWKS() auth.JWKS { return s.keys.Public() }

// Me returns the caller's current account.
func (s *Service) Me(ctx context.Context, p auth.Principal) (domain.User, error) {
	u, err := s.repo.GetUser(ctx, p.UserID)
	if err != nil {
		return domain.User{}, err
	}
	return u.User, nil
}

func (s *Service) ListUsers(ctx context.Context) ([]domain.User, error) {
	return s.repo.ListUsers(ctx)
}

func (s *Service) CreateUser(ctx context.Context, in domain.UserInput) (domain.User, error) {
	if err := in.Validate(true); err != nil {
		return domain.User{}, err
	}
	hash, err := hashPassword(in.Password)
	if err != nil {
		return domain.User{}, err
	}
	return s.repo.CreateUser(ctx, in, hash)
}

// UpdateUser edits name, role, salesperson link and active flag, and resets
// the password when one is given. Email is the login and does not change.
// An admin cannot demote or deactivate themselves, so the system can never
// be left without an administrator by accident.
func (s *Service) UpdateUser(ctx context.Context, caller auth.Principal, id int64, in domain.UserInput) (domain.User, error) {
	current, err := s.repo.GetUser(ctx, id)
	if err != nil {
		return domain.User{}, err
	}
	in.Email = current.Email
	if err := in.Validate(false); err != nil {
		return domain.User{}, err
	}
	active := current.Active
	if in.Active != nil {
		active = *in.Active
	}
	if id == caller.UserID && (in.Role != auth.RoleAdmin || !active) {
		return domain.User{}, apperr.Invalid("cannot_demote_self", "you cannot remove your own admin access")
	}

	u, err := s.repo.UpdateUser(ctx, id, in, active)
	if err != nil {
		return domain.User{}, err
	}
	if in.Password != "" {
		hash, err := hashPassword(in.Password)
		if err != nil {
			return domain.User{}, err
		}
		if err := s.repo.UpdatePassword(ctx, id, hash); err != nil {
			return domain.User{}, err
		}
	}
	return u, nil
}

// EnsureAdmin creates the first administrator when there are no users yet,
// so a fresh installation can be signed into. It does nothing afterwards.
func (s *Service) EnsureAdmin(ctx context.Context, email, password string) (bool, error) {
	n, err := s.repo.CountUsers(ctx)
	if err != nil || n > 0 {
		return false, err
	}
	_, err = s.CreateUser(ctx, domain.UserInput{Email: email, Name: "Administrator", Password: password, Role: auth.RoleAdmin})
	return err == nil, err
}

func hashPassword(pw string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
