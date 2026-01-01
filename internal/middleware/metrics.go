package middleware

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/user/llm-gateway/internal/store"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "status", "tenant_id", "model"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"tenant_id", "model"},
	)

	llmTokenUsage = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "llm_token_usage_total",
			Help: "Total number of LLM tokens processed",
		},
		[]string{"tenant_id", "model", "type"},
	)

	llmTTFT = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "llm_ttft_seconds",
			Help:    "Time To First Token latency in seconds",
			Buckets: []float64{0.1, 0.2, 0.5, 1.0, 2.0, 5.0},
		},
		[]string{"tenant_id", "model"},
	)
)

func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		// Extract context info if available
		tenantID := "unknown"
		if val, exists := c.Get("tenant"); exists {
			if t, ok := val.(*store.Tenant); ok {
				tenantID = t.TenantID
			}
		}

		// Try to extract model if set by Handler
		model := "unknown"
		if val, exists := c.Get("model"); exists {
			model = val.(string)
		}

		httpRequestsTotal.WithLabelValues(method, status, tenantID, model).Inc()
		httpRequestDuration.WithLabelValues(tenantID, model).Observe(duration)
	}
}

// RecordTokenUsage allows other packages to record token metrics
func RecordTokenUsage(tenantID, model string, inputTokens, outputTokens int) {
	llmTokenUsage.WithLabelValues(tenantID, model, "input").Add(float64(inputTokens))
	llmTokenUsage.WithLabelValues(tenantID, model, "output").Add(float64(outputTokens))
}

// RecordTTFT records the Time To First Token
func RecordTTFT(tenantID, model string, durationSeconds float64) {
	llmTTFT.WithLabelValues(tenantID, model).Observe(durationSeconds)
}
