#!/usr/bin/env bash
set -euo pipefail

echo "=== Tempest Benchmark Repository Verification ==="

COMMITS=$(GIT_CONFIG_GLOBAL=/dev/null git log --oneline | wc -l | tr -d ' ')
echo "Total commits in branch: $COMMITS"

echo "Running build..."
GIT_CONFIG_GLOBAL=/dev/null go build -buildvcs=false ./...

echo "Running full test suite..."
GIT_CONFIG_GLOBAL=/dev/null go test ./...

echo "Running race detector..."
GIT_CONFIG_GLOBAL=/dev/null go test -race ./...

echo "Running static analysis..."
GIT_CONFIG_GLOBAL=/dev/null go vet ./...

echo "=== All Verification Checks Passed! ==="
