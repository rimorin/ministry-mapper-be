#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TEST_DB_DIR="$PROJECT_ROOT/test_pb_data"
BOOT_TIMEOUT=60
PB_PORT=19090
TEST_BINARY="$PROJECT_ROOT/test-server"

cd "$PROJECT_ROOT"

SERVER_PID=""
cleanup() {
  # SERVER_PID is cleared after bootstrap stop; this only fires if the
  # script is interrupted before the explicit stop above.
  if [ -n "${SERVER_PID:-}" ] && kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "🛑 Stopping bootstrap server..."
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
    lsof -ti tcp:$PB_PORT | xargs kill -9 2>/dev/null || true
  fi
  echo "🗑️  Removing test_pb_data and binary..."
  rm -rf "$TEST_DB_DIR" "$TEST_BINARY"
}
trap cleanup EXIT INT TERM

# Kill any leftover process on the port from a previous interrupted run
lsof -ti tcp:$PB_PORT | xargs kill -9 2>/dev/null || true

# Step 1: Generate fresh test DB
echo "🗑️  Removing old test_pb_data and binary..."
rm -rf "$TEST_DB_DIR" "$TEST_BINARY"

echo "🔨 Compiling test server binary..."
go build -tags testdata -o "$TEST_BINARY" .

echo "🚀 Generating fresh test DB with seed data..."
"$TEST_BINARY" serve --http=127.0.0.1:$PB_PORT --dir="$TEST_DB_DIR" &
SERVER_PID=$!

# Step 2: Poll until PocketBase's health endpoint responds, meaning all
# migrations have run. Falls back to BOOT_TIMEOUT if it never comes up.
echo "⏳ Waiting for server to be ready (up to ${BOOT_TIMEOUT}s)..."
for i in $(seq 1 $BOOT_TIMEOUT); do
  if curl -sf "http://127.0.0.1:$PB_PORT/api/health" >/dev/null 2>&1; then
    echo "✅ Server ready after ${i}s"
    break
  fi
  if [ "$i" -eq "$BOOT_TIMEOUT" ]; then
    echo "❌ Server did not become ready within ${BOOT_TIMEOUT}s"
    exit 1
  fi
  sleep 1
done

echo "🛑 Stopping bootstrap server..."
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true
lsof -ti tcp:$PB_PORT | xargs kill -9 2>/dev/null || true
SERVER_PID=""

# Checkpoint WAL so the DB files are fully flushed before tests read them
echo "💾 Checkpointing WAL files..."
sqlite3 "$TEST_DB_DIR/data.db"      "PRAGMA wal_checkpoint(TRUNCATE);"
sqlite3 "$TEST_DB_DIR/auxiliary.db" "PRAGMA wal_checkpoint(TRUNCATE);" 2>/dev/null || true

echo "🔍 Verifying seed data..."
CONGS=$(sqlite3 "$TEST_DB_DIR/data.db" "SELECT COUNT(*) FROM congregations;")
USERS=$(sqlite3 "$TEST_DB_DIR/data.db" "SELECT COUNT(*) FROM users;")
ADDRS=$(sqlite3 "$TEST_DB_DIR/data.db" "SELECT COUNT(*) FROM addresses;")

if [ "$CONGS" -lt 2 ] || [ "$USERS" -lt 5 ] || [ "$ADDRS" -lt 30 ]; then
  echo "❌ Seed verification failed (congregations=$CONGS users=$USERS addresses=$ADDRS)"
  exit 1
fi
echo "✅ Seed verified: ${CONGS} congregations, ${USERS} users, ${ADDRS} addresses"

# Step 3: Run integration tests — tests.NewTestApp copies the DB to a temp
# dir per test, so no live server is needed here.
echo "🧪 Running integration tests..."
go test -tags testdata -v -timeout 120s ./internal/setup/

# Steps 4 & 5: cleanup trap stops the server and removes test_pb_data on EXIT
echo "✅ All tests passed."
