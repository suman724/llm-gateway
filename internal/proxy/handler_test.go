package proxy

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/user/llm-gateway/internal/store"
)

func TestCreateCompletion_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mocks
	mockRL := store.NewMockRateLimitStore()
	mockUsage := &store.MockUsageStore{}
	mockModel := &store.MockModelStore{
		Models: map[string]*store.Model{
			"gpt-4": {ModelID: "gpt-4", BaseURLs: []string{"http://mock-llm.com"}},
		},
	}

	// Handler
	h := NewHandler(mockRL, mockModel, mockUsage, 1*time.Second)

	tests := []struct {
		name           string
		requestBody    string
		tenant         *store.Tenant
		expectedStatus int
	}{
		{
			name:        "Valid Request",
			requestBody: `{"model": "gpt-4", "messages": [{"role": "user", "content": "hi"}]}`,
			tenant: &store.Tenant{
				TenantID:      "t1",
				AllowedModels: []string{"gpt-4"},
			},
			expectedStatus: http.StatusBadGateway, // Because http://mock-llm.com is unreachable, but validation passed
		},
		{
			name:        "Model Not Allowed",
			requestBody: `{"model": "gpt-4", "messages": []}`,
			tenant: &store.Tenant{
				TenantID:      "t1",
				AllowedModels: []string{"claude-2"}, // gpt-4 not allowed
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:        "Too Many Messages",
			requestBody: `{"model": "gpt-4", "messages": ` + makeLargeMessageList(60) + `}`,
			tenant: &store.Tenant{
				TenantID:      "t1",
				AllowedModels: []string{"gpt-4"},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("POST", "/chat/completions", bytes.NewBufferString(tt.requestBody))
			c.Set("tenant", tt.tenant)

			h.CreateCompletion(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestCreateCompletion_Streaming(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock Upstream LLM Server (SSE)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			panic("expected http.ResponseWriter to be an http.Flusher")
		}

		// Simulating OpenAI Stream
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" World"}}]}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond)
		}
	}))
	defer upstream.Close()

	// Mocks
	mockRL := store.NewMockRateLimitStore()
	mockUsage := &store.MockUsageStore{}
	mockModel := &store.MockModelStore{
		Models: map[string]*store.Model{
			"gpt-4-stream": {ModelID: "gpt-4-stream", BaseURLs: []string{upstream.URL}, APIKeyEnv: "OPENAI_API_KEY"},
		},
	}

	h := NewHandler(mockRL, mockModel, mockUsage, 1*time.Second)

	// User Request
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	reqBody := `{"model": "gpt-4-stream", "messages": [{"role": "user", "content": "hi"}], "stream": true}`
	c.Request, _ = http.NewRequest("POST", "/v1/chat/completions", bytes.NewBufferString(reqBody))

	tenant := &store.Tenant{
		TenantID:      "t-stream",
		AllowedModels: []string{"*"},
	}
	c.Set("tenant", tenant)

	// Execute
	h.CreateCompletion(c)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "data: ")
	assert.Contains(t, w.Body.String(), "Hello")

	// Wait for async usage log
	time.Sleep(100 * time.Millisecond) // Give goroutine time to finish (or use WaitGroup if exposed)

	// Check Usage Log
	assert.Len(t, mockUsage.Records, 1)
	if len(mockUsage.Records) > 0 {
		rec := mockUsage.Records[0]
		assert.Equal(t, "t-stream", rec.TenantID)
		assert.True(t, rec.OutputTokens > 0, "Should count output tokens")
	}
}

func TestHandler_Shutdown(t *testing.T) {
	mockRL := store.NewMockRateLimitStore()
	mockUsage := &store.MockUsageStore{}
	mockModel := &store.MockModelStore{}
	h := NewHandler(mockRL, mockModel, mockUsage, 1*time.Second)

	// Simulate an async task running
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		time.Sleep(50 * time.Millisecond)
	}()

	start := time.Now()
	// Shutdown should wait
	err := h.Shutdown(context.Background())
	elapsed := time.Since(start)

	assert.NoError(t, err)
	assert.True(t, elapsed >= 50*time.Millisecond, "Shutdown should wait for async task")
}

func makeLargeMessageList(n int) string {
	// Helper to generate JSON array of n messages
	// simple approximation
	s := "["
	for i := 0; i < n; i++ {
		s += `{"role": "user", "content": "msg"},`
	}
	s = s[:len(s)-1] + "]"
	return s
}
