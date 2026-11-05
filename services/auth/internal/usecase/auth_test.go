package usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/services/auth/internal/domain"
)

// ---- fakes

type fakeRepo struct {
	users  map[int64]StoredUser
	nextID int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{users: map[int64]StoredUser{}} }

func (f *fakeRepo) CreateUser(_ context.Context, in domain.UserInput, hash string) (domain.User, error) {
	for _, u := range f.users {
		if u.Email == in.Email {
			return domain.User{}, apperr.Conflict("email_taken", "taken")
		}
	}
	f.nextID++
	u := StoredUser{User: domain.User{ID: f.nextID, Email: in.Email, Name: in.Name, Role: in.Role, SalespersonID: in.SalespersonID, Active: true}, PasswordHash: hash}
	f.users[u.ID] = u
	return u.User, nil
}

func (f *fakeRepo) GetUser(_ context.Context, id int64) (StoredUser, error) {
	u, ok := f.users[id]
	if !ok {
		return StoredUser{}, apperr.NotFound("user_not_found", "not found")
	}
	return u, nil
}

func (f *fakeRepo) GetUserByEmail(_ context.Context, email string) (StoredUser, error) {
	for _, u := range f.users {
		if u.Email == email {
			return u, nil
		}
	}
	return StoredUser{}, apperr.NotFound("user_not_found", "not found")
}

func (f *fakeRepo) ListUsers(context.Context) ([]domain.User, error) { return nil, nil }

func (f *fakeRepo) UpdateUser(_ context.Context, id int64, in domain.UserInput, active bool) (domain.User, error) {
	u := f.users[id]
	u.Name, u.Role, u.SalespersonID, u.Active = in.Name, in.Role, in.SalespersonID, active
	f.users[id] = u
	return u.User, nil
}

func (f *fakeRepo) UpdatePassword(_ context.Context, id int64, hash string) error {
	u := f.users[id]
	u.PasswordHash = hash
	f.users[id] = u
	return nil
}

func (f *fakeRepo) CountUsers(context.Context) (int64, error) { return int64(len(f.users)), nil }

type testKeys struct {
	key *rsa.PrivateKey
}

func (k testKeys) Current() (string, *rsa.PrivateKey) { return "test", k.key }
func (k testKeys) Public() auth.JWKS {
	return auth.JWKS{Keys: []auth.JWK{auth.PublicJWK("test", &k.key.PublicKey)}}
}

func newService(t *testing.T) (*Service, *fakeRepo, *auth.Verifier) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keys := testKeys{key: key}
	repo := newFakeRepo()
	verifier := auth.NewVerifierWithKeys(func(_ context.Context, kid string) (*rsa.PublicKey, error) {
		if kid != "test" {
			return nil, fmt.Errorf("unknown kid %q", kid)
		}
		return &key.PublicKey, nil
	})
	return NewService(repo, keys, time.Hour), repo, verifier
}

func mustCreate(t *testing.T, svc *Service, in domain.UserInput) domain.User {
	t.Helper()
	u, err := svc.CreateUser(context.Background(), in)
	require.NoError(t, err)
	return u
}

// ---- tests

func TestLogin_IssuesTokenTheGatewayCanVerify(t *testing.T) {
	svc, _, verifier := newService(t)
	ctx := context.Background()
	u := mustCreate(t, svc, domain.UserInput{Email: "Andi@Example.com ", Name: "Andi", Password: "secret123", Role: auth.RoleSales, SalespersonID: 4})

	s, err := svc.Login(ctx, "andi@example.COM", "secret123")

	require.NoError(t, err)
	assert.Equal(t, "andi@example.com", s.User.Email, "emails are case-insensitive")
	p, err := verifier.Verify(ctx, s.Token)
	require.NoError(t, err)
	assert.Equal(t, auth.Principal{UserID: u.ID, Email: "andi@example.com", Name: "Andi", Role: auth.RoleSales, SalespersonID: 4}, p)
	assert.WithinDuration(t, time.Now().Add(time.Hour), s.ExpiresAt, 5*time.Second)
}

