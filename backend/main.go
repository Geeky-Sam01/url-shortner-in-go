package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	_ "github.com/url-shortner/backend/docs"
	"github.com/url-shortner/backend/config"
	"github.com/url-shortner/backend/db"
	"github.com/url-shortner/backend/handlers"
	"github.com/url-shortner/backend/redis"
	"github.com/url-shortner/backend/worker"
)

// @title           High-Performance URL Shortener API
// @version         1.0
// @description     A highly scalable, resume-worthy URL shortener with edge-redirection and analytics.
// @host            localhost:8080
// @BasePath        /

func main() {
	// Load local .env file if it exists. Ignore failure since production uses environment variables directly.
	_ = godotenv.Load()

	// Initialize structured logging: Text Handler for local dev, JSON Handler for production.
	var handler slog.Handler
	if os.Getenv("ENV") == "production" {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	if cfg.FrontendURL == "" {
		slog.Error("FRONTEND_URL is required")
		os.Exit(1)
	}

	// 2. Connect to PostgreSQL
	database, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := db.RunMigrations(database); err != nil {
		slog.Error("database migrations failed", "error", err)
		os.Exit(1)
	}

	// 3. Connect to Redis (Upstash)
	redisClient, err := redis.Connect(cfg.RedisURL)
	if err != nil {
		slog.Error("redis connection failed", "error", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	// 4. Start Analytics Worker
	analyticsWorker := worker.NewAnalyticsWorker(database, redisClient)
	analyticsWorker.Start()

	// 5. Setup Gin Router
	if os.Getenv("ENV") == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}
	router := gin.Default()

	// Simple CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", cfg.FrontendURL)
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	urlHandler := &handlers.URLHandler{
		DB:          database,
		Redis:       redisClient,
		FrontendURL: cfg.FrontendURL,
	}

	// API Routes
	api := router.Group("/api")
	{
		api.POST("/shorten", urlHandler.CreateURL)
		api.GET("/urls", urlHandler.GetURLs)
		api.DELETE("/urls/:key", urlHandler.DeleteURL)
	}

	// Fallback Route
	router.GET("/:key", urlHandler.RedirectFallback)

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Swagger documentation route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// 6. Start Server
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		slog.Info("starting server", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server crashed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down server...")

	// 5 seconds timeout to shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
	} else {
		slog.Info("server gracefully stopped")
	}

	slog.Info("stopping background analytics worker...")
	analyticsWorker.Stop()

	slog.Info("server exiting")
}
