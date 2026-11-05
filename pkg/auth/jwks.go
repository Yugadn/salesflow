package auth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWK is one RSA public key in JSON Web Key form (RFC 7517).
type JWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// JWKS is the document auth-service publishes at /.well-known/jwks.json.
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// PublicJWK encodes pub as a signing JWK.
func PublicJWK(kid string, pub *rsa.PublicKey) JWK {
	return JWK{
		Kty: "RSA",
		Kid: kid,
		Use: "sig",
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// PublicKey decodes k back into an RSA public key.
func (k JWK) PublicKey() (*rsa.PublicKey, error) {
	if k.Kty != "RSA" {
		return nil, fmt.Errorf("jwk %s: unsupported key type %q", k.Kid, k.Kty)
	}
	n, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("jwk %s: modulus: %w", k.Kid, err)
	}
	e, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("jwk %s: exponent: %w", k.Kid, err)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}, nil
}

// KeyFunc returns the public key for a token's kid.
type KeyFunc func(ctx context.Context, kid string) (*rsa.PublicKey, error)

// Verifier checks tokens and turns them into a Principal.
type Verifier struct {
	keyFn  KeyFunc
	parser *jwt.Parser
}

// NewVerifier verifies tokens against the keys published at a JWKS URL.
func NewVerifier(jwksURL string) *Verifier {
	return NewVerifierWithKeys(newJWKSSource(jwksURL).key)
}

// NewVerifierWithKeys verifies tokens against keys from keyFn; auth-service
// uses it to check its own tokens without calling itself over HTTP.
func NewVerifierWithKeys(keyFn KeyFunc) *Verifier {
	return &Verifier{
		keyFn: keyFn,
		parser: jwt.NewParser(
			jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
			jwt.WithIssuer(Issuer),
			jwt.WithAudience(Audience),
			jwt.WithExpirationRequired(),
			jwt.WithLeeway(30*time.Second),
		),
	}
}

// ErrInvalidToken wraps every reason a token is rejected.
var ErrInvalidToken = errors.New("invalid token")

// Verify parses and validates token, returning its caller.
func (v *Verifier) Verify(ctx context.Context, token string) (Principal, error) {
	var claims Claims
	_, err := v.parser.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		return v.keyFn(ctx, kid)
	})
	if err != nil {
		return Principal{}, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	userID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || userID <= 0 {
		return Principal{}, fmt.Errorf("%w: bad subject %q", ErrInvalidToken, claims.Subject)
	}
	if !claims.Role.Valid() {
		return Principal{}, fmt.Errorf("%w: unknown role %q", ErrInvalidToken, claims.Role)
	}
	if claims.Role == RoleSales && claims.SalespersonID <= 0 {
		return Principal{}, fmt.Errorf("%w: sales token without salesperson", ErrInvalidToken)
	}
	return Principal{
		UserID:        userID,
		Email:         claims.Email,
		Name:          claims.Name,
		Role:          claims.Role,
		SalespersonID: claims.SalespersonID,
	}, nil
}

// jwksSource fetches keys from a JWKS URL.
//
// Keys are fetched lazily, on the first token, so a service can start before
// auth-service is up. A token signed with an unknown kid (after a key
// rotation) triggers a refetch, at most once per minRefresh, so a flood of
// forged tokens cannot turn into a flood of JWKS requests.
type jwksSource struct {
	url        string
	client     *http.Client
	minRefresh time.Duration

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	lastFetched time.Time
}

func newJWKSSource(url string) *jwksSource {
	return &jwksSource{
		url:        url,
		client:     &http.Client{Timeout: 5 * time.Second},
		minRefresh: 10 * time.Second,
		keys:       map[string]*rsa.PublicKey{},
	}
}

func (v *jwksSource) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if time.Since(v.lastFetched) < v.minRefresh {
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	if err := v.fetch(ctx); err != nil {
		return nil, err
	}
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown signing key %q", kid)
}

// fetch replaces the key set; the caller holds v.mu.
func (v *jwksSource) fetch(ctx context.Context) error {
	v.lastFetched = time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch jwks: status %d", resp.StatusCode)
	}
	var set JWKS
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return fmt.Errorf("decode jwks: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		pub, err := k.PublicKey()
		if err != nil {
			return err
		}
		keys[k.Kid] = pub
	}
	v.keys = keys
	return nil
}
