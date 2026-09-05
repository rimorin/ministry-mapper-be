#!/bin/bash
# PostToolUse hook: after Claude edits a Go file, fail loudly if it is not
# gofmt-clean or its package does not pass go vet. Exit 2 feeds the output back
# to Claude so it fixes the file before moving on.
set -u
file=$(python3 -c 'import json,sys; print(json.load(sys.stdin).get("tool_input", {}).get("file_path", ""))' 2>/dev/null)
case "$file" in
  *.go) ;;
  *) exit 0 ;;
esac
[ -f "$file" ] || exit 0

cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}" || exit 0

unformatted=$(gofmt -l "$file")
if [ -n "$unformatted" ]; then
  echo "gofmt: $file is not formatted. Run: gofmt -w $file" >&2
  exit 2
fi

pkg=$(python3 -c 'import os,sys; print(os.path.relpath(sys.argv[1], os.getcwd()))' "$(dirname "$file")")
tags=""
grep -q '^//go:build testdata' "$file" && tags="-tags testdata"
if ! out=$(go vet $tags "./$pkg" 2>&1); then
  echo "go vet ./$pkg failed:" >&2
  echo "$out" >&2
  exit 2
fi
exit 0
