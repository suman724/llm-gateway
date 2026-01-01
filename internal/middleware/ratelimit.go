package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/user/llm-gateway/internal/store"
)

func RateLimitMiddleware(rlStore store.RateLimitStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantCtx, exists := c.Get("tenant")
		if !exists {
			slog.Error("Tenant context missing in RateLimitMiddleware", "path", c.Request.URL.Path)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Tenant context missing"})
			return
		}
		tenant := tenantCtx.(*store.Tenant)

		// Check RPM
		currentRPM, err := rlStore.IncrementRPM(c.Request.Context(), tenant.TenantID)
		if err != nil {
			slog.Error("Rate limit check failed", "error", err, "tenant_id", tenant.TenantID)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Rate limit check failed"})
			return
		}

		if currentRPM > int64(tenant.RPMLimit) {
			slog.Warn("Rate limit exceeded (RPM)", "tenant_id", tenant.TenantID, "limit", tenant.RPMLimit, "current", currentRPM)
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded (RPM)",
				"limit": tenant.RPMLimit,
			})
			return
		}

		// Check TPM (Tokens Per Minute)
		// We check the *current* usage against the limit.
		// Note: We are not adding the current request's tokens here because we haven't processed it yet.
		// This is a "Check then Act" (with Act happening asynchronously in Handler).
		// It's slightly loose but performant.
		currentTPM, err := rlStore.GetTPM(c.Request.Context(), tenant.TenantID)
		if err != nil {
			slog.Error("TPM check failed", "error", err)
			// checking TPM failure shouldn't block? failing closed for safety
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Rate limit check failed (TPM)"})
			return
		}

		if currentTPM > int64(tenant.TPMLimit) {
			slog.Warn("Rate limit exceeded (TPM)", "tenant_id", tenant.TenantID, "limit", tenant.TPMLimit, "current", currentTPM)
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "Rate limit exceeded (TPM)",
				"limit": tenant.TPMLimit,
			})
			return
		}

		c.Next()
	}
}
