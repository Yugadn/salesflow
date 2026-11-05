package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/pkg/httpx"
)

var (
	sales = auth.Principal{UserID: 7, Email: "andi@example.com", Name: "Andi", Role: auth.RoleSales, SalespersonID: 3}
	admin = auth.Principal{UserID: 1, Email: "admin@example.com", Name: "Admin", Role: auth.RoleAdmin}
)

type issuer struct {
	key     *rsa.PrivateKey
	kid     string
	server  *httptest.Server
	fetches atomic.Int32
}

// newIssuer stands in for auth-service: a key pair and a JWKS endpoint.
func newIssuer(t *testing.T) *issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	is := &issuer{key: key, kid: "key-1"}
	is.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		is.fetches.Add(1)
		_ = json.NewEncoder(w).Encode(auth.JWKS{Keys: []auth.JWK{auth.PublicJWK(is.kid, &is.key.PublicKey)}})
	}))
	t.Cleanup(is.server.Close)
	return is
}

func (is *issuer) token(t *testing.T, p auth.Principal, ttl time.Duration) string {
	t.Helper()
	tok, err := auth.Sign(is.key, is.kid, p, time.Now(), ttl)
	require.NoError(t, err)
	return tok
}

func TestVerify_ValidToken(t *testing.T) {
	is := newIssuer(t)
	v := auth.NewVerifier(is.server.URL)

	got, err := v.Verify(context.Background(), is.token(t, sales, time.Hour))

	require.NoError(t, err)
	assert.Equal(t, sales, got)
}

func TestVerify_Rejects(t *testing.T) {
	is := newIssuer(t)
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	custom := func(mutate func(c *auth.Claims), key *rsa.PrivateKey, method jwt.SigningMethod) string {
		c := auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Issuer: auth.Issuer, Audience: jwt.ClaimStrings{auth.Audience}, Subject: "7",
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
			Role: auth.RoleSales, SalespersonID: 3,
		}
		mutate(&c)
		tok := jwt.NewWithClaims(method, c)
		tok.Header["kid"] = is.kid
		var signed string
		var err error
		if method == jwt.SigningMethodHS256 {
			signed, err = tok.SignedString([]byte("guessable"))
		} else {
			signed, err = tok.SignedString(key)
		}
		require.NoError(t, err)
		return signed
	}
	noop := func(*auth.Claims) {}

	tests := map[string]string{
		"expired":               is.token(t, sales, -time.Hour),
		"signed by another key": custom(noop, other, jwt.SigningMethodRS256),
		"HS256 algorithm swap":  custom(noop, nil, jwt.SigningMethodHS256),
		"wrong issuer":          custom(func(c *auth.Claims) { c.Issuer = "evil" }, is.key, jwt.SigningMethodRS256),
		"wrong audience":        custom(func(c *auth.Claims) { c.Audience = jwt.ClaimStrings{"other-app"} }, is.key, jwt.SigningMethodRS256),
		"no expiry":             custom(func(c *auth.Claims) { c.ExpiresAt = nil }, is.key, jwt.SigningMethodRS256),
		"unknown role":          custom(func(c *auth.Claims) { c.Role = "superuser" }, is.key, jwt.SigningMethodRS256),
		"sales w/o salesperson": custom(func(c *auth.Claims) { c.SalespersonID = 0 }, is.key, jwt.SigningMethodRS256),
		"non-numeric subject":   custom(func(c *auth.Claims) { c.Subject = "abc" }, is.key, jwt.SigningMethodRS256),
		"garbage":               "not.a.jwt",
	}
	v := auth.NewVerifier(is.server.URL)
	for name, tok := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := v.Verify(context.Background(), tok)
			assert.ErrorIs(t, err, auth.ErrInvalidToken)
		})
	}
}

func TestVerify_FetchesKeysLazilyAndCachesThem(t *testing.T) {
	is := newIssuer(t)
	v := auth.NewVerifier(is.server.URL)
	assert.Zero(t, is.fetches.Load(), "nothing fetched before the first token")

	for range 5 {
		_, err := v.Verify(context.Background(), is.token(t, sales, time.Hour))
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), is.fetches.Load(), "keys are cached")
}

func TestVerify_UnknownKidDoesNotHammerTheIssuer(t *testing.T) {
	is := newIssuer(t)
	v := auth.NewVerifier(is.server.URL)
	_, err := v.Verify(context.Background(), is.token(t, sales, time.Hour))
	require.NoError(t, err)

	forged, err := auth.Sign(is.key, "made-up-kid", sales, time.Now(), time.Hour)
	require.NoError(t, err)
	for range 20 {
		_, err := v.Verify(context.Background(), forged)
		require.Error(t, err)
	}
	assert.Equal(t, int32(1), is.fetches.Load(), "refetches are rate-limited")
}

func TestMiddleware(t *testing.T) {
	is := newIssuer(t)
	e := echo.New()
	e.HTTPErrorHandler = httpx.NewServer(slog.New(slog.DiscardHandler)).HTTPErrorHandler
	e.Use(auth.Middleware(auth.NewVerifier(is.server.URL)))
	ok := func(c echo.Context) error {
		p, _ := auth.FromContext(c.Request().Context())
		return c.String(http.StatusOK, string(p.Role))
	}
	e.GET("/healthz", func(c echo.Context) error { return c.String(http.StatusOK, "up") })
	e.POST("/internal/quote", func(c echo.Context) error { return c.String(http.StatusOK, "internal") })
	e.GET("/anyone", ok, auth.AnyRole)
	e.POST("/admin-only", ok, auth.AdminOnly)

	call := func(method, path, token string) (int, string) {
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}

	tests := []struct {
		name, method, path, token string
		want                      int
	}{
		{"health needs no token", "GET", "/healthz", "", 200},
		{"internal routes need no token", "POST", "/internal/quote", "", 200},
		{"no token", "GET", "/anyone", "", 401},
		{"bad token", "GET", "/anyone", "nope", 401},
		{"valid token", "GET", "/anyone", is.token(t, sales, time.Hour), 200},
		{"wrong role", "POST", "/admin-only", is.token(t, sales, time.Hour), 403},
		{"right role", "POST", "/admin-only", is.token(t, admin, time.Hour), 200},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, body := call(tt.method, tt.path, tt.token)
			assert.Equal(t, tt.want, got, body)
		})
	}
}
