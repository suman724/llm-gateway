package admin

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-gateway/internal/store"
)

type AdminHandler struct {
	tenantStore store.TenantStore
	apiKey      string // Admin API Key for protection
}

func NewAdminHandler(ts store.TenantStore, apiKey string) *AdminHandler {
	return &AdminHandler{
		tenantStore: ts,
		apiKey:      apiKey,
	}
}

// Protected Middleware
func (h *AdminHandler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("X-Admin-Key")
		if key != h.apiKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Admin Key"})
			return
		}
		c.Next()
	}
}

type CreateTenantRequest struct {
	TenantID      string   `json:"tenant_id" binding:"required"`
	Name          string   `json:"name" binding:"required"`
	APIKey        string   `json:"api_key" binding:"required"`
	RPMLimit      int      `json:"rpm_limit"`
	TPMLimit      int      `json:"tpm_limit"`
	AllowedModels []string `json:"allowed_models"`
}

func (h *AdminHandler) CreateTenant(c *gin.Context) {
	var req CreateTenantRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate Defaults
	if req.RPMLimit == 0 {
		req.RPMLimit = 100
	}
	if req.TPMLimit == 0 {
		req.TPMLimit = 100000
	}
	if len(req.AllowedModels) == 0 {
		req.AllowedModels = []string{"*"}
	}

	tenant := &store.Tenant{
		TenantID:      req.TenantID,
		Name:          req.Name,
		APIKey:        req.APIKey,
		RPMLimit:      req.RPMLimit,
		TPMLimit:      req.TPMLimit,
		AllowedModels: req.AllowedModels,
		IsActive:      true,
	}

	if err := h.tenantStore.CreateTenant(context.Background(), tenant); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create tenant"})
		return
	}

	c.JSON(http.StatusCreated, tenant)
}
