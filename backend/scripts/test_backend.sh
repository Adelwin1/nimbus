#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(
  cd "$(dirname "${BASH_SOURCE[0]}")/.." &&
  pwd
)"

cd "$ROOT_DIR"

if [[ ! -f .env.test ]]; then
  echo "Missing .env.test"
  echo "Copy .env.test.example to .env.test and configure TEST_DATABASE_URL."
  exit 1
fi

set -a
source .env.test
set +a

if [[ -z "${TEST_DATABASE_URL:-}" ]]; then
  echo "TEST_DATABASE_URL is missing."
  exit 1
fi

if [[ -n "${DATABASE_URL:-}" ]] &&
   [[ "$DATABASE_URL" == "$TEST_DATABASE_URL" ]]; then
  echo "TEST_DATABASE_URL must not equal DATABASE_URL."
  exit 1
fi

if ! command -v goose >/dev/null 2>&1; then
  echo "The goose command is required."
  exit 1
fi

echo "Applying test migrations..."
goose \
  -dir migrations \
  postgres \
  "$TEST_DATABASE_URL" \
  up

echo "Running backend tests..."
go test -count=1 ./...

echo "Running race detector..."
go test -race -count=1 ./...

echo "Running go vet..."
go vet ./...

echo "Backend test gate passed."
