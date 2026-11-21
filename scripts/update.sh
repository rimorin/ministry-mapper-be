#!/bin/bash
set -euo pipefail

# Update all Go packages
echo "🔄 Updating Go packages..."
go get -u ./...

# The SQLite driver is built against exact versions of these two libraries,
# and "go get -u" moves them ahead of it. Put them back to whatever the
# driver's own go.mod asks for, so this follows the driver when it upgrades.
SQLITE_DIR="$(go list -m -f '{{.Dir}}' modernc.org/sqlite)"
for module in modernc.org/libc modernc.org/memory; do
  wanted="$(awk -v m="$module" '$1 == m { print $2 }' "$SQLITE_DIR/go.mod")"
  if [ -z "$wanted" ]; then
    echo "❌ Could not read the version of $module that modernc.org/sqlite needs."
    exit 1
  fi
  echo "📌 Keeping $module at $wanted, as modernc.org/sqlite needs"
  go get "$module@$wanted"
done

# Tidy up the go.mod file
echo "🧹 Tidying up go.mod..."
go mod tidy

# Success message
echo "✅ Go packages updated successfully."
