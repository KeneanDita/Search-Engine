// Package ranking implements BM25 and hybrid re-ranking.
package ranking

import (
	"math"
	"strings"
)

// BM25Params holds the k1 and b parameters for BM25.
type BM25Params struct {
	K1 float64
	B  float64
}

// DefaultBM25Params returns the standard BM25 parameters.
func DefaultBM25Params() BM25Params {
	return BM25Params{K1: 1.5, B: 0.75}
}

// BM25Scorer scores documents using the BM25 ranking function.
type BM25Scorer struct {
	params  BM25Params
	avgDocLen float64
	docCount  int
	idf       map[string]float64
}

// NewBM25Scorer creates a scorer pre-computed with corpus statistics.
// docLengths: number of tokens per document
// termDocFreq: for each term, how many documents contain it
func NewBM25Scorer(docLengths []int, termDocFreq map[string]int, params BM25Params) *BM25Scorer {
	if len(docLengths) == 0 {
		return &BM25Scorer{params: params}
	}
	total := 0
	for _, l := range docLengths {
		total += l
	}
	avgLen := float64(total) / float64(len(docLengths))
	N := len(docLengths)

	idf := make(map[string]float64, len(termDocFreq))
	for term, df := range termDocFreq {
		// Robertson-Spärck Jones IDF with smoothing
		idf[term] = math.Log((float64(N)-float64(df)+0.5)/(float64(df)+0.5) + 1)
	}
	return &BM25Scorer{
		params:    params,
		avgDocLen: avgLen,
		docCount:  N,
		idf:       idf,
	}
}

// ScoreDoc computes the BM25 score for a document given query terms.
// docTokens: tokenised content of the document
// queryTerms: tokenised query
// docLen: total token count of the document
func (b *BM25Scorer) ScoreDoc(docTokens []string, queryTerms []string, docLen int) float64 {
	if b.avgDocLen == 0 {
		return 0
	}
	// Build term frequency map for this document
	tf := make(map[string]int, len(docTokens))
	for _, t := range docTokens {
		tf[strings.ToLower(t)]++
	}

	score := 0.0
	for _, term := range queryTerms {
		term = strings.ToLower(term)
		freq := float64(tf[term])
		if freq == 0 {
			continue
		}
		idfVal := b.idf[term]
		if idfVal == 0 {
			// If term not in corpus, use IDF assuming df=1
			idfVal = math.Log((float64(b.docCount)-1.0+0.5)/(1.0+0.5) + 1)
		}
		numerator := freq * (b.params.K1 + 1)
		denominator := freq + b.params.K1*(1-b.params.B+b.params.B*float64(docLen)/b.avgDocLen)
		score += idfVal * numerator / denominator
	}
	return score
}

// NormaliseBM25 normalises BM25 scores to [0,1] range.
func NormaliseBM25(scores []float64) []float64 {
	if len(scores) == 0 {
		return scores
	}
	max := scores[0]
	for _, s := range scores[1:] {
		if s > max {
			max = s
		}
	}
	if max == 0 {
		return scores
	}
	out := make([]float64, len(scores))
	for i, s := range scores {
		out[i] = s / max
	}
	return out
}
