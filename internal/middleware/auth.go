package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-gateway/internal/store"
)

func AuthMiddleware(tenantStore store.TenantStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
			return
		}

		apiKey := parts[1]

		tenant, err := tenantStore.GetTenant(c.Request.Context(), apiKey)
		if err != nil {
			slog.Warn("Failed to validate tenant", "error", err, "ip", c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API Key"})
			return
		}

		if tenant == nil {
			slog.Warn("Tenant not found for key", "ip", c.ClientIP())
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid API Key"})
			return
		}

		// Store tenant in context for subsequent middleware/handlers
		c.Set("tenant", tenant)
		c.Next()
	}
}
