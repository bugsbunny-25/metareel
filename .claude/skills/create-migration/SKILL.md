---
name: create-migration
description: Scaffold a new goose SQL migration for metareel and show next steps
disable-model-invocation: true
---

To create a new migration, run:

```
make migrate-new MIGRATION_NAME=<descriptive_name>
```

Use snake_case for the name (e.g. `add_users_table`, `add_index_on_media_title`).

This generates two timestamped files in `db/migrations/`:
- `<timestamp>_<name>.sql` — add your `-- +goose Up` and `-- +goose Down` SQL blocks here

After editing the migration SQL:
1. Apply it: `make migrate-up`
2. If the migration changes table schema, regenerate Go code: `make sqlc`
3. Verify: `make migrate-status`

To roll back the last migration: `make migrate-down`
