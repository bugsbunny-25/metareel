---
name: local-verification
description: Run the full metareel stack locally against a copy of real data to verify backend and admin-UI changes end to end (API auth, endpoints, UI in the browser). Use after implementing a feature, before reporting it done.
---

# Local end-to-end verification

Unit tests are not enough for this project: several bugs (sqlc param numbering,
upstream field names, stuck runs, UI layout) only showed up when running for real.

## 1. Prepare an isolated copy

```bash
S=<scratchpad dir>
cp data/metareel.db $S/test.db && rm -f $S/test.db-wal $S/test.db-shm
sqlite3 $S/test.db "update task_schedules set enabled=0"   # never scrape from a test run
go build -o $S/metareel ./cmd/server
(cd web && npm run build)                                  # only if the UI changed
```

Never point a test server at `data/metareel.db` itself.

## 2. Start Redis and the server (background tasks)

```bash
go -C tools run ./cmd/devredis --addr=127.0.0.1:16379        # spare port, background
DATABASE_URL="file:$S/test.db?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)" \
SERVER_PORT=18080 REDIS_ADDR=127.0.0.1:16379 UI_SERVE_STATIC=true UI_STATIC_DIR=web/dist \
LOG_FILE_PATH=$S/server.log $S/metareel                       # background
```

`.env` is loaded too (TMDB key etc.); explicit env vars win. Wait for `curl localhost:18080/healthz`.

## 3. Check the API

- Public routes need a key: create one via `POST /api/v1/admin/api-keys {"name":"test"}` and send `X-API-Key`. Verify 401 without it.
- Admin copies (`/api/v1/admin/...`) need no key.
- Cross-check numbers against SQL on the copy (`sqlite3 $S/test.db ...`).
- Exercise error paths: bad params → 400, unknown ids → 404.
- Check `$S/server.log` for warnings (migrations applied, `public api access`, errors).

## 4. Check the UI (built-in browser)

- `http://localhost:18080/#/...`; set a 1440×900 viewport (reset to desktop after).
- Prefer `find`/`read_page` refs over pixel coordinates for clicks; take screenshots/zooms to judge layout.
- Walk every page touched, open drawers, try one real edit on the copy, and check both themes (`document.documentElement.dataset.theme='light'`).

## 5. Clean up

Stop the server and devredis tasks; reset the browser viewport. Report what was
verified live versus only in tests.
