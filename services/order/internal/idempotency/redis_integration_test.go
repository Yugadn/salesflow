//go:build integration

package idempotency_test

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/testinfra"
	"github.com/yugadn/salesflow/services/order/internal/idempotency"
	"github.com/yugadn/salesflow/services/order/internal/usecase"
)

func TestRedisStore(t *testing.T) {
	ctx := context.Background()
	opts, err := redis.ParseURL(testinfra.Redis(t))
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	store := idempotency.NewRedis(client)

	first, err := store.Claim(ctx, "7:abc", "fp1")
	require.NoError(t, err)
	assert.Equal(t, usecase.ClaimNew, first.State)

	second, err := store.Claim(ctx, "7:abc", "fp1")
	require.NoError(t, err)
	assert.Equal(t, usecase.Claim{State: usecase.ClaimPending, Fingerprint: "fp1"}, second)

	ttl, err := client.TTL(ctx, "idem:order:7:abc").Result()
	require.NoError(t, err)
	assert.Positive(t, ttl, "a pending claim must expire so a crash cannot block the key forever")

	require.NoError(t, store.Complete(ctx, "7:abc", "fp1", 42))
	done, err := store.Claim(ctx, "7:abc", "fp1")
	require.NoError(t, err)
	assert.Equal(t, usecase.Claim{State: usecase.ClaimDone, Fingerprint: "fp1", OrderID: 42}, done)

	require.NoError(t, store.Release(ctx, "7:abc"))
	again, err := store.Claim(ctx, "7:abc", "fp2")
	require.NoError(t, err)
	assert.Equal(t, usecase.ClaimNew, again.State, "a released key can be claimed again")
}

// Only one of many simultaneous claims wins: SET NX is atomic in Redis.
func TestRedisStore_ConcurrentClaimsHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	opts, err := redis.ParseURL(testinfra.Redis(t))
	require.NoError(t, err)
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	store := idempotency.NewRedis(client)

	results := make(chan usecase.ClaimState, 20)
	for range 20 {
		go func() {
			c, err := store.Claim(ctx, "7:race", "fp")
			if err != nil {
				results <- -1
				return
			}
			results <- c.State
		}()
	}
	winners := 0
	for range 20 {
		s := <-results
		require.NotEqual(t, usecase.ClaimState(-1), s)
		if s == usecase.ClaimNew {
			winners++
		}
	}
	assert.Equal(t, 1, winners)
}
