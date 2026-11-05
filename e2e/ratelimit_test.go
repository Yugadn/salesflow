//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRateLimit_SignInIsThrottled runs last (files run in name order): it
// exhausts this client's sign-in allowance on purpose.
//
// Each attempt also sends forged client-IP headers. nginx overwrites them
// with the real peer address before the gateway sees them, so rotating them
// must not buy the attacker extra attempts.
func TestRateLimit_SignInIsThrottled(t *testing.T) {
	var limited bool
	for i := 0; i < 15 && !limited; i++ {
		forged := map[string]string{"X-Real-IP": fmt.Sprintf("10.9.9.%d", i), "X-Forwarded-For": fmt.Sprintf("10.8.8.%d", i)}
		status, _, _ := request(t, "", http.MethodPost, authURL+"/login",
			map[string]string{"email": "nobody@test.local", "password": "wrong-password"}, nil, forged)
		limited = status == http.StatusTooManyRequests
	}
	assert.True(t, limited, "repeated failed sign-ins must be throttled, even with forged client IPs")
}
