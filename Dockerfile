# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Install git and ssl certificates
RUN apk add --no-cache git ca-certificates

# Copy go mod file
COPY go.mod ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o llm-gateway ./cmd/server

# Final Stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy binary from builder
COPY --from=builder /app/llm-gateway /llm-gateway

# User nonroot:nonroot
USER 65532:65532

EXPOSE 8080

ENTRYPOINT ["/llm-gateway"]
