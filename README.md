# hopper

```
       ┌──────────────┐
       │ ░ dummy rows │
       └───┐      ┌───┘
           └─┐  ┌─┘
             ▼  ▼
       Cloud Spanner
```

`hopper` is a command-line tool to generate and load dummy data into Google Cloud Spanner.

Named after [Minecraft's hopper](https://minecraft.wiki/w/Hopper) — the block that
funnels whatever drops in down into the container below. This one funnels dummy rows
into Spanner.

It reads the schema directly from the target database, fills every column with a
type-appropriate random value by default, and lets you override individual
columns with [gofakeit](https://github.com/brianvoe/gofakeit) templates.
Interleaved tables are handled automatically: child rows are distributed across
their parents and inherit the parent's primary key.

It is a companion to [hammer](https://github.com/daichirata/hammer) (schema management for Spanner).

The examples below use the [Spanner sample schema](https://cloud.google.com/spanner/docs/schema-and-data-model) (`Singers` → `Albums` → `Songs`, interleaved).

## Installation

```
go install github.com/daichirata/hopper@latest
```

## Quick start

```
# 1000 rows into Singers (primary key auto-generated, other columns randomized)
hopper run spanner://projects/p/instances/i/databases/d --table 'Singers=1000'
```

The database is addressed by a `spanner://` URI, the same form hammer uses:

```
spanner://projects/PROJECT/instances/INSTANCE/databases/DATABASE[?credentials=/path/to/key.json]
```

When `SPANNER_EMULATOR_HOST` is set, hopper talks to the emulator (no credentials needed).

## Specifying tables and counts

Each `--table TABLE=N` (repeatable) sets the **total** number of rows for a table:

```
--table 'Singers=10'
--table 'Albums=1000'
```

Parent/child relationships come from the schema's `INTERLEAVE` clauses. Child rows
are distributed across their parents round-robin and inherit the parent's primary
key, so the two flags above produce 1000 Albums spread over 10 Singers (~100 each).
To control "rows per parent", choose the totals accordingly:

```
--table 'Singers=10' --table 'Albums=1000'   # ~100 Albums per Singer
--table 'Singers=10' --table 'Albums=300'    # ~30 Albums per Singer
```

### Auto-completing parents

If you specify a child table on its own, hopper automatically creates the parent
chain (one row each) so the interleave constraints are satisfied:

```
hopper run spanner://... --table 'Albums=300'
# -> creates 1 Singers row and 300 Albums interleaved under it
```

## Column values

By default every column gets a type-appropriate random value, and primary key
columns get a collision-free unique value (UUID for STRING, sequential for
INT64, and so on). Generated/stored columns are skipped automatically.

Override a column with `--set TABLE.COLUMN=TEMPLATE`, where TEMPLATE is a
[gofakeit](https://github.com/brianvoe/gofakeit#templates) template using `{{ }}`:

```
--set 'Albums.MarketingBudget={{ Number 0 1000000 }}'
--set 'Singers.FirstName={{ FirstName }}'
--set 'Singers.LastName={{ FirstName }}-{{ Index }}'
```

`TABLE` is matched by name, so the leaf table name is enough (`Albums.MarketingBudget`),
whether or not the table is interleaved.

### Template building blocks

Any [gofakeit function](https://github.com/brianvoe/gofakeit#functions) works inside
`{{ }}`. Common ones:

| Template                                     | Result                                |
|----------------------------------------------|---------------------------------------|
| `{{ Number 0 100 }}`                         | random integer in a range             |
| `{{ Float64 }}`                              | random float                          |
| `{{ LetterN 12 }}`                           | 12 random letters                     |
| `{{ Regex "[A-Z0-9]{8}" }}`                  | string matching a regex               |
| `{{ UUID }}`                                 | a UUID                                |
| `{{ RandomString (SliceString "a" "b" "c") }}` | pick one of the given values        |
| `{{ FirstName }}` `{{ Email }}` `{{ Phone }}` `{{ Company }}` | realistic fake data |
| `{{ Sentence 5 }}`                           | a 5-word sentence                     |
| `{{ Index }}`                                | row sequence number (0-based)         |

`{{ Index }}` is added by hopper (the current row index); everything else is a
gofakeit function. Templates can be combined (`{{ FirstName }}-{{ Index }}`). The
rendered string is converted to the column's type, so use a numeric template
(`{{ Number ... }}`) for numeric columns.

### Inferring from column names

By default, any unset STRING/BYTES column whose name matches a gofakeit function
is filled with that function automatically: `Email` → emails, `FirstName` →
first names, and likewise `Phone`, `Company`, `City`, `Country`, … Names are
matched case-insensitively, ignoring underscores (`first_name` matches
`FirstName`). Explicit `--set` always wins, primary keys keep their unique
values, and anything unmatched — or whose value would not fit the column's
declared size — falls back to the type default. Pass `--no-infer` to disable it.

## Config file

For anything non-trivial, use a YAML file (`--config hopper.yaml`) — a flat list of
tables with row counts and column templates:

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
    # SingerId and other parent keys are inherited automatically.
```

## Flags

```
-c, --config string   path to a YAML config file
    --table           rows to generate as TABLE=N            (repeatable)
    --set             column template as TABLE.COLUMN=TEMPLATE (repeatable)
    --seed int        random seed (0 = time-based)
    --dry-run         generate rows but do not insert
    --no-infer        disable inferring a gofakeit function from column names
```

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
