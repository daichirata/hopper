# hopper

`hopper` is a command-line tool to generate and load dummy data into Google Cloud Spanner.

It reads the schema directly from the target database, fills every column with a
type-appropriate random value by default, and lets you override individual
columns with simple rules. Interleaved tables are handled automatically: child
rows inherit their parent's primary key, and you can control how many child rows
to generate per parent.

It is a companion to [hammer](https://github.com/daichirata/hammer) (schema management for Spanner).

## Installation

```
go install github.com/daichirata/hopper@latest
```

## Quick start

```
# 1000 rows into Users (primary key auto-generated, other columns randomized)
hopper run spanner://projects/p/instances/i/databases/d --table 'Users=1000'
```

The database is addressed by a `spanner://` URI, the same form hammer uses:

```
spanner://projects/PROJECT/instances/INSTANCE/databases/DATABASE[?credentials=/path/to/key.json]
```

When `SPANNER_EMULATOR_HOST` is set, hopper talks to the emulator (no credentials needed).

## Specifying tables and counts

Tables are given with `--table PATH=N` (repeatable). The dotted path expresses
the interleave hierarchy:

```
--table 'Users=1000'                       # root: 1000 rows in total
--table 'Users.UserAvatars=100'            # child: 100 rows per parent User
--table 'Users.UserAvatars.AvatarTags=5'   # grandchild: 5 rows per parent UserAvatar
```

- A path with **no dot** sets the **total** number of rows.
- A path **with dots** sets the number of rows **per parent**.

### Auto-completing parents

If you specify a child table on its own, hopper automatically creates the parent
chain (one row each) so the interleave constraints are satisfied:

```
hopper run spanner://... --table 'UserAvatars=300'
# -> creates 1 Users row and 300 UserAvatars interleaved under it
```

## Column rules

By default every column gets a type-appropriate random value, and primary key
columns get a collision-free unique value (UUID for STRING, sequential for
INT64, and so on). Override a column with `--set PATH.Column=RULE`:

```
--set 'Users.ShardId=range:0-10'                     # random integer in [0, 10]
--set 'Users.Name=template:{ .Random }-{ .Index }'   # text/template with { } delimiters
```

Templates expose:

| Variable      | Meaning                              |
|---------------|--------------------------------------|
| `{ .Index }`  | row sequence number (0-based)        |
| `{ .Random }` | a fresh random token per reference   |

Per-column rule precedence: `template` > `range` > primary-key auto-numbering > type default.

## Config file

For anything non-trivial, use a YAML file (`--config hopper.yaml`). It maps 1:1
to the CLI model:

```yaml
tables:
  Users:
    rows: 1000
    columns:
      ShardId: { range: [0, 10] }
      Name:    { template: "{ .Random }-{ .Index }" }
    children:
      UserAvatars:
        rows_per_parent: 100
        columns:
          Url: { template: "https://example.com/{ .Random }.png" }
        # UserId and other parent keys are inherited automatically.
```

## Flags

```
-c, --config string   path to YAML config file
    --table           table rows as PATH=N            (repeatable)
    --set             column rule as PATH.Column=RULE (repeatable)
    --seed int        random seed (0 = time-based)
    --dry-run         generate rows but do not insert
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
