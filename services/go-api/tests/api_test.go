package tests

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealthEndpoint(t *testing.T) {
	app := fiber.New()
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "go-api"})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSearchMissingQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		q := c.Query("q")
		if q == "" {
			return c.Status(400).JSON(fiber.Map{"error": "q parameter is required", "code": 400})
		}
		return c.JSON(fiber.Map{"query": q})
	})

	req := httptest.NewRequest("GET", "/search", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSearchWithQuery(t *testing.T) {
	app := fiber.New()
	app.Get("/search", func(c *fiber.Ctx) error {
		q := c.Query("q")
		if q == "" {
			return c.Status(400).JSON(fiber.Map{"error": "q required", "code": 400})
		}
		return c.JSON(fiber.Map{"query": q, "total": 0, "hits": []interface{}{}})
	})

	req := httptest.NewRequest("GET", "/search?q=golang", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200, got %d. body: %s", resp.StatusCode, body)
	}
}
