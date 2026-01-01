package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/llm-gateway/internal/store"
)

func TestCreateTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockStore := store.NewMockTenantStore()
	h := NewAdminHandler(mockStore, "secret-admin-key")

	tests := []struct {
		name       string
		apiKey     string
		body       string
		wantStatus int
	}{
		{
			name:       "Unauthorized",
			apiKey:     "wrong-key",
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "Invalid JSON",
			apiKey:     "secret-admin-key",
			body:       `{invalid}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "Success",
			apiKey:     "secret-admin-key",
			body:       `{"tenant_id": "new-tenant", "name": "New Tenant", "api_key": "new-key"}`,
			wantStatus: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/admin/tenants", bytes.NewBufferString(tt.body))
			c.Request.Header.Set("X-Admin-Key", tt.apiKey)

			// Execute Middleware manually since we are testing Handler logic + Middleware
			middleware := h.AuthMiddleware()
			middleware(c)
			if !c.IsAborted() {
				h.CreateTenant(c)
			}

			assert.Equal(t, tt.wantStatus, w.Code)
		})
	}

	// Verify Persistence
	tenant, _ := mockStore.GetTenant(nil, "new-key")
	if assert.NotNil(t, tenant) {
		assert.Equal(t, "new-tenant", tenant.TenantID)
		assert.Equal(t, 100, tenant.RPMLimit) // Default
	}
}
