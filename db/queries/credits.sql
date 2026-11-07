-- TMDB cast and crew (migration 000025).

-- name: ListCreditTargets :many
-- TMDB titles with when their credits were last fetched ('' = never),
-- never fetched first, then oldest.
SELECT tmdb_kind, tmdb_id, CAST(COALESCE(credits_fetched_at, '') AS TEXT) AS credits_fetched_at
FROM tmdb_titles
ORDER BY credits_fetched_at IS NOT NULL, credits_fetched_at, tmdb_kind, tmdb_id;

-- name: UpsertPerson :exec
INSERT INTO tmdb_people (person_id, name, profile_path, known_for, updated_at)
VALUES (sqlc.arg(person_id), sqlc.arg(name), sqlc.narg(profile_path), sqlc.narg(known_for), CURRENT_TIMESTAMP)
ON CONFLICT(person_id) DO UPDATE SET
    name = excluded.name, profile_path = excluded.profile_path, known_for = excluded.known_for, updated_at = CURRENT_TIMESTAMP;

-- name: DeleteTitleCredits :exec
DELETE FROM tmdb_credits WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: InsertCredit :exec
INSERT INTO tmdb_credits (tmdb_kind, tmdb_id, person_id, credit_type, job, department, character, credit_order, episodes)
VALUES (sqlc.arg(tmdb_kind), sqlc.arg(tmdb_id), sqlc.arg(person_id), sqlc.arg(credit_type), sqlc.arg(job), sqlc.arg(department),
        sqlc.narg(character), sqlc.arg(credit_order), sqlc.narg(episodes))
ON CONFLICT(tmdb_kind, tmdb_id, credit_type, job, person_id) DO NOTHING;

-- name: SetCreditsFetched :exec
UPDATE tmdb_titles SET credits_fetched_at = CURRENT_TIMESTAMP
WHERE tmdb_kind = sqlc.arg(tmdb_kind) AND tmdb_id = sqlc.arg(tmdb_id);

-- name: ListTitleCredits :many
SELECT c.person_id, p.name, CAST(COALESCE(p.profile_path, '') AS TEXT) AS profile_path, c.credit_type, c.job, c.department,
       CAST(COALESCE(c.character, '') AS TEXT) AS character, c.credit_order, c.episodes
FROM tmdb_credits c
JOIN tmdb_people p ON p.person_id = c.person_id
WHERE c.tmdb_kind = sqlc.arg(tmdb_kind) AND c.tmdb_id = sqlc.arg(tmdb_id)
ORDER BY c.credit_type, c.credit_order, c.job, c.person_id;

-- name: GetPerson :one
SELECT person_id, name, CAST(COALESCE(profile_path, '') AS TEXT) AS profile_path, CAST(COALESCE(known_for, '') AS TEXT) AS known_for
FROM tmdb_people WHERE person_id = sqlc.arg(person_id);

-- name: ListPersonCredits :many
-- A person's credits on the titles metareel knows, with the titles.
SELECT c.tmdb_kind, c.tmdb_id, c.credit_type, c.job, c.department, CAST(COALESCE(c.character, '') AS TEXT) AS character,
       c.credit_order, c.episodes, CAST(COALESCE(m.title, '') AS TEXT) AS title,
       CAST(COALESCE(m.release_date, '') AS TEXT) AS release_date, CAST(COALESCE(m.poster_path, '') AS TEXT) AS poster_path
FROM tmdb_credits c
JOIN tmdb_titles m ON m.tmdb_kind = c.tmdb_kind AND m.tmdb_id = c.tmdb_id
WHERE c.person_id = sqlc.arg(person_id)
ORDER BY m.release_date DESC, c.tmdb_kind, c.tmdb_id, c.credit_type, c.job;

-- name: ListPersonChartStats :many
-- FlixPatrol chart stats of each title a person has a credit on.
SELECT CAST(CASE t.kind WHEN 'tv_show' THEN 'tv' ELSE 'movie' END AS TEXT) AS tmdb_kind, CAST(t.tmdb_id AS TEXT) AS tmdb_id,
       COUNT(DISTINCT r.ranked_on) AS days, CAST(SUM(11 - r.rank) AS INTEGER) AS points, CAST(MIN(r.rank) AS INTEGER) AS best_rank,
       COUNT(DISTINCT r.country) AS countries,
       CAST(MIN(r.ranked_on) AS TEXT) AS first_charted, CAST(MAX(r.ranked_on) AS TEXT) AS last_charted
FROM (SELECT DISTINCT tmdb_kind, tmdb_id FROM tmdb_credits WHERE person_id = sqlc.arg(person_id)) c
JOIN titles t ON t.tmdb_id = c.tmdb_id AND t.kind = CASE c.tmdb_kind WHEN 'tv' THEN 'tv_show' ELSE 'movie' END
JOIN rankings r ON r.title_id = t.id
GROUP BY 1, 2;

-- name: ListCreditsOfType :many
-- Every credit of one type (cast | crew) with the person, for people
-- analytics.
SELECT c.tmdb_kind, c.tmdb_id, c.person_id, p.name, CAST(COALESCE(p.profile_path, '') AS TEXT) AS profile_path,
       c.job, CAST(COALESCE(c.character, '') AS TEXT) AS character, c.credit_order
FROM tmdb_credits c
JOIN tmdb_people p ON p.person_id = c.person_id
WHERE c.credit_type = sqlc.arg(credit_type);
