package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/user/llm-gateway/internal/admin"
	"github.com/user/llm-gateway/internal/config"
	"github.com/user/llm-gateway/internal/middleware"
	"github.com/user/llm-gateway/internal/proxy"
	"github.com/user/llm-gateway/internal/store"
	"github.com/user/llm-gateway/internal/telemetry"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

func main() {
	// Load Configuration
	cfg := config.LoadConfig()

	// Initialize Gin
	r := gin.Default()

	// Initialize Stores
	// Note: In real usage, pass real credentials/config
	tenantStore, err := store.NewDynamoDBTenantStore(context.Background(), cfg.AWSRegion, cfg.DynamoDBTableName)
	if err != nil {
		log.Fatalf("Failed to init DynamoDB: %v", err)
	}

	// Initialize Models Store
	modelStore, err := store.NewDynamoDBModelStore(context.Background(), cfg.AWSRegion, "LLMGateway_Models")
	if err != nil {
		log.Fatalf("Failed to init DynamoDB Models: %v", err)
	}

	// Initialize Usage Store
	usageStore, err := store.NewDynamoDBUsageStore(context.Background(), cfg.AWSRegion, "LLMGateway_UsageLogs")
	if err != nil {
		log.Fatalf("Failed to init Usage Store: %v", err)
	}

	rlStore := store.NewRedisRateLimitStore(cfg.RedisAddr, cfg.RedisPassword)

	// Initialize Telemetry (OpenTelemetry)
	tpShutdown, err := telemetry.InitTracer()
	if err != nil {
		slog.Error("Failed to init telemetry", "error", err)
		// Don't fatal, just log
	} else {
		defer func() {
			if err := tpShutdown(context.Background()); err != nil {
				slog.Error("Failed to shutdown telemetry", "error", err)
			}
		}()
	}

	// Initialize Handler
	proxyHandler := proxy.NewHandler(rlStore, modelStore, usageStore, cfg.LLMTimeout)

	// Register Middleware
	r.Use(otelgin.Middleware("llm-gateway"))
	r.Use(middleware.MetricsMiddleware()) // Prometheus Metrics (First to capture all)
	r.Use(middleware.AuthMiddleware(tenantStore))
	r.Use(middleware.RateLimitMiddleware(rlStore)) // Check RPM

	// Admin Routes (Protected)
	adminHandler := admin.NewAdminHandler(tenantStore, os.Getenv("ADMIN_API_KEY"))
	adminGroup := r.Group("/admin")
	adminGroup.Use(adminHandler.AuthMiddleware())
	adminGroup.POST("/tenants", adminHandler.CreateTenant)

	// Routes
	r.POST("/v1/chat/completions", proxyHandler.CreateCompletion)
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	// Metrics Endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Initialize Structured Logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Graceful Shutdown Setup
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: r,
	}

	// Start Server in Goroutine
	go func() {
		slog.Info("Starting server", "port", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server init failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for Interrupt Signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	// Context with 10s timeout for active requests and cleanup
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	// Wait for async tasks (Usage Logs)
	slog.Info("Waiting for async tasks to complete...")
	if err := proxyHandler.Shutdown(ctx); err != nil {
		slog.Error("Failed to complete async tasks", "error", err)
	}

	slog.Info("Server exiting")
}
