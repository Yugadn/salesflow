// Package repository persists users and signing keys in PostgreSQL.
package repository

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/services/auth/internal/domain"
	"github.com/yugadn/salesflow/services/auth/internal/repository/sqlcdb"
	"github.com/yugadn/salesflow/services/auth/internal/usecase"
)

type Postgres struct {
	q *sqlcdb.Queries
}

func NewPostgres(pool *pgxpool.Pool) *Postgres { return &Postgres{q: sqlcdb.New(pool)} }

func mapErr(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return apperr.NotFound("user_not_found", "user not found")
	case db.IsUniqueViolation(err) && strings.Contains(err.Error(), "users_salesperson_idx"):
		return apperr.Conflict("salesperson_has_account", "this salesperson already has a login")
	case db.IsUniqueViolation(err):
		return apperr.Conflict("email_taken", "a user with this email already exists")
	default:
		return fmt.Errorf("users: %w", err)
	}
}

func toUser(u sqlcdb.User) usecase.StoredUser {
	var sp int64
	if u.SalespersonID != nil {
		sp = *u.SalespersonID
	}
	return usecase.StoredUser{
		User: domain.User{
			ID: u.ID, Email: u.Email, Name: u.Name, Role: auth.Role(u.Role),
			SalespersonID: sp, Active: u.Active, CreatedAt: u.CreatedAt,
		},
		PasswordHash: u.PasswordHash,
	}
}

func salesperson(id int64) *int64 {
	if id <= 0 {
		return nil
	}
	return &id
}

func (p *Postgres) CreateUser(ctx context.Context, in domain.UserInput, hash string) (domain.User, error) {
	row, err := p.q.CreateUser(ctx, sqlcdb.CreateUserParams{
		Email: in.Email, Name: in.Name, PasswordHash: hash, Role: string(in.Role), SalespersonID: salesperson(in.SalespersonID),
	})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toUser(row).User, nil
}

func (p *Postgres) GetUser(ctx context.Context, id int64) (usecase.StoredUser, error) {
	row, err := p.q.GetUser(ctx, id)
	if err != nil {
		return usecase.StoredUser{}, mapErr(err)
	}
	return toUser(row), nil
}

func (p *Postgres) GetUserByEmail(ctx context.Context, email string) (usecase.StoredUser, error) {
	row, err := p.q.GetUserByEmail(ctx, email)
	if err != nil {
		return usecase.StoredUser{}, mapErr(err)
	}
	return toUser(row), nil
}

func (p *Postgres) ListUsers(ctx context.Context) ([]domain.User, error) {
	rows, err := p.q.ListUsers(ctx)
	if err != nil {
		return nil, mapErr(err)
	}
	out := make([]domain.User, 0, len(rows))
	for _, r := range rows {
		out = append(out, toUser(r).User)
	}
	return out, nil
}

func (p *Postgres) UpdateUser(ctx context.Context, id int64, in domain.UserInput, active bool) (domain.User, error) {
	row, err := p.q.UpdateUser(ctx, sqlcdb.UpdateUserParams{
		ID: id, Name: in.Name, Role: string(in.Role), SalespersonID: salesperson(in.SalespersonID), Active: active,
	})
	if err != nil {
		return domain.User{}, mapErr(err)
	}
	return toUser(row).User, nil
}

func (p *Postgres) UpdatePassword(ctx context.Context, id int64, hash string) error {
	if err := p.q.UpdatePassword(ctx, sqlcdb.UpdatePasswordParams{ID: id, PasswordHash: hash}); err != nil {
		return mapErr(err)
	}
	return nil
}

func (p *Postgres) CountUsers(ctx context.Context) (int64, error) {
	return p.q.CountUsers(ctx)
}

// KeyRing holds the signing keys in memory after loading them once.
type KeyRing struct {
	kid    string
	key    *rsa.PrivateKey
	public auth.JWKS
}

func (k *KeyRing) Current() (string, *rsa.PrivateKey) { return k.kid, k.key }
func (k *KeyRing) Public() auth.JWKS                  { return k.public }

// LoadKeys loads the signing keys, generating and storing the first one on a
// fresh database. The newest key signs; all keys are published.
func (p *Postgres) LoadKeys(ctx context.Context) (*KeyRing, error) {
	rows, err := p.q.ListSigningKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("load signing keys: %w", err)
	}
	if len(rows) == 0 {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate signing key: %w", err)
		}
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			return nil, err
		}
		kid := keyID(&key.PublicKey)
		pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
		// Two replicas starting at once may both generate; the loser reloads the winner's key.
		if err := p.q.InsertSigningKey(ctx, sqlcdb.InsertSigningKeyParams{Kid: kid, PrivateKeyPem: pemText}); err != nil && !db.IsUniqueViolation(err) {
			return nil, fmt.Errorf("store signing key: %w", err)
		}
		if rows, err = p.q.ListSigningKeys(ctx); err != nil {
			return nil, fmt.Errorf("reload signing keys: %w", err)
		}
	}

	ring := &KeyRing{}
	for i, r := range rows {
		block, _ := pem.Decode([]byte(r.PrivateKeyPem))
		if block == nil {
			return nil, fmt.Errorf("signing key %s: bad PEM", r.Kid)
		}
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("signing key %s: %w", r.Kid, err)
		}
		key, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("signing key %s: not RSA", r.Kid)
		}
		if i == 0 {
			ring.kid, ring.key = r.Kid, key
		}
		ring.public.Keys = append(ring.public.Keys, auth.PublicJWK(r.Kid, &key.PublicKey))
	}
	return ring, nil
}

// keyID derives a stable kid from the public key.
func keyID(pub *rsa.PublicKey) string {
	sum := sha256.Sum256(pub.N.Bytes())
	return hex.EncodeToString(sum[:8])
}
