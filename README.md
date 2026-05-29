<p align="center">
  <img src="static/logo.png" alt="hopper" width="480">
</p>

<h1 align="center">hopper</h1>

<p align="center">
  <a href="https://github.com/daichirata/hopper/actions/workflows/test.yaml"><img src="https://github.com/daichirata/hopper/actions/workflows/test.yaml/badge.svg" alt="Test"></a>
  <a href="https://github.com/daichirata/hopper/releases/latest"><img src="https://img.shields.io/github/v/release/daichirata/hopper" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
</p>

`hopper` is a command-line tool to generate and load dummy data into Google Cloud Spanner.

It reads the schema directly from the target database, fills every column with a
type-appropriate random value by default, and lets you override individual columns
with [gofakeit](https://github.com/brianvoe/gofakeit) templates. Interleaved and
foreign-key relationships are handled automatically.

The examples below use the [Spanner sample schema](https://cloud.google.com/spanner/docs/schema-and-data-model) (`Singers` → `Albums` → `Songs`, interleaved).

## Installation

```
go install github.com/daichirata/hopper@latest
```

Or use the Docker image, published to `ghcr.io/daichirata/hopper` on each tagged release:

```
docker run --rm ghcr.io/daichirata/hopper run --help
```

## Quick start

```
# 1000 rows into Singers (primary key auto-generated, other columns randomized)
hopper run spanner://projects/p/instances/i/databases/d --table 'Singers=1000'
```

The database is given as a `spanner://` DSN:

```
spanner://projects/{projectId}/instances/{instanceId}/databases/{databaseName}?credentials=/path/to/keyfile.json
```

| Param          | Required | Description                                                              |
|----------------|----------|--------------------------------------------------------------------------|
| `projectId`    | true     | The Google Cloud Platform project id                                     |
| `instanceId`   | true     | The id of the instance running Spanner                                   |
| `databaseName` | true     | The name of the Spanner database                                         |
| `credentials`  | false    | Path to the keyfile. If omitted, the default application credentials are used. |

When `SPANNER_EMULATOR_HOST` is set, hopper talks to the emulator (no credentials needed).

## Usage

```
❯ bin/hopper help
hopper is a command-line tool to load dummy data into Google Cloud Spanner.

Usage:
  hopper [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  run         Generate and load dummy data into Spanner
  scaffold    Print a config template generated from the database schema
  version     Display version

Flags:
  -h, --help   help for hopper

Use "hopper [command] --help" for more information about a command.
```

### `hopper run DATABASE`

Generate dummy data and load it into Spanner.

| Flag | Description |
|------|-------------|
| `-c`, `--config FILE`         | path to a YAML config file |
| `--table TABLE=N`             | total rows per table (repeatable) |
| `--set TABLE.COL=TEMPLATE`    | gofakeit template per column (repeatable) |
| `--seed N`                    | random seed (`0` = time-based) |
| `--dry-run`                   | generate rows but do not insert (prints a few sample rows) |
| `--no-infer`                  | disable inferring a gofakeit function from column names |
| `--null-rate F`               | probability (0–1) of leaving a nullable column `NULL` |
| `--clear`                     | delete existing rows from each target table before loading |
| `--clear-batch-size N`        | max rows per Delete commit during `--clear` (default `100`; auto-halved on `too-many-mutations`) |
| `-v`, `--verbose`             | print extra runtime info on stderr (e.g. the seed used) |

### `hopper scaffold DATABASE`

Print a YAML config template generated from the database schema. Columns whose name matches a gofakeit function are pre-filled; the rest are commented out for you to fill in.

| Flag | Description |
|------|-------------|
| `--table T`                   | table to include (repeatable; default: all tables) |

### `hopper version`

Print the version.

## Tables and row counts

`--table TABLE=N` (repeatable) sets the **total** number of rows for a table:

```
--table 'Singers=10'
--table 'Albums=1000'
```

Parent/child relationships are read from the schema, so you only provide counts:

- **Interleaved** (`INTERLEAVE IN PARENT`) and **foreign-key** tables are generated
  parent-first. Interleaved children inherit the parent's primary key; FK columns
  reference a random row of the parent — so referential integrity always holds.
- Children are distributed across their parents **round-robin**. `Singers=10` +
  `Albums=1000` gives ~100 Albums per Singer; choose the totals to set the ratio
  (`Albums=300` → ~30 each).
- Naming only a child **auto-completes its parents** (one row each):
  `--table 'Albums=300'` creates 1 Singer with 300 Albums under it.

## Column values

Every column is filled automatically; use `--set` to override specific ones.

**Defaults** — when a column has no `--set`:

- **Primary keys and unique-index columns** get a collision-free unique value (UUID for STRING, sequential for INT64, …).
- **Other columns** are inferred from the column name when it matches a gofakeit
  function (`Email`, `FirstName`, `Phone`, …); otherwise a type-appropriate random
  value is used. Add `--no-infer` to disable name inference.
- **Commit-timestamp** columns (`OPTIONS (allow_commit_timestamp=true)`) are set to the pending commit timestamp.
- **Generated / stored** columns are skipped.
- `--null-rate` (0–1) randomly leaves nullable columns NULL.

**Overrides** — `--set TABLE.COLUMN=TEMPLATE`, a
[gofakeit](https://github.com/brianvoe/gofakeit#templates) template using `{{ }}`:

```
--set 'Albums.MarketingBudget={{ Number 0 1000000 }}'
--set 'Singers.FirstName={{ FirstName }}'
--set 'Singers.LastName={{ FirstName }}-{{ Index }}'
```

`TABLE` is matched by name, so the leaf table name is enough. Common building blocks:

| Template                                       | Result                                    |
|------------------------------------------------|-------------------------------------------|
| `{{ Number 0 100 }}`                           | random integer in a range                 |
| `{{ Float64 }}`                                | random float                              |
| `{{ LetterN 12 }}`                             | 12 random letters                         |
| `{{ Regex "[A-Z0-9]{8}" }}`                    | string matching a regex                   |
| `{{ UUID }}`                                   | a UUID                                    |
| `{{ RandomString (SliceString "a" "b" "c") }}` | pick one of the values                    |
| `{{ FirstName }}` `{{ Email }}` `{{ Phone }}`  | realistic fake data                       |
| `{{ Sentence 5 }}`                             | a 5-word sentence                         |
| `{{ Index }}`                                  | row sequence number (0-based)             |
| `{{ add Index 1 }}`                            | arithmetic: `add` `sub` `mul` `div` `mod` |
| `{{ Col "FirstName" }}`                        | value of another column in the same row   |
| `{{ Ref "<table>" "<column>" }}`               | random value picked from another generated table |
| `{{ RefDistinct "<table>" "<column>" "<scope>" }}` | same, but unique per scope + no self-reference |

`{{ Index }}`, the arithmetic helpers, and `{{ Col }}` are hopper additions; everything
else is a gofakeit function (any [gofakeit function](https://github.com/brianvoe/gofakeit#functions)
works). Templates can be combined (`{{ FirstName }}-{{ Index }}`), and the result is
converted to the column's type — use a numeric template for numeric columns. ARRAY
columns get a single templated element (or a few random ones by default).

`{{ Col "OtherColumn" }}` reads a column already generated for the same row, so you can
derive one value from another (`--set 'Singers.Nickname={{ Col "FirstName" }}-{{ Index }}'`).
Columns are generated in schema order, so reference only columns that come earlier.

`{{ Ref "<table>" "<column>" }}` picks a random row from a **previously generated** table and
returns the value of its `<column>`. Useful for filling logical references that the schema
doesn't express as `FOREIGN KEY` (so hopper can't auto-resolve them). For example, imagine a
`Reviews` table whose `ReviewerSingerId` column points at `Singers.SingerId` only logically:
`--set 'Reviews.ReviewerSingerId={{ Ref "Singers" "SingerId" }}'` keeps each row pointing at
a real Singer.

`{{ RefDistinct "<table>" "<column>" "<scope>" }}` adds two guarantees: (1) the picked value
is never equal to the current row's `<scope>` value (no self-reference), and (2) within rows
sharing the same `<scope>` value, picked values are not reused. If the pool is exhausted,
hopper fails with a clear error. For example, imagine a `Collaborations` table interleaved
in `Singers`, where each row records that one Singer (the parent, `SingerId`) collaborated
with another (`GuestSingerId`), with `(SingerId, GuestSingerId)` constrained `UNIQUE` and a
Singer not allowed to list themself as a collaborator. Then
`--set 'Collaborations.GuestSingerId={{ RefDistinct "Singers" "SingerId" "SingerId" }}'`
fills it correctly: the picked `SingerId` is compared against the current row's own
`SingerId` (no self-collaboration), and within the same parent Singer no `GuestSingerId` is
reused.

## Configuration file

For anything non-trivial, use a YAML file. Generate a starting point with
`hopper scaffold DATABASE [--table T]`: every column is listed — name-inferred ones
are pre-filled, the rest are commented out (`# Column:`) for you to fill in — while
primary keys and generated columns are skipped.

```
# scaffold every table in the schema
hopper scaffold spanner://projects/p/instances/i/databases/d > hopper.yaml

# scaffold a subset
hopper scaffold spanner://projects/p/instances/i/databases/d \
  --table Singers --table Albums > hopper.yaml
```

Edit the generated `hopper.yaml`, then pass it with `--config`:

```yaml
tables:
  Singers:
    rows: 10
    columns:
      FirstName: "{{ FirstName }}"
      LastName: "{{ LastName }}"
  Albums:
    rows: 1000
    columns:
      AlbumTitle: "{{ Sentence 3 }}"
      MarketingBudget: "{{ Number 0 1000000 }}"
```

```
hopper run spanner://projects/p/instances/i/databases/d --config hopper.yaml
```

`--config` can be combined with `--table` / `--set`, which override or extend it —
handy for bumping a count or tweaking one column without editing the file.

## Development

```
make build          # build ./bin/hopper
make test           # unit tests
make deps           # go get -t ./... && go mod tidy
```

End-to-end test against the Cloud Spanner emulator:

```
docker run -d -p 9010:9010 -p 9020:9020 gcr.io/cloud-spanner-emulator/emulator
SPANNER_EMULATOR_HOST=localhost:9010 go test -tags e2e ./internal/hopper
```

## License

MIT