func TestLogin_FailuresLookIdentical(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()
	mustCreate(t, svc, domain.UserInput{Email: "rina@example.com", Name: "Rina", Password: "secret123", Role: auth.RoleWarehouse})

	disabled := mustCreate(t, svc, domain.UserInput{Email: "old@example.com", Name: "Old", Password: "secret123", Role: auth.RoleWarehouse})
	d := repo.users[disabled.ID]
	d.Active = false
	repo.users[disabled.ID] = d

	cases := map[string][2]string{
		"unknown email":    {"nobody@example.com", "secret123"},
		"wrong password":   {"rina@example.com", "wrong-password"},
		"inactive account": {"old@example.com", "secret123"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Login(ctx, c[0], c[1])
			ae, ok := apperr.As(err)
			require.True(t, ok)
			assert.Equal(t, apperr.KindUnauthorized, ae.Kind)
			assert.Equal(t, "invalid_credentials", ae.Code)
			assert.Equal(t, "email or password is incorrect", ae.Message, "must not reveal which part was wrong")
		})
	}
}

func TestCreateUser_Validation(t *testing.T) {
	svc, _, _ := newService(t)
	tests := []struct {
		name     string
		in       domain.UserInput
		wantCode string
	}{
		{"sales needs a salesperson", domain.UserInput{Email: "a@x.com", Name: "A", Password: "secret123", Role: auth.RoleSales}, "salesperson_required"},
		{"short password", domain.UserInput{Email: "a@x.com", Name: "A", Password: "short", Role: auth.RoleAdmin}, "invalid_password"},
		{"password longer than bcrypt reads", domain.UserInput{Email: "a@x.com", Name: "A", Password: string(make([]byte, 73)), Role: auth.RoleAdmin}, "invalid_password"},
		{"bad email", domain.UserInput{Email: "not-an-email", Name: "A", Password: "secret123", Role: auth.RoleAdmin}, "invalid_email"},
		{"unknown role", domain.UserInput{Email: "a@x.com", Name: "A", Password: "secret123", Role: "root"}, "invalid_role"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateUser(context.Background(), tt.in)
			ae, ok := apperr.As(err)
			require.True(t, ok, "%v", err)
			assert.Equal(t, tt.wantCode, ae.Code)
		})
	}
}

func TestCreateUser_DropsSalespersonForOtherRoles(t *testing.T) {
	svc, _, _ := newService(t)
	u := mustCreate(t, svc, domain.UserInput{Email: "w@x.com", Name: "W", Password: "secret123", Role: auth.RoleWarehouse, SalespersonID: 9})
	assert.Zero(t, u.SalespersonID)
}

func TestUpdateUser(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	admin := mustCreate(t, svc, domain.UserInput{Email: "admin@x.com", Name: "Admin", Password: "secret123", Role: auth.RoleAdmin})
	caller := admin.Principal()
	worker := mustCreate(t, svc, domain.UserInput{Email: "w@x.com", Name: "W", Password: "secret123", Role: auth.RoleWarehouse})

	t.Run("password reset takes effect", func(t *testing.T) {
		_, err := svc.UpdateUser(ctx, caller, worker.ID, domain.UserInput{Name: "W", Role: auth.RoleWarehouse, Password: "new-secret1"})
		require.NoError(t, err)
		_, err = svc.Login(ctx, "w@x.com", "secret123")
		require.Error(t, err, "old password no longer works")
		_, err = svc.Login(ctx, "w@x.com", "new-secret1")
		require.NoError(t, err)
	})

	t.Run("deactivating blocks sign-in", func(t *testing.T) {
		off := false
		_, err := svc.UpdateUser(ctx, caller, worker.ID, domain.UserInput{Name: "W", Role: auth.RoleWarehouse, Active: &off})
		require.NoError(t, err)
		_, err = svc.Login(ctx, "w@x.com", "new-secret1")
		require.Error(t, err)
	})

	t.Run("admin cannot demote or deactivate themselves", func(t *testing.T) {
		_, err := svc.UpdateUser(ctx, caller, admin.ID, domain.UserInput{Name: "Admin", Role: auth.RoleWarehouse})
		ae, ok := apperr.As(err)
		require.True(t, ok)
		assert.Equal(t, "cannot_demote_self", ae.Code)

		off := false
		_, err = svc.UpdateUser(ctx, caller, admin.ID, domain.UserInput{Name: "Admin", Role: auth.RoleAdmin, Active: &off})
		require.Error(t, err)
	})
}

func TestEnsureAdmin_OnlyOnEmptyDatabase(t *testing.T) {
	svc, repo, _ := newService(t)
	ctx := context.Background()

	created, err := svc.EnsureAdmin(ctx, "admin@salesflow.local", "admin12345")
	require.NoError(t, err)
	assert.True(t, created)

	created, err = svc.EnsureAdmin(ctx, "other@salesflow.local", "admin12345")
	require.NoError(t, err)
	assert.False(t, created)
	assert.Len(t, repo.users, 1)
}
