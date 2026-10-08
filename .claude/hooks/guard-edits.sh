#!/usr/bin/env bash
# PreToolUse (Edit|Write): block edits that should never be made by hand.
# Exit 2 blocks the tool call and shows stderr to Claude.
file=$(jq -r '.tool_input.file_path // empty')
[ -z "$file" ] && exit 0

case "$(basename "$file")" in
  .env)
    echo "Blocked: .env holds real secrets. Edit .env.example instead." >&2
    exit 2 ;;
esac

case "$file" in
  */internal/repository/sqlc/*.go)
    echo "Blocked: internal/repository/sqlc is generated. Edit db/queries/*.sql (or db/migrations) and run 'make sqlc'." >&2
    exit 2 ;;
  */web/dist/*)
    echo "Blocked: web/dist is build output. Edit web/src and run 'npm run build' in web/." >&2
    exit 2 ;;
esac
exit 0
