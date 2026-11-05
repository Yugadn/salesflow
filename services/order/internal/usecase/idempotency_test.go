package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
)

// fakeIdem mirrors the Redis store's semantics in memory.
type fakeIdem struct {
	claims map[string]Claim
	down   bool
}

func newFakeIdem() *fakeIdem { return &fakeIdem{claims: map[string]Claim{}} }

func (f *fakeIdem) Claim(_ context.Context, key, fp string) (Claim, error) {
	if f.down {
		return Claim{}, errors.New("connection refused")
	}
	if c, ok := f.claims[key]; ok {
		return c, nil
	}
	f.claims[key] = Claim{State: ClaimPending, Fingerprint: fp}
	return Claim{State: ClaimNew}, nil
}

func (f *fakeIdem) Complete(_ context.Context, key, fp string, id int64) error {
	f.claims[key] = Claim{State: ClaimDone, Fingerprint: fp, OrderID: id}
	return nil
}

func (f *fakeIdem) Release(_ context.Context, key string) error {
	delete(f.claims, key)
	return nil
}

const key = "order-form-7f3c2a"

func TestCreateIdempotent_DoubleSubmitCreatesOneOrder(t *testing.T) {
	svc, store, _ := newService()
	svc.WithIdempotency(newFakeIdem())
	ctx := context.Background()

	first, replayed, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.NoError(t, err)
	assert.False(t, replayed)

	second, replayed, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Equal(t, first.ID, second.ID)
	assert.Len(t, store.orders, 1)
}

func TestCreateIdempotent_KeyReusedForDifferentOrder(t *testing.T) {
	svc, store, _ := newService()
	svc.WithIdempotency(newFakeIdem())
	ctx := context.Background()
	_, _, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.NoError(t, err)

	other := draftInput()
	other.Lines[0].Qty = 5
	_, _, err = svc.CreateIdempotent(ctx, andi, key, other)

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "idempotency_key_reused", ae.Code)
	assert.Len(t, store.orders, 1)
}

func TestCreateIdempotent_ConcurrentDuplicateWaits(t *testing.T) {
	svc, _, _ := newService()
	idem := newFakeIdem()
	svc.WithIdempotency(idem)
	fp, err := fingerprint(draftInput())
	require.NoError(t, err)
	idem.claims["2:"+key] = Claim{State: ClaimPending, Fingerprint: fp} // first request still running

	_, _, err = svc.CreateIdempotent(context.Background(), andi, key, draftInput())

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "request_in_progress", ae.Code)
}

func TestCreateIdempotent_FailureFreesTheKey(t *testing.T) {
	svc, store, master := newService()
	idem := newFakeIdem()
	svc.WithIdempotency(idem)
	ctx := context.Background()

	master.err = apperr.Invalid("price_not_set", "no price")
	_, _, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.Error(t, err)
	assert.Empty(t, idem.claims, "a failed attempt must not block a corrected retry")

	master.err = nil
	_, replayed, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.NoError(t, err)
	assert.False(t, replayed)
	assert.Len(t, store.orders, 1)
}

func TestCreateIdempotent_KeysAreScopedPerUser(t *testing.T) {
	svc, store, _ := newService()
	svc.WithIdempotency(newFakeIdem())
	ctx := context.Background()

	_, _, err := svc.CreateIdempotent(ctx, andi, key, draftInput())
	require.NoError(t, err)
	_, replayed, err := svc.CreateIdempotent(ctx, admin, key, draftInput())
	require.NoError(t, err)

	assert.False(t, replayed, "another user's identical key is a different request")
	assert.Len(t, store.orders, 2)
}

func TestCreateIdempotent_StoreDownFailsClosed(t *testing.T) {
	svc, store, _ := newService()
	svc.WithIdempotency(&fakeIdem{claims: map[string]Claim{}, down: true})

	_, _, err := svc.CreateIdempotent(context.Background(), andi, key, draftInput())

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, apperr.KindUnavailable, ae.Kind)
	assert.Empty(t, store.orders, "no order without the duplicate guard")
}

func TestCreateIdempotent_NoKeyCreatesNormally(t *testing.T) {
	svc, store, _ := newService()
	svc.WithIdempotency(&fakeIdem{down: true}) // never touched without a key

	_, _, err := svc.CreateIdempotent(context.Background(), andi, "", draftInput())

	require.NoError(t, err)
	assert.Len(t, store.orders, 1)
}

func TestCreateIdempotent_RejectsMalformedKeys(t *testing.T) {
	svc, _, _ := newService()
	svc.WithIdempotency(newFakeIdem())

	for _, bad := range []string{"short", "has spaces in it", "x" + string(make([]byte, 200))} {
		_, _, err := svc.CreateIdempotent(context.Background(), andi, bad, draftInput())
		ae, ok := apperr.As(err)
		require.True(t, ok, "key %q", bad)
		assert.Equal(t, "invalid_idempotency_key", ae.Code)
	}
}
