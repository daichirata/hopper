# Recipes

Task-oriented recipes for loading dummy data with `hopper`, organized by the schema
patterns that come up most often. The [README](../README.md) is the reference (flags,
templates, semantics); this is the cookbook.

## Sample schema

The examples below use this schema: `friendships` is interleaved in `users`, and a separate
`leaderboard_entries` table (a sharded leaderboard) is also interleaved in `users`.

```sql
CREATE TABLE users (
  user_id    STRING(36) NOT NULL,
  name       STRING(MAX) NOT NULL,
  email      STRING(MAX) NOT NULL,
  status     STRING(MAX) NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
) PRIMARY KEY (user_id);

CREATE TABLE friendships (
  user_id        STRING(36) NOT NULL,
  friend_user_id STRING(36) NOT NULL,
  created_at     TIMESTAMP NOT NULL,
  updated_at     TIMESTAMP NOT NULL,
) PRIMARY KEY (user_id, friend_user_id),
  INTERLEAVE IN PARENT users ON DELETE CASCADE;

CREATE TABLE leaderboard_entries (
  user_id        STRING(36) NOT NULL,
  leaderboard_id STRING(MAX) NOT NULL,
  score          INT64 NOT NULL DEFAULT (0),
  shard_key      INT64 NOT NULL DEFAULT (0),
  created_at     TIMESTAMP NOT NULL,
  updated_at     TIMESTAMP NOT NULL,
) PRIMARY KEY (user_id, leaderboard_id),
  INTERLEAVE IN PARENT users ON DELETE CASCADE;
CREATE INDEX idx_leaderboard_shard_key_leaderboard_id_score
  ON leaderboard_entries (shard_key, leaderboard_id, score DESC);
```

`spanner://...` below is shorthand for the full DSN
(`spanner://projects/p/instances/i/databases/d`).

## Load a single table

The simplest case — 100 rows into `users`:

```
$ hopper run spanner://... --table 'users=100'
users  100 rows
```

Every column is filled by type and name. Columns whose name matches a gofakeit function
(`name`, `email`, …) get realistic values; the rest get a type-appropriate random value.

## Load parent and child together

Pass `users` and `friendships` together and hopper builds the parent first, then the child.
Because `friendships` is interleaved in `users`, `friendships.user_id` inherits the parent's
value automatically.

```
$ hopper run spanner://... \
    --table 'users=100' \
    --table 'friendships=500'
users        100 rows
friendships  500 rows
```

`friendships=500` is the **total**; the 500 rows are distributed round-robin across the 100
users, so each user ends up with about 5 friendships.

## Load only the child and auto-create the parents

Name only the child and the parent `users` is detected from the schema and created with one
row:

```
$ hopper run spanner://... --table 'friendships=100'
users        1 rows
friendships  100 rows
```

This works through multiple interleave levels — name the leaf table and every ancestor is
completed with one row each. Tables referenced by `FOREIGN KEY` are completed the same way.

## Fill a logical reference with values that actually exist

In `friendships`, `user_id` is the interleave parent key (so the parent's `users.user_id` is
inherited), but `friend_user_id` is just a `STRING(36)` in the schema — by default it gets a
random UUID that points at no real user.

When you want it to reference a real user (to verify a JOIN, say), use the `RefDistinct`
template:

```
$ hopper run spanner://... \
    --table 'users=100' \
    --table 'friendships=1000' \
    --set 'friendships.friend_user_id={{ RefDistinct "users" "user_id" "user_id" }}'
```

`RefDistinct "users" "user_id" "user_id"` picks a random value from the `user_id` column of
the `users` rows generated in this run, with two guarantees:

1. it never equals the current row's own `user_id` (no self-friendship);
2. within rows that share the same `user_id`, the picked `friend_user_id` is not reused (no
   duplicate friend for the same user).

The arguments are, in order, the *referenced table*, the *referenced column*, and the *scope
column on the current row*. With `users=100, friendships=1000` each user gets 10 friendships,
each chosen without repeats from the other 99 users. If the pool runs out, hopper stops with
`RefDistinct(...): exhausted available values for ...`.

For a looser logical reference (one user writing many posts — duplicates and self-reference
are fine), use `Ref`:

```
$ hopper run spanner://... \
    --table 'users=100' \
    --table 'posts=1000' \
    --set 'posts.author_id={{ Ref "users" "user_id" }}'
```

`Ref` simply returns a random value of the given column from another already-generated table,
with no constraints — pick `Ref` or `RefDistinct` per use case.

## Append without overwriting existing rows

Without `--clear`, hopper inserts with `spanner.Insert`, so a primary-key collision fails
loudly with `AlreadyExists` — this is intentional, to prevent silent overwrites.

Values are random (and reproducible with `--seed`), so re-running usually just adds more
rows. If you need sequential primary keys, give each batch a distinct prefix so the runs
don't collide:

```
# first run
$ hopper run spanner://... \
    --table 'users=100' \
    --set 'users.user_id=batch-a-{{ add Index 1 }}'

# second run
$ hopper run spanner://... \
    --table 'users=100' \
    --set 'users.user_id=batch-b-{{ add Index 1 }}'
```

## Enum-like columns

To pick `active` / `suspended` / `deleted` at random for `status`:

```
$ hopper run spanner://... \
    --table 'users=100' \
    --set 'users.status={{ RandomString (SliceString "active" "suspended" "deleted") }}'
```

`SliceString` builds a string slice and `RandomString` returns one element at random. Most
enum-like columns are covered by this pattern.

## Deterministic (round-robin) values

`RandomString` is random; when you want a column to cycle through values **deterministically**
by row, use the `index` built-in with `mod Index`:

```
$ hopper run spanner://... \
    --table 'users=99' \
    --set 'users.status={{ index (SliceString "active" "suspended" "deleted") (mod Index 3) }}'
```

Row 0 → `active`, row 1 → `suspended`, row 2 → `deleted`, row 3 → `active`, … — an exact,
even split rather than a random one.

## Skewed distributions

`Number 0 3` and `RandomString (SliceString ...)` give a uniform distribution. To approximate
a skew — say `active` 90%, `suspended` 5%, `deleted` 5% — repeat values in the slice, since
hopper has no built-in weighting:

```
--set 'users.status={{ RandomString (SliceString "active" "active" "active" "active" "active" "active" "active" "active" "active" "suspended") }}'
```

Nine `active` to one `suspended` gives roughly 9:1. Not elegant, but easy. The same trick
skews `shard_key` toward one hot value, or you can narrow a `Number` range to bias it.

## Nullable columns and NULL_FILTERED indexes

Suppose notifications track when each was read, with a `NULL_FILTERED` index so unread
(`read_at IS NULL`) rows are excluded:

```sql
CREATE TABLE notifications (
  user_id         STRING(36) NOT NULL,
  notification_id STRING(36) NOT NULL,
  category        STRING(MAX) NOT NULL,
  read_at         TIMESTAMP,
) PRIMARY KEY (user_id, notification_id),
  INTERLEAVE IN PARENT users ON DELETE CASCADE;
CREATE NULL_FILTERED INDEX idx_notifications_user_category_read
  ON notifications (user_id, category, read_at DESC);
```

To verify the index excludes unread rows, you want a mix of NULL and non-NULL `read_at`. A
per-column null rate gives a probabilistic split:

```
$ hopper run spanner://... \
    --table 'users=100' \
    --table 'notifications=10000' \
    --set 'notifications.category={{ RandomString (SliceString "comment" "like" "follow" "mention") }}' \
    --null-rate 'notifications.read_at=0.5'
```

About half the rows are unread (`read_at = NULL`) and excluded from the index. The per-column
rate overrides the global `--null-rate`, so other nullable columns are unaffected.

For an **exact, deterministic** split instead of a probabilistic one, use `{{ Null }}` in a
conditional — e.g. every third notification unread:

```
--set 'notifications.read_at={{ if eq (mod Index 3) 0 }}{{ Null }}{{ else }}2024-01-01T00:00:00Z{{ end }}'
```

(For random non-NULL timestamps, prefer the `--null-rate` form above; the literal here keeps
the example simple.)

## Build data for a query-optimization comparison

A concrete end-to-end example. The sharded leaderboard has an index on
`(shard_key, leaderboard_id, score)`, and you fetch the top 10 like this:

```sql
SELECT *
FROM leaderboard_entries@{FORCE_INDEX=idx_leaderboard_shard_key_leaderboard_id_score}
WHERE shard_key BETWEEN 0 AND 3
  AND leaderboard_id = "1"
ORDER BY score DESC
LIMIT 10
```

A `shard_key BETWEEN min AND max` range scans the whole index range even with the index in
place. Rewriting it to `UNNEST` the shards and `CROSS JOIN` lets each shard take only `LIMIT`
rows:

```sql
SELECT *
FROM UNNEST([0, 1, 2, 3]) AS shard
CROSS JOIN leaderboard_entries
WHERE leaderboard_id = "1"
  AND shard_key = shard
ORDER BY score DESC
LIMIT 10
```

To compare the two plans you need data where `shard_key` is spread evenly over 0..3,
`leaderboard_id` is fixed, and `score` is distributed over 1..N:

```
$ hopper run spanner://... \
    --table 'users=10000' \
    --table 'leaderboard_entries=10000' \
    --set 'leaderboard_entries.leaderboard_id=1' \
    --set 'leaderboard_entries.shard_key={{ Number 0 3 }}' \
    --set 'leaderboard_entries.score={{ add Index 1 }}' \
    --clear
```

- `users=10000` with `leaderboard_entries=10000` gives one entry per user.
- `leaderboard_id=1` is a literal, not a template — a `--set` value with no `{{ }}` is used
  verbatim.
- `shard_key={{ Number 0 3 }}` spreads rows evenly over 0..3.
- `score={{ add Index 1 }}` assigns 1..10000, so `ORDER BY score DESC` has something to sort.
- `--clear` empties the target tables before loading.

Load this and compare the two plans (and their scanned-row counts) in Cloud Spanner Studio.

## Consolidate into YAML

Once the `--set` flags pile up, the CLI gets unwieldy. Scaffold a template and keep the
configuration in a YAML file instead:

```
$ hopper scaffold spanner://... > hopper.yaml
$ hopper run spanner://... --config hopper.yaml
```

See the [README](../README.md#configuration-file) for the file format. A checked-in YAML also
makes verification data reproducible from CI — a `go test` or `make` target can rebuild the
same dataset with one command.
