#!/bin/bash

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO_NAME="$(basename "$REPO_ROOT")"
OUTPUT_DIR="$(dirname "$REPO_ROOT")"
OUTPUT_FILE="$OUTPUT_DIR/stock_ticker_watcher_$(date +%Y%m%d_%H%M%S).zip"

cd "$OUTPUT_DIR"

zip -r "$OUTPUT_FILE" "$REPO_NAME" \
  -x "*/node_modules/*" \
  -x "*/dist/*" \
  -x "*/.git/*" \
  -x "*/.next/*" \
  -x "*/coverage/*" \
  -x "*/test-results/*" \
  -x "*/playwright-report/*" \
  -x "*/tmp/*" \
  -x "*/stocks.db" \
  -x "*/.DS_Store" \
  -x "*/.vite/*" \
  -x "*/vendor/*" \
  -x "*/.claude/*" \
  -x "*.zip"

echo "Created archive:"
echo "$OUTPUT_FILE"
