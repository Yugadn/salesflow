package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/pkg/auth"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

// ClaimState is what an idempotency store knows about a key.
type ClaimState int

const (
	ClaimNew     ClaimState = iota // first time: the caller now owns the key
	ClaimPending                   // another request with this key is still running
	ClaimDone                      // a request with this key already created an order
)

// Claim is the result of claiming an idempotency key.
type Claim struct {
	State       ClaimState
	Fingerprint string // hash of the request that first used the key
	OrderID     int64  // set when State is ClaimDone
}

// IdempotencyStore remembers which order each idempotency key produced.
type IdempotencyStore interface {
	// Claim atomically takes key if unused, otherwise reports its state.
	Claim(ctx context.Context, key, fingerprint string) (Claim, error)
	// Complete records the order the key produced.
	Complete(ctx context.Context, key, fingerprint string, orderID int64) error
	// Release frees a key whose request failed, so the client may retry it.
	Release(ctx context.Context, key string) error
}

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_\-:.]{8,128}$`)

// CreateIdempotent creates an order at most once per idempotency key.
//
// A salesperson on a weak connection may tap "save" twice, or their client may
// resend after a timeout. With the same key, the second request gets back the
// order the first one created instead of a duplicate (replayed = true).
// Reusing a key for a different order is a client bug and is rejected. If the
// store is unreachable the request fails rather than risk a duplicate order:
// in B2B a duplicate order means a duplicate shipment.
func (s *Service) CreateIdempotent(ctx context.Context, p auth.Principal, key string, in CreateInput) (o domain.Order, replayed bool, err error) {
	if key == "" {
		o, err = s.Create(ctx, p, in)
		return o, false, err
	}
	if s.idem == nil {
		return domain.Order{}, false, apperr.Unavailable("idempotency_unavailable", "idempotency keys are not supported right now")
	}
	if !keyPattern.MatchString(key) {
		return domain.Order{}, false, apperr.Invalid("invalid_idempotency_key", "Idempotency-Key must be 8-128 letters, digits or -_:.")
	}

	fp, err := fingerprint(in)
	if err != nil {
		return domain.Order{}, false, err
	}
	// Keys are scoped per user so two users can never collide.
	scoped := fmt.Sprintf("%d:%s", p.UserID, key)

	claim, err := s.idem.Claim(ctx, scoped, fp)
	if err != nil {
		return domain.Order{}, false, apperr.Unavailable("idempotency_unavailable", "could not check the Idempotency-Key; retry the request")
	}
	if claim.State != ClaimNew && claim.Fingerprint != fp {
		return domain.Order{}, false, apperr.Invalid("idempotency_key_reused", "this Idempotency-Key was already used for a different order")
	}
	switch claim.State {
	case ClaimPending:
		return domain.Order{}, false, apperr.Conflict("request_in_progress", "an identical request is still being processed; retry shortly")
	case ClaimDone:
		o, err = s.Get(ctx, p, claim.OrderID)
		return o, true, err
	}

	o, err = s.Create(ctx, p, in)
	if err != nil {
		_ = s.idem.Release(ctx, scoped)
		return domain.Order{}, false, err
	}
	// The order exists now; a failure to record that is not the client's
	// problem, so it is not returned. The pending claim expires on its own.
	_ = s.idem.Complete(ctx, scoped, fp, o.ID)
	return o, false, nil
}

func fingerprint(in CreateInput) (string, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("fingerprint request: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
