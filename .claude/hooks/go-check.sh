#!/usr/bin/env bash
# PostToolUse (Edit|Write): gofmt the edited Go file and vet its package.
# Exit 2 feeds vet failures back to Claude.
file=$(jq -r '.tool_input.file_path // empty')
case "$file" in *.go) ;; *) exit 0 ;; esac
[ -f "$file" ] || exit 0

gofmt -w "$file"
cd "${CLAUDE_PROJECT_DIR:-$(dirname "$0")/../..}" || exit 0
dir=$(dirname "$file")
rel=${dir#"$PWD"/}
if ! out=$(go vet "./$rel" 2>&1); then
  echo "go vet ./$rel failed:" >&2
  echo "$out" >&2
  exit 2
fi
exit 0
