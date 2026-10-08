---
name: database-changes
description: How to add migrations and sqlc queries in metareel, including the SQLite/sqlc pitfalls that silently break queries (param numbering, ORDER BY params, nullable subqueries, date formats). Use whenever touching db/migrations, db/queries or internal/repository.
---

# Database changes (SQLite + goose + sqlc)

## Migrations

- Files are **sequential, 6 digits**: `db/migrations/000NNN_short_name.sql`. Create with `make migrate-new MIGRATION_NAME=add_x` (writes the next number with empty Up/Down blocks). Don't use plain `goose create` — it makes timestamped or 5-digit names.
- Each file has `-- +goose Up` and `-- +goose Down`; Down must undo Up.
- Migrations run automatically at server start (`repository.Migrate`) and in tests (`newTestDB`). Never edit a migration that has shipped (deployed DB already ran it); an unreleased one on your branch may be edited.
- After schema changes: `make sqlc`, then `go build ./...`.

## Writing queries (`db/queries/*.sql`)

Generated code lands in `internal/repository/sqlc` — **never hand-edit it** (it was hand-edited once and the next `make sqlc` broke the build). Always regenerate and review `git diff internal/repository/sqlc`.

Use named params — positional `?` produce `Column1 interface{}` fields:

```sql
WHERE (CAST(sqlc.arg(kind) AS TEXT) = '' OR kind = sqlc.arg(kind))        -- optional string filter
  AND (CAST(sqlc.arg(schedule_id) AS INTEGER) = 0 OR schedule_id = sqlc.arg(schedule_id))
  AND (sqlc.narg(from_date) IS NULL OR ranked_on >= sqlc.narg(from_date))  -- nullable param
LIMIT sqlc.arg(limit) OFFSET sqlc.arg(offset);
```

### Pitfalls (each one bit us)

1. **`sqlc.slice` must be the last parameter in the query.** sqlc numbers named params `?1..?N`, then expands the slice to bare `?,?,?`; SQLite numbers bare `?` after the highest seen. A slice placed first shifts every later param onto the wrong value.
2. **Params inside `ORDER BY` are not rewritten** (the literal `sqlc.arg(x)` stays in the SQL and fails at runtime). Pass them through a one-row subquery:
   ```sql
   FROM titles t, (SELECT CAST(sqlc.arg(sort) AS TEXT) AS sort_key) o
   ORDER BY CASE WHEN o.sort_key = 'name_asc' THEN t.name END COLLATE NOCASE ASC, ..., t.id DESC
   ```
3. **Scalar subqueries are typed non-null** even when they return NULL → scan error. `COALESCE((SELECT ...), 0)` with a sentinel and convert back in the repository (see `rankPtr`).
4. **Aggregates over dates come back as `string`/`interface{}`**. `CAST(COALESCE(MAX(x), '') AS TEXT)` and parse in Go (`parseStoredDate`).
5. CHAR(n) columns (e.g. `country`) generate `interface{}`; convert with `toString`.

### Dates in SQLite

- `time.Time` params are stored as text like `2026-04-28 00:00:00 +0000 UTC` (always pass UTC midnight for dates). Range filters work by string comparison as long as both sides use that format — always bind `time.Time`, never hand-format strings.
- `DEFAULT CURRENT_TIMESTAMP` columns are `YYYY-MM-DD HH:MM:SS` (UTC). For "last N hours" windows use `datetime('now', CAST(sqlc.arg(since) AS TEXT))` with `'-24 hours'`, not a Go timestamp.

## Repository wrappers

- Convert `sql.Null*` ↔ pointers (`nullStringPtr`, `toNullString`, …); return domain structs, not sqlc rows, from anything the service layer uses widely.
- Return `nil, nil` for "not found" only where callers expect optional data; otherwise propagate `sql.ErrNoRows` so handlers map it to 404.
- Multi-statement writes go in a transaction (`q.WithTx(tx)`), e.g. `RatingsRepository.SaveTitleRatings`.

## Verify

- `make sqlc && go build ./... && go test -count=1 ./internal/...`
- Query against a **copy** of real data (never `data/metareel.db` directly): `cp data/metareel.db $SCRATCH/x.db && sqlite3 $SCRATCH/x.db '...'`.
