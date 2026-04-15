package ranking

import (
	"sort"

	"github.com/searchengine/go-api/internal/models"
)

const (
	defaultKeywordWeight  = 0.4
	defaultSemanticWeight = 0.6
)

// HybridConfig controls the fusion weights.
type HybridConfig struct {
	KeywordWeight  float64
	SemanticWeight float64
}

// DefaultHybridConfig returns balanced weights.
func DefaultHybridConfig() HybridConfig {
	return HybridConfig{
		KeywordWeight:  defaultKeywordWeight,
		SemanticWeight: defaultSemanticWeight,
	}
}

// RRFScore computes Reciprocal Rank Fusion score.
// k is the smoothing constant (typically 60).
func RRFScore(rankKeyword, rankSemantic, k int) float64 {
	return 1.0/float64(k+rankKeyword) + 1.0/float64(k+rankSemantic)
}

// HybridResult holds a hit with both sub-scores.
type HybridResult struct {
	Hit           models.SearchHit
	KeywordScore  float64
	SemanticScore float64
	HybridScore   float64
	KeywordRank   int
	SemanticRank  int
}

// FuseResults combines keyword and semantic result lists using
// Reciprocal Rank Fusion (RRF) — works without score normalisation.
func FuseResults(
	keywordHits []models.SearchHit,
	semanticHits []models.SearchHit,
	cfg HybridConfig,
) []models.SearchHit {
	type entry struct {
		hit           models.SearchHit
		keywordRank   int
		semanticRank  int
		keywordScore  float64
		semanticScore float64
	}

	byID := make(map[string]*entry)

	for rank, h := range keywordHits {
		if e, ok := byID[h.ID]; ok {
			e.keywordRank = rank + 1
			e.keywordScore = h.Score
		} else {
			cp := h
			byID[h.ID] = &entry{hit: cp, keywordRank: rank + 1, keywordScore: h.Score, semanticRank: len(keywordHits) + len(semanticHits)}
		}
	}
	for rank, h := range semanticHits {
		if e, ok := byID[h.ID]; ok {
			e.semanticRank = rank + 1
			e.semanticScore = h.Score
		} else {
			cp := h
			byID[h.ID] = &entry{hit: cp, semanticRank: rank + 1, semanticScore: h.Score, keywordRank: len(keywordHits) + len(semanticHits)}
		}
	}

	const k = 60
	type scored struct {
		entry *entry
		rrf   float64
	}
	items := make([]scored, 0, len(byID))
	for _, e := range byID {
		rrf := RRFScore(e.keywordRank, e.semanticRank, k)
		items = append(items, scored{entry: e, rrf: rrf})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].rrf == items[j].rrf {
			return items[i].entry.hit.ID < items[j].entry.hit.ID
		}
		return items[i].rrf > items[j].rrf
	})

	result := make([]models.SearchHit, 0, len(items))
	for _, s := range items {
		h := s.entry.hit
		h.Score = s.rrf
		h.KeywordScore = s.entry.keywordScore
		h.SemanticScore = s.entry.semanticScore
		result = append(result, h)
	}
	return result
}

// WeightedFuse combines scores with explicit weights (alternative to RRF).
func WeightedFuse(
	keywordHits []models.SearchHit,
	semanticHits []models.SearchHit,
	cfg HybridConfig,
) []models.SearchHit {
	// Normalise keyword scores
	kScores := make([]float64, len(keywordHits))
	sScores := make([]float64, len(semanticHits))
	for i, h := range keywordHits {
		kScores[i] = h.Score
	}
	for i, h := range semanticHits {
		sScores[i] = h.Score
	}
	kNorm := NormaliseBM25(kScores)
	sNorm := NormaliseBM25(sScores)

	byID := make(map[string]models.SearchHit)
	kScore := make(map[string]float64)
	sScore := make(map[string]float64)

	for i, h := range keywordHits {
		byID[h.ID] = h
		kScore[h.ID] = kNorm[i]
	}
	for i, h := range semanticHits {
		if _, exists := byID[h.ID]; !exists {
			byID[h.ID] = h
		}
		sScore[h.ID] = sNorm[i]
	}

	out := make([]models.SearchHit, 0, len(byID))
	for id, h := range byID {
		combined := cfg.KeywordWeight*kScore[id] + cfg.SemanticWeight*sScore[id]
		h.Score = combined
		h.KeywordScore = kScore[id]
		h.SemanticScore = sScore[id]
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Score > out[j].Score
	})
	return out
}
