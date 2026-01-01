package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/ioutil"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"github.com/user/llm-gateway/internal/middleware"
	"github.com/user/llm-gateway/internal/store"
)

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Handler struct {
	rlStore    store.RateLimitStore
	modelStore store.ModelStore
	usageStore store.UsageStore
	httpClient *http.Client
	cb         *gobreaker.CircuitBreaker
	wg         sync.WaitGroup
}

func NewHandler(rlStore store.RateLimitStore, modelStore store.ModelStore, usageStore store.UsageStore, timeout time.Duration) *Handler {
	st := gobreaker.Settings{
		Name:        "LLM-Proxy-CB",
		MaxRequests: 5,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.6
		},
	}

	return &Handler{
		rlStore:    rlStore,
		modelStore: modelStore,
		usageStore: usageStore,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		cb: gobreaker.NewCircuitBreaker(st),
	}
}

// Shutdown waits for all async tasks to complete
func (h *Handler) Shutdown(ctx context.Context) error {
	// Create a channel to signal completion
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *Handler) CreateCompletion(c *gin.Context) {
	start := time.Now()
	tenantCtx, exists := c.Get("tenant")
	if !exists {
		slog.Error("Tenant context missing", "path", c.Request.URL.Path)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tenant context missing"})
		return
	}
	tenant := tenantCtx.(*store.Tenant)

	// 1. Read and buffer body to inspect model
	// Hard Limit: 10MB to prevent OOM
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10*1024*1024)
	bodyBytes, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		if err.Error() == "http: request body too large" {
			slog.Warn("Request body too large", "tenant_id", tenant.TenantID)
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large (limit: 10MB)"})
			return
		}
		slog.Error("Failed to read body", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}
	// Restore body for upstream
	c.Request.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))

	var chatReq ChatRequest
	if err := json.Unmarshal(bodyBytes, &chatReq); err != nil {
		slog.Warn("Invalid JSON body", "error", err, "tenant_id", tenant.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON body"})
		return
	}

	// Validation: Max Messages
	if len(chatReq.Messages) > 50 {
		slog.Warn("Too many messages", "count", len(chatReq.Messages), "tenant_id", tenant.TenantID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Too many messages in conversation (max: 50)"})
		return
	}

	logger := slog.With("tenant_id", tenant.TenantID, "model", chatReq.Model)

	// 2. Validate Model access
	allowed := false
	for _, m := range tenant.AllowedModels {
		if m == "*" || m == chatReq.Model {
			allowed = true
			break
		}
	}
	if !allowed {
		logger.Warn("Model not allowed for this tenant")
		c.JSON(http.StatusForbidden, gin.H{"error": "Model not allowed for this tenant"})
		return
	}

	// 3. Lookup Model Config
	modelConfig, err := h.modelStore.GetModel(c.Request.Context(), chatReq.Model)
	if err != nil {
		logger.Error("Failed to resolve model config", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resolve model config"})
		return
	}
	if modelConfig == nil {
		logger.Warn("Model configuration not found")
		c.JSON(http.StatusNotFound, gin.H{"error": "Model configuration not found"})
		return
	}

	// 4. Determine Upstream Candidates
	baseURLs := modelConfig.BaseURLs
	if len(baseURLs) == 0 {
		logger.Error("No base URLs configured for model")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Misconfigured model: no base URLs"})
		return
	}
	apiKey := os.Getenv(modelConfig.APIKeyEnv)
	if apiKey == "" {
		logger.Warn("API Key env var not set for model", "env_var", modelConfig.APIKeyEnv)
	}

	// Retry Policy Config (Headers > Defaults)
	retryMax := 3
	backoffMs := 100
	retryFactor := 2.0

	// Helper to parse header int
	if hVal := c.GetHeader("X-LLM-Retry-Max"); hVal != "" {
		if val, err := strconv.Atoi(hVal); err == nil && val >= 0 && val <= 10 {
			retryMax = val
		}
	}
	if hVal := c.GetHeader("X-LLM-Retry-Backoff-Ms"); hVal != "" {
		if val, err := strconv.Atoi(hVal); err == nil && val >= 0 {
			backoffMs = val
		}
	}

	// 5. Execute Request with Retry & Failover
	// Using shared client for connection pooling
	var resp *http.Response
	var lastErr error

	attempt := 0
	urlIndex := 0

	for attempt <= retryMax {
		// Round-robin selection of URL based on attempt count (Failover strategy)
		currentURL := baseURLs[urlIndex%len(baseURLs)]

		logger.Info("Attempting upstream", "attempt", attempt, "url", currentURL, "stream", chatReq.Stream)

		// Use c.Request.Context() to propagate client cancellation
		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, currentURL, bytes.NewBuffer(bodyBytes))
		if err != nil {
			logger.Error("Failed to create upstream request", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upstream request"})
			return
		}
		proxyReq.Header = c.Request.Header.Clone()
		proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
		proxyReq.Header.Del("Host")
		// Remove retry headers from upstream request
		proxyReq.Header.Del("X-LLM-Retry-Max")
		proxyReq.Header.Del("X-LLM-Retry-Backoff-Ms")

		// Execute with Circuit Breaker
		respInterface, cbErr := h.cb.Execute(func() (interface{}, error) {
			return h.httpClient.Do(proxyReq)
		})

		if cbErr != nil {
			lastErr = cbErr
			if cbErr == gobreaker.ErrOpenState {
				logger.Warn("Circuit breaker open")
				break
			}
		} else {
			resp = respInterface.(*http.Response)
			lastErr = nil // Clear error if success
		}

		// success condition
		if lastErr == nil && resp.StatusCode < 500 && resp.StatusCode != 429 {
			break
		}

		// Failure handling
		attempt++

		// Failover on Network Error, 5xx, or 429
		shouldFailover := lastErr != nil || resp.StatusCode >= 500 || resp.StatusCode == 429
		if shouldFailover {
			urlIndex++ // Switch to next provider/url
		}

		if attempt <= retryMax {
			// Skip backoff for 429 Failover (Fail fast to backup)
			if resp != nil && resp.StatusCode == 429 && shouldFailover {
				logger.Info("Rate limited (429), failing over immediately", "url", currentURL)
				continue
			}

			sleepTime := time.Duration(backoffMs) * time.Millisecond * time.Duration(math.Pow(retryFactor, float64(attempt-1)))
			time.Sleep(sleepTime)
		}
	}

	if lastErr != nil {
		logger.Error("Upstream provider failed after retries", "error", lastErr)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream provider failed", "details": lastErr.Error()})
		return
	}
	if resp != nil && resp.StatusCode >= 500 {
		logger.Error("Upstream provider returned 5xx after retries", "status", resp.StatusCode)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream provider error", "status": resp.StatusCode})
		return
	}
	defer resp.Body.Close()

	// Log Latency
	latency := time.Since(start)
	logger.Info("Proxy request completed", "status", resp.StatusCode, "latency_ms", latency.Milliseconds())

	// 7. Forward Response Headers
	for k, vv := range resp.Header {
		for _, v := range vv {
			c.Header(k, v)
		}
	}
	c.Status(resp.StatusCode)

	// Calculate Input Tokens (Approx)
	inputLen := len(bodyBytes)
	inputTokens := inputLen / 4

	// 8. Handle Response Body (Streaming vs Non-Streaming)
	var outputTokens int

	if chatReq.Stream {
		// Streaming Response
		outputTokens = h.streamResponse(c, resp.Body, tenant.TenantID, chatReq.Model, start)
	} else {
		// Non-Streaming Response
		body, _ := ioutil.ReadAll(resp.Body)
		c.Writer.Write(body)
		outputTokens = len(body) / 4
	}

	// 9. Update Metrics & Logs (Async)
	// We do this AFTER response is done (streaming blocks until done)
	// 9. Update Metrics & Logs (Async)
	// We do this AFTER response is done (streaming blocks until done)
	h.wg.Add(1)
	go func(tid, mid string, in, out int) {
		defer h.wg.Done()

		// Update Rate Limit
		estTokens := in + out
		_, err := h.rlStore.IncrementTPM(context.Background(), tid, estTokens)
		if err != nil {
			slog.Error("Failed to increment TPM", "error", err)
		}

		// Log Usage Persistence
		requestID := uuid.New().String()
		usageRec := &store.UsageRecord{
			TenantID:     tid,
			Timestamp:    start.Format(time.RFC3339Nano),
			RequestID:    requestID,
			ModelID:      mid,
			InputTokens:  in,
			OutputTokens: out,
		}

		// Retry Logic (Simple backing off)
		// Try 3 times
		for i := 0; i < 3; i++ {
			if err := h.usageStore.LogUsage(context.Background(), usageRec); err != nil {
				slog.Error("Failed to log usage, retrying", "attempt", i+1, "error", err)
				time.Sleep(time.Duration(100*(i+1)) * time.Millisecond)
				continue
			}
			break
		}
	}(tenant.TenantID, chatReq.Model, inputTokens, outputTokens)

	// Prometheus Metrics
	middleware.RecordTokenUsage(tenant.TenantID, chatReq.Model, inputTokens, outputTokens)

	// Set model in context for metrics
	c.Set("model", chatReq.Model)
}

// streamResponse forwards SSE events to client and counts tokens
func (h *Handler) streamResponse(c *gin.Context, body io.Reader, tenantID, model string, start time.Time) int {
	scanner := bufio.NewScanner(body)
	outputTokens := 0
	firstByte := true

	// Create a flushing writer
	c.Writer.Flush()

	for scanner.Scan() {
		line := scanner.Text()

		// Record TTFT on first line
		if firstByte {
			ttft := time.Since(start).Seconds()
			middleware.RecordTTFT(tenantID, model, ttft)
			firstByte = false
		}

		// Write line to client immediately
		c.Writer.WriteString(line + "\n")
		c.Writer.Flush()

		// Token Counting Logic
		// Check for "data: " prefix
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				continue
			}

			// Parse partial JSON to get content
			// We only need choices[0].delta.content
			// Optimization: Quick string search or lightweight JSON parser
			// For robustness, lets use JSON (though slightly slower, it's safer)
			var partial struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &partial); err == nil {
				if len(partial.Choices) > 0 {
					content := partial.Choices[0].Delta.Content
					// Count tokens: rough approx len/4
					outputTokens += len(content) / 4
				}
			}
		}
	}
	return outputTokens
}
