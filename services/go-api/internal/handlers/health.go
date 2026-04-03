package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// HealthHandler checks all downstream dependencies.
type HealthHandler struct {
	osAddr  string
	nlpURL  string
	pgDSN   string
	logger  *zap.Logger
	client  *http.Client
}

// NewHealthHandler creates the health handler.
func NewHealthHandler(osAddr, nlpURL string, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		osAddr: osAddr,
		nlpURL: nlpURL,
		logger: logger,
		client: &http.Client{Timeout: 3 * time.Second},
	}
}

// Health handles GET /health
func (h *HealthHandler) Health(c *fiber.Ctx) error {
	checks := map[string]string{
		"opensearch": h.checkHTTP(c.Context(), h.osAddr+"/_cluster/health"),
		"nlp":        h.checkHTTP(c.Context(), h.nlpURL+"/health"),
	}
	allOK := true
	for _, v := range checks {
		if v != "ok" {
			allOK = false
		}
	}
	status := fiber.StatusOK
	if !allOK {
		status = fiber.StatusServiceUnavailable
	}
	return c.Status(status).JSON(fiber.Map{
		"status":  map[bool]string{true: "ok", false: "degraded"}[allOK],
		"service": "go-api",
		"checks":  checks,
	})
}

func (h *HealthHandler) checkHTTP(ctx context.Context, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "error: " + err.Error()
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "unreachable"
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return "unhealthy"
	}
	return "ok"
}
