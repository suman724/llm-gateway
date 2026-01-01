package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/llm-gateway/internal/store"
)

func TestRateLimitMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		setupStore     func() *store.MockRateLimitStore
		tenant         *store.Tenant
		expectedStatus int
	}{
		{
			name: "Allowed Request",
			setupStore: func() *store.MockRateLimitStore {
				return store.NewMockRateLimitStore()
			},
			tenant:         &store.Tenant{TenantID: "t1", RPMLimit: 10, TPMLimit: 100},
			expectedStatus: http.StatusOK,
		},
		{
			name: "RPM Limit Exceeded",
			setupStore: func() *store.MockRateLimitStore {
				m := store.NewMockRateLimitStore()
				m.RPM["t1"] = 11 // > 10
				return m
			},
			tenant:         &store.Tenant{TenantID: "t1", RPMLimit: 10, TPMLimit: 100},
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name: "TPM Limit Exceeded",
			setupStore: func() *store.MockRateLimitStore {
				m := store.NewMockRateLimitStore()
				m.TPM["t1"] = 101 // > 100
				return m
			},
			tenant:         &store.Tenant{TenantID: "t1", RPMLimit: 10, TPMLimit: 100},
			expectedStatus: http.StatusTooManyRequests,
		},
		{
			name: "Store Error",
			setupStore: func() *store.MockRateLimitStore {
				m := store.NewMockRateLimitStore()
				m.Err = errors.New("redis down")
				return m
			},
			tenant:         &store.Tenant{TenantID: "t1", RPMLimit: 10, TPMLimit: 100},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/", nil)

			// Inject Tenant into Context (simulating Auth Middleware)
			c.Set("tenant", tt.tenant)

			rlStore := tt.setupStore()
			RateLimitMiddleware(rlStore)(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
