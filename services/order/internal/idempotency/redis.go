// Package idempotency stores idempotency keys in Redis.
package idempotency

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/yugadn/salesflow/services/order/internal/usecase"
)

const (
	// A pending claim outlives any request, then expires so a crashed request
	// does not block its key forever.
	pendingTTL = 30 * time.Second
	// How long a completed key is remembered; clients retry within minutes.
	doneTTL = 24 * time.Hour
)

// Redis keeps one string per key:
//
//	idem:order:<user>:<key> = "pending|<fingerprint>" or "done|<fingerprint>|<order id>"
type Redis struct {
	client *redis.Client
}

func NewRedis(client *redis.Client) *Redis { return &Redis{client: client} }

func redisKey(key string) string { return "idem:order:" + key }

func (r *Redis) Claim(ctx context.Context, key, fingerprint string) (usecase.Claim, error) {
	// SET NX can lose to a key that expires between it and the GET; one retry covers that.
	for range 2 {
		ok, err := r.client.SetNX(ctx, redisKey(key), "pending|"+fingerprint, pendingTTL).Result()
		if err != nil {
			return usecase.Claim{}, fmt.Errorf("claim idempotency key: %w", err)
		}
		if ok {
			return usecase.Claim{State: usecase.ClaimNew}, nil
		}
		val, err := r.client.Get(ctx, redisKey(key)).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return usecase.Claim{}, fmt.Errorf("read idempotency key: %w", err)
		}
		return parse(val)
	}
	return usecase.Claim{}, errors.New("idempotency key kept expiring while claiming it")
}

func parse(val string) (usecase.Claim, error) {
	parts := strings.Split(val, "|")
	switch {
	case len(parts) == 2 && parts[0] == "pending":
		return usecase.Claim{State: usecase.ClaimPending, Fingerprint: parts[1]}, nil
	case len(parts) == 3 && parts[0] == "done":
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			return usecase.Claim{}, fmt.Errorf("corrupt idempotency record %q", val)
		}
		return usecase.Claim{State: usecase.ClaimDone, Fingerprint: parts[1], OrderID: id}, nil
	default:
		return usecase.Claim{}, fmt.Errorf("corrupt idempotency record %q", val)
	}
}

func (r *Redis) Complete(ctx context.Context, key, fingerprint string, orderID int64) error {
	return r.client.Set(ctx, redisKey(key), fmt.Sprintf("done|%s|%d", fingerprint, orderID), doneTTL).Err()
}

func (r *Redis) Release(ctx context.Context, key string) error {
	return r.client.Del(ctx, redisKey(key)).Err()
}
