#!/usr/bin/env bash
# integration_test.sh — Full pipeline smoke test
# Runs against a live stack (docker compose up must be running)
set -euo pipefail

BASE_API="http://localhost:8080"
CRAWLER="http://localhost:8000"
NLP="http://localhost:8001"
INDEXER="http://localhost:8081"

PASS=0
FAIL=0

run_test() {
  local name="$1"
  local result="$2"
  local expected="$3"
  if echo "$result" | grep -q "$expected"; then
    echo "  ✓ $name"
    ((PASS++))
  else
    echo "  ✗ $name (expected '$expected' in: $result)"
    ((FAIL++))
  fi
}

echo ""
echo "══════════════════════════════════════════"
echo "  Search Engine Integration Tests"
echo "══════════════════════════════════════════"

# ── Health checks ─────────────────────────────────────────────────────────
echo ""
echo "[ Health Checks ]"
run_test "go-api health"       "$(curl -sf $BASE_API/health)"         '"status"'
run_test "crawler health"      "$(curl -sf $CRAWLER/health)"          '"status"'
run_test "nlp health"          "$(curl -sf $NLP/health)"              '"status"'
run_test "indexer health"      "$(curl -sf $INDEXER/health)"          '"status"'

# ── Seed data ─────────────────────────────────────────────────────────────
echo ""
echo "[ Seeding Test Document ]"
SEED_RESPONSE=$(curl -sf -X POST $INDEXER/index \
  -H "Content-Type: application/json" \
  -d '{
    "documents": [{
      "id": "test001",
      "url": "https://en.wikipedia.org/wiki/Machine_learning",
      "title": "Machine Learning - Wikipedia",
      "content": "Machine learning is a branch of artificial intelligence and computer science which focuses on the use of data and algorithms.",
      "tokens": ["machine", "learning", "artificial", "intelligence", "algorithm"],
      "keyphrases": ["machine learning", "artificial intelligence"],
      "entities": {"ORG": ["Wikipedia"]},
      "embedding": [],
      "word_count": 21,
      "language": "en",
      "source": "test"
    }]
  }')
run_test "seed document indexed" "$SEED_RESPONSE" '"indexed"'

# Wait for OpenSearch to index
sleep 2

# ── Search ────────────────────────────────────────────────────────────────
echo ""
echo "[ Search Queries ]"
SEARCH=$(curl -sf "$BASE_API/api/v1/search?q=machine+learning&mode=keyword")
run_test "keyword search returns response" "$SEARCH" '"query"'
run_test "keyword search has hits field"   "$SEARCH" '"hits"'

HYBRID=$(curl -sf "$BASE_API/api/v1/search?q=artificial+intelligence&mode=hybrid")
run_test "hybrid search responds"          "$HYBRID"  '"query"'

# ── NLP endpoint ──────────────────────────────────────────────────────────
echo ""
echo "[ NLP Processing ]"
EMBED=$(curl -sf -X POST $NLP/embed \
  -H "Content-Type: application/json" \
  -d '{"text": "search engine technology"}')
run_test "embed returns vector"   "$EMBED" '"embedding"'
run_test "embed has dimension"    "$EMBED" '"dimension"'

PROCESS=$(curl -sf -X POST $NLP/process \
  -H "Content-Type: application/json" \
  -d '{"documents": [{"url": "https://example.com", "title": "Test", "content": "The quick brown fox jumps over the lazy dog near the river."}]}')
run_test "process returns tokens"     "$PROCESS" '"tokens"'
run_test "process returns keyphrases" "$PROCESS" '"keyphrases"'

# ── Crawler stats ─────────────────────────────────────────────────────────
echo ""
echo "[ Crawler Stats ]"
STATS=$(curl -sf $CRAWLER/stats)
run_test "crawler stats respond" "$STATS" '"queued_documents"'

# ── Summary ───────────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════"
echo "  Results: ${PASS} passed, ${FAIL} failed"
echo "══════════════════════════════════════════"
echo ""
[ "$FAIL" -eq 0 ] && exit 0 || exit 1
