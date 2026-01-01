# LLM Gateway - High Performance AI Proxy

A production-ready, multi-tenant LLM Gateway written in **Golang**. This gateway provides a unified request interface (OpenAI compatible) for multiple upstream providers, enforcing strict token-based rate limits (RPM/TPM), authentication, and billing observability.

Designed for specialized high-scale environments on **AWS ECS Fargate**.

## 🚀 Key Features

*   **Unified Interface**: 100% compatible with OpenAI Chat Completions API (`POST /v1/chat/completions`). Drop-in replacement for existing SDKs.
*   **Multi-Tenancy**: Granular access control via API Keys backed by DynamoDB. Isolate users by Tenant ID.
*   **Token-Based Rate Limiting**: Enforce limits on both **Requests Per Minute (RPM)** and **Tokens Per Minute (TPM)** using Redis (Lua scripts).
*   **Dynamic Model Routing**: Route requests to different upstream providers (OpenAI, Anthropic, Self-Hosted) dynamically based on configuration in DynamoDB.
*   **Streaming Support**: Full Server-Sent Events (SSE) support with real-time token counting and **Time To First Token (TTFT)** metrics.
*   **Observability**:
    *   **Prometheus Metrics**: Detailed metrics on latency, status codes, and token usage (`input_tokens`, `output_tokens`).
    *   **Distributed Tracing**: OpenTelemetry integration for full request lifecycle visibility.
    *   **Structured Logging**: JSON logs (`slog`) with Request ID correlation.

## 🛡️ Non-Functional Requirements (Implemented)

This gateway is harderened for production usage:

*   **Reliability**:
    *   **Graceful Shutdown**: Uses `sync.WaitGroup` to ensure all async tasks (usage logs) complete before server exit (zero data loss).
    *   **Resiliency**: Circuit Breaker (`gobreaker`) protects against cascading failures.
    *   **Retries**: Exponential backoff retries with failover to backup providers on 429s or 5xx errors.
*   **Scalability**:
    *   **Stateless Architecture**: Designed for horizontal scaling behind an ALB (AWS ECS Autoscaling implemented).
    *   **Async Logging**: Token usage is logged asynchronously to DynamoDB to decouple latency from billing operations.
*   **Security**:
    *   **Input Validation**: Enforces max body size (10MB) and max message depth (50) to prevent abuse.
    *   **Tenant Isolation**: Strict validation of `Authorization` headers.

## 🏗️ Architecture

```mermaid
graph LR
    Client --> ALB[Load Balancer]
    ALB --> Gateway[LLM Gateway (ECS)]
    
    subgraph "Gateway Core"
        Gateway --> Auth[Auth Middleware]
        Gateway --> RL[Rate Limit (Redis)]
        Gateway --> Proxy[Proxy Handler]
    end
    
    Auth -.-> DDB[(DynamoDB Tenants)]
    Proxy --> Upstream[LLM Providers]
    Proxy -.-> Usage[(DynamoDB Usage)]
```

### Components
*   **Store**: DynamoDB (Tenants, Models), Redis (Rate Limits).
*   **Middleware**: Auth, RateLimit, Metrics, OpenTelemetry.
*   **Proxy**: Handles request forwarding, streaming parsing, and failover logic.

## 🛠️ Getting Started

### Prerequisites
*   **Go** 1.23+
*   **Docker**
*   **Redis** (Local or ElastiCache)
*   **DynamoDB** (Local or AWS)

### Local Development

1.  **Clone & Dig In**:
    ```bash
    git clone https://github.com/user/llm-gateway.git
    cd llm-gateway
    ```

2.  **Configuration**:
    The service is configured via Environment Variables:
    ```bash
    export SERVER_PORT=8080
    export AWS_REGION=us-east-1
    export DYNAMODB_TABLE_NAME=LLMGateway_Tenants
    export REDIS_ADDR=localhost:6379
    export ADMIN_API_KEY=secret_admin
    ```

3.  **Run Locally**:
    ```bash
    go run cmd/server/main.go
    ```

### Docker Build

```bash
docker build -t llm-gateway .
docker run -p 8080:8080 --env-file .env llm-gateway
```

## 🔌 API Usage

### 1. Chat Completion (OpenAI Compatible)
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer <YOUR_TENANT_KEY>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### 2. Admin API (Create Tenant)
```bash
curl -X POST http://localhost:8080/admin/tenants \
  -H "X-Admin-Key: secret_admin" \
  -d '{
    "tenant_id": "vip_user",
    "api_key": "sk-vip-123",
    "rpm_limit": 1000,
    "tpm_limit": 500000,
    "allowed_models": ["*"]
  }'
```

---

## 🧪 Testing

The project includes a comprehensive test suite (Unit + Integration):

```bash
# Run all unit tests
go test ./...

# Run load tests (requires k6)
k6 run load_tests/script.js
```
