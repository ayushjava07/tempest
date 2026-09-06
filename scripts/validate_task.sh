#!/usr/bin/env bash
set -euo pipefail

TASK_ID="${1:?Usage: $0 <DEF-ID>}"
TASK_DIR="benchmarks/tasks/$TASK_ID"

echo "=== Validating Benchmark Task: $TASK_ID ==="

if [ ! -d "$TASK_DIR" ]; then
  echo "Error: Task directory $TASK_DIR does not exist"
  exit 1
fi

echo "Task directory found: $TASK_DIR"
echo "=== Validation complete for $TASK_ID ==="
