package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/searchengine/go-indexer/internal/models"
)

const (
	processedQueue = "queue:processed"
	batchSize      = 32
	pollTimeout    = 5 * time.Second
)

// Consumer reads processed documents from Redis and calls the index function.
type Consumer struct {
	rdb    *redis.Client
	logger *zap.Logger
	indexFn func(ctx context.Context, docs []models.ProcessedDocument) (int, []error)
}

// NewConsumer creates a new queue consumer.
func NewConsumer(
	rdb *redis.Client,
	logger *zap.Logger,
	indexFn func(ctx context.Context, docs []models.ProcessedDocument) (int, []error),
) *Consumer {
	return &Consumer{rdb: rdb, logger: logger, indexFn: indexFn}
}

// Run starts the consumer loop. Blocks until ctx is cancelled.
func (c *Consumer) Run(ctx context.Context, workers int) error {
	g, gctx := errgroup.WithContext(ctx)
	for i := 0; i < workers; i++ {
		workerID := i
		g.Go(func() error {
			return c.workerLoop(gctx, workerID)
		})
	}
	return g.Wait()
}

func (c *Consumer) workerLoop(ctx context.Context, id int) error {
	c.logger.Info("queue worker started", zap.Int("worker_id", id), zap.String("queue", processedQueue))
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		batch, err := c.readBatch(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			c.logger.Error("read batch failed", zap.Int("worker_id", id), zap.Error(err))
			time.Sleep(time.Second)
			continue
		}
		if len(batch) == 0 {
			continue
		}

		indexed, errs := c.indexFn(ctx, batch)
		if len(errs) > 0 {
			for _, e := range errs {
				c.logger.Error("index error", zap.Error(e))
			}
		}
		c.logger.Info("batch indexed", zap.Int("worker_id", id), zap.Int("indexed", indexed), zap.Int("total", len(batch)))
	}
}

func (c *Consumer) readBatch(ctx context.Context) ([]models.ProcessedDocument, error) {
	// Block on first item
	result, err := c.rdb.BLPop(ctx, pollTimeout, processedQueue).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("blpop: %w", err)
	}

	docs := make([]models.ProcessedDocument, 0, batchSize)
	// Parse the first item
	var first models.ProcessedDocument
	if err := json.Unmarshal([]byte(result[1]), &first); err != nil {
		c.logger.Error("unmarshal doc", zap.Error(err))
	} else {
		docs = append(docs, first)
	}

	// Drain up to batchSize-1 more items non-blocking
	for i := 1; i < batchSize; i++ {
		raw, err := c.rdb.LPop(ctx, processedQueue).Result()
		if err == redis.Nil {
			break
		}
		if err != nil {
			break
		}
		var d models.ProcessedDocument
		if err := json.Unmarshal([]byte(raw), &d); err == nil {
			docs = append(docs, d)
		}
	}
	return docs, nil
}
