package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/llm-gateway/internal/store"
)

func TestAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Setup Mock Store
	mockStore := store.NewMockTenantStore()
	mockStore.Tenants["valid-key"] = &store.Tenant{
		APIKey:   "valid-key",
		TenantID: "tenant-1",
		IsActive: true,
	}
	mockStore.Tenants["inactive-key"] = &store.Tenant{
		APIKey:   "inactive-key",
		TenantID: "tenant-2",
		IsActive: false,
	}

	tests := []struct {
		name           string
		authHeader     string
		expectedStatus int
		checkContext   bool
	}{
		{
			name:           "Valid Token",
			authHeader:     "Bearer valid-key",
			expectedStatus: http.StatusOK,
			checkContext:   true,
		},
		{
			name:           "Invalid Token",
			authHeader:     "Bearer invalid-key",
			expectedStatus: http.StatusUnauthorized,
			checkContext:   false,
		},
		{
			name:           "Inactive Tenant",
			authHeader:     "Bearer inactive-key",
			expectedStatus: http.StatusUnauthorized, // Middleware calls GetTenant which might return nil or handle IsActive check
			checkContext:   false,
		},
		{
			name:           "Missing Header",
			authHeader:     "",
			expectedStatus: http.StatusUnauthorized,
			checkContext:   false,
		},
		{
			name:           "Malformed Header",
			authHeader:     "Basic foo",
			expectedStatus: http.StatusUnauthorized,
			checkContext:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/", nil)
			if tt.authHeader != "" {
				c.Request.Header.Set("Authorization", tt.authHeader)
			}

			// Define a handler to verify context
			dummyHandler := func(c *gin.Context) {
				if tt.checkContext {
					val, exists := c.Get("tenant")
					assert.True(t, exists, "Tenant should exist in context")
					tenant := val.(*store.Tenant)
					assert.Equal(t, "tenant-1", tenant.TenantID)
				}
				c.Status(http.StatusOK)
			}

			// Invoke Middleware
			AuthMiddleware(mockStore)(c)

			// Invoke Handler if not aborted
			if !c.IsAborted() {
				dummyHandler(c)
			}

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}
