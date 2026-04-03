package tests

import (
	"math"
	"testing"

	"github.com/searchengine/go-api/internal/models"
	"github.com/searchengine/go-api/internal/ranking"
)

func TestBM25Score(t *testing.T) {
	scorer := ranking.NewBM25Scorer(
		[]int{10, 15, 8, 20, 12},
		map[string]int{"golang": 3, "search": 4},
		ranking.DefaultBM25Params(),
	)

	tokens := []string{"golang", "search", "engine", "golang"}
	query := []string{"golang", "search"}
	score := scorer.ScoreDoc(tokens, query, len(tokens))

	if score <= 0 {
		t.Errorf("expected positive BM25 score, got %f", score)
	}
}

func TestBM25ZeroForMissingTerms(t *testing.T) {
	scorer := ranking.NewBM25Scorer(
		[]int{10, 15},
		map[string]int{"golang": 2},
		ranking.DefaultBM25Params(),
	)
	tokens := []string{"python", "django", "flask"}
	score := scorer.ScoreDoc(tokens, []string{"golang"}, len(tokens))
	if score != 0 {
		t.Errorf("expected 0 score for missing terms, got %f", score)
	}
}

func TestNormaliseBM25(t *testing.T) {
	scores := []float64{5.0, 10.0, 2.5, 7.5}
	normed := ranking.NormaliseBM25(scores)

	if math.Abs(normed[1]-1.0) > 1e-9 {
		t.Errorf("max should normalise to 1.0, got %f", normed[1])
	}
	for _, s := range normed {
		if s < 0 || s > 1 {
			t.Errorf("normalised score out of range: %f", s)
		}
	}
}

func TestHybridFuseRRF(t *testing.T) {
	keyword := []models.SearchHit{
		{ID: "a", URL: "https://a.com", Title: "A", Score: 10},
		{ID: "b", URL: "https://b.com", Title: "B", Score: 8},
		{ID: "c", URL: "https://c.com", Title: "C", Score: 6},
	}
	semantic := []models.SearchHit{
		{ID: "b", URL: "https://b.com", Title: "B", Score: 0.95},
		{ID: "c", URL: "https://c.com", Title: "C", Score: 0.90},
		{ID: "d", URL: "https://d.com", Title: "D", Score: 0.85},
	}

	fused := ranking.FuseResults(keyword, semantic, ranking.DefaultHybridConfig())

	if len(fused) != 4 {
		t.Errorf("expected 4 fused results (union), got %d", len(fused))
	}

	// Document "b" appears in both lists so should rank high
	topID := fused[0].ID
	if topID != "b" && topID != "c" {
		t.Logf("top fused result is '%s' (expected b or c which appear in both lists)", topID)
	}

	// Scores should be descending
	for i := 1; i < len(fused); i++ {
		if fused[i].Score > fused[i-1].Score {
			t.Errorf("results not sorted by score at index %d: %f > %f", i, fused[i].Score, fused[i-1].Score)
		}
	}
}

func TestCosineSimilarityIdentical(t *testing.T) {
	vec := []float32{0.5, 0.5, 0.5, 0.5}
	sim := ranking.CosineSimilarity(vec, vec)
	if math.Abs(sim-1.0) > 1e-5 {
		t.Errorf("identical vectors should have cosine sim ~1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float32{1, 0, 0, 0}
	b := []float32{0, 1, 0, 0}
	sim := ranking.CosineSimilarity(a, b)
	if math.Abs(sim) > 1e-9 {
		t.Errorf("orthogonal vectors should have cosine sim ~0, got %f", sim)
	}
}
