package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/searchengine/go-indexer/internal/database"
	"github.com/searchengine/go-indexer/internal/indexer"
	"github.com/searchengine/go-indexer/internal/models"
	"github.com/searchengine/go-indexer/internal/queue"
)

var (
	docsIndexedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "indexer_docs_indexed_total",
		Help: "Total documents successfully indexed",
	})
	indexDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "indexer_duration_seconds",
		Help:    "Indexing duration",
		Buckets: prometheus.DefBuckets,
	})
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Config from env
	redisURL := getEnv("REDIS_URL", "redis://redis:6379")
	pgDSN := getEnv("POSTGRES_DSN", "postgres://searchengine:searchengine@postgres:5432/searchengine")
	osAddr := getEnv("OPENSEARCH_URL", "http://opensearch:9200")
	port := getEnv("PORT", "8081")
	workers := 4

	// Redis
	rOpts, err := redis.ParseURL(redisURL)
	if err != nil {
		logger.Fatal("parse redis url", zap.Error(err))
	}
	rdb := redis.NewClient(rOpts)
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Fatal("redis ping", zap.Error(err))
	}
	logger.Info("redis connected")

	// Postgres
	pgPool, err := database.NewPGPool(ctx, pgDSN, logger)
	if err != nil {
		logger.Fatal("postgres connect", zap.Error(err))
	}
	defer pgPool.Close()

	// OpenSearch
	osClient, err := indexer.NewOSClient(osAddr, logger)
	if err != nil {
		logger.Fatal("opensearch connect", zap.Error(err))
	}

	// Indexer
	idx := indexer.New(osClient, pgPool.Pool, logger)

	indexFn := func(ctx context.Context, docs []models.ProcessedDocument) (int, []error) {
		t := time.Now()
		n, errs := idx.IndexDocuments(ctx, docs)
		indexDuration.Observe(time.Since(t).Seconds())
		docsIndexedTotal.Add(float64(n))
		return n, errs
	}

	// Queue consumer
	consumer := queue.NewConsumer(rdb, logger, indexFn)

	// HTTP server (health + metrics + direct index endpoint)
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "go-indexer"})
	})
	mux.HandleFunc("/index", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req models.IndexRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		indexed, errs := indexFn(r.Context(), req.Documents)
		errStrs := make([]string, 0, len(errs))
		for _, e := range errs {
			errStrs = append(errStrs, e.Error())
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models.IndexResponse{
			Indexed: indexed,
			Failed:  len(req.Documents) - indexed,
			Errors:  errStrs,
		})
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", port),
		Handler: mux,
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		logger.Info("indexer HTTP server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		shutCtx, sc := context.WithTimeout(context.Background(), 5*time.Second)
		defer sc()
		return srv.Shutdown(shutCtx)
	})
	g.Go(func() error {
		return consumer.Run(gctx, workers)
	})

	if err := g.Wait(); err != nil {
		logger.Error("service error", zap.Error(err))
	}
	logger.Info("indexer service stopped")
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
