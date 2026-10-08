---
name: debug-instance
description: Diagnose a running metareel deployment (scrapes not happening, failing runs, missing data, auth errors) from its logs and admin API. Use when the user reports the container/instance misbehaving or shares data/metareel.log.
---

# Debugging a deployed instance

The user's instance is reachable on the LAN (ask for the host if unknown; it has been
`http://kundan-eq13.local:5080`). Everything below is read-only unless noted.

## 1. Start from the admin API, not the log file

The log file shows what was logged, and older builds logged **only successes** — a
gap in the log usually means runs were failing, not that nothing ran.

```bash
B=http://<host>:<port>/api/v1/admin
curl -s $B/stats | jq .                       # run counts 24h/7d, running, schedule health, mapping coverage
curl -s "$B/task-runs?limit=50" | jq '.[] | {id, schedule_id, status, retry_count, started_at, error_message}'
curl -s "$B/task-runs?status=failed&limit=200" | jq -r '.[].error_message' | sort | uniq -c
curl -s $B/task-runs/<id>/logs | jq -r '.[] | "\(.created_at) \(.level) \(.message)"'
curl -s $B/task-schedules | jq .
```

`/task-schedules/{id}/runs?limit=500` gives a long history per schedule — group by
date + status to find when failures started.

## 2. Read the log file efficiently

It is JSON lines; summarize instead of reading it whole:

```bash
python3 -c "import json,collections,sys; c=collections.Counter((j['level'],j['msg']) for j in map(json.loads,open(sys.argv[1]))); [print(v,k) for k,v in c.most_common(40)]" data/metareel.log
```

Useful messages: `scheduled task enqueued`, `flixpatrol task started|failed|completed`,
`flixpatrol fetch failed`, `title mapped|not mapped`, `marked interrupted task runs as failed`,
`public api access`, `component=asynq` lines (Redis/enqueue problems). `server listening`
marks restarts.

## 3. Known failure signatures

| Symptom | Cause | Fix |
|---|---|---|
| `403 ... <title>Just a moment...` | FlixPatrol Cloudflare challenge | Set `FLARESOLVERR_URL`; check FlareSolverr health `GET http://<fs>:8191/` |
| `flaresolverr ... Timeout after N seconds` | Challenge not solved in time | Raise `FLARESOLVERR_MAX_TIMEOUT`; check the FlareSolverr container |
| `missing section heading: TOP 10 Movies` | Page layout changed or empty/blocked page | Fetch the page (via FlareSolverr) and update `parseTop10Table` |
| Runs stuck `started` | Server restarted mid-run | Auto-failed after the task timeout + 10 min; no action |
| Public API `401` | Missing/rotated API key | Settings & API keys in the admin UI |
| Public API `429` | Per-key rate limit | Raise `rate_limit_per_minute` in Settings |
| Many `title not mapped` | Upstream naming/year mismatch | Titles page → Missing TMDB → Find matches |

## 4. Reproduce the scrape without touching the DB

```bash
go run ./cmd/fpcheck -flaresolverr http://<host>:8191 -titles 5          # chart + mapping, debug logs
go run ./cmd/fpcheck -flaresolverr http://<host>:8191 -slug <flixpatrol-slug>
```

Ask the user to run commands that go through FlareSolverr against FlixPatrol;
don't solve bot challenges yourself. Reading the instance's admin API is fine.

## 5. Reporting

Lead with the root cause and evidence (counts, first failure date, one example error),
then what was changed and what the user must do (env vars, keys, redeploy).
