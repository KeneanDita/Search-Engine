package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"

	"github.com/searchengine/go-api/internal/cache"
	"github.com/searchengine/go-api/internal/handlers"
	"github.com/searchengine/go-api/internal/middleware"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Config
	port := getEnv("PORT", "8080")
	redisURL := getEnv("REDIS_URL", "redis://redis:6379")
	osAddr := getEnv("OPENSEARCH_URL", "http://opensearch:9200")
	nlpURL := getEnv("NLP_URL", "http://python-nlp:8001")
	rateRPS := 20.0
	rateBurst := 50

	// Redis
	rOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Fatal("parse redis url", zap.Error(err))
	}
	rdb := redis.NewClient(rOpts)
	defer rdb.Close()

	// Cache (5-minute TTL for search results)
	searchCache := cache.NewRedisCache(rdb, 5*time.Minute, logger)

	// Handlers
	searchH := handlers.NewSearchHandler(osAddr, nlpURL, searchCache, logger)
	docH := handlers.NewDocumentHandler(osAddr, logger)
	healthH := handlers.NewHealthHandler(osAddr, nlpURL, logger)

	// Rate limiter
	rl := middleware.NewRateLimiter(rateRPS, rateBurst, logger)

	// Fiber app
	app := fiber.New(fiber.Config{
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
				"code":  500,
			})
		},
	})

	// Global middleware
	app.Use(requestid.New())
	app.Use(middleware.Recovery(logger))
	app.Use(middleware.RequestLogger(logger))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, X-Request-ID",
	}))
	app.Use(compress.New())

	// Routes
	api := app.Group("/api/v1", rl.Middleware())

	api.Get("/search", searchH.Search)
	api.Get("/document/:id", docH.GetDocument)

	app.Get("/health", healthH.Health)
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("search API starting", zap.String("port", port))
		if err := app.Listen(":" + port); err != nil {
			logger.Error("server error", zap.Error(err))
		}
	}()

	<-quit
	logger.Info("shutting down")
	if err := app.ShutdownWithTimeout(5 * time.Second); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
	logger.Info("search API stopped")
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
