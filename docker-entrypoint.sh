#!/bin/sh
# Restores data.db from the Litestream backup when it is missing, then starts PocketBase.
set -e

DB=/app/pb_data/data.db

if [ -n "$LITESTREAM_REPLICA_URL" ] && [ ! -e "$DB" ]; then
  # A WAL left without its database would be applied to the restored file.
  rm -f "$DB-wal" "$DB-shm"
  litestream restore -if-replica-exists -integrity-check quick -o "$DB" "$LITESTREAM_REPLICA_URL"
fi

exec ./main serve --http=0.0.0.0:8080
