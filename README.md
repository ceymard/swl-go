# swl-go

**swl** (Structured Workflow Language) is a command-line ETL tool: read data from files and databases, reshape it through a pipeline of transforms, and write it out elsewhere. 

```bash
make build
./swl users.json :: flatten :: out.db
./swl postgres://localhost/app :: coerce :: users.csv
```

Run `./swl` with no arguments to list handlers, file extensions, and URI protocols.

---

## How it works

### Pipeline model

A command is a sequence of **stages** separated by `::`:

```
sources [++ more-sources] [:: transforms] [:: sink]
```

Each stage reads or writes **collections**. A collection is a named group of **rows**

```
┌─────────┐    coll.Stream     ┌───────────┐    coll.Stream     ┌────────┐
│ Source  │ ─────────────────► │ Transform │ ─────────────────► │  Sink  │
│ pg-src  │  Collection{Name,  │  flatten  │                    │ csv-   │
│ json-src│   Rows: iter…}     │  coerce   │                    │ sink   │
└─────────┘                    └───────────┘                    └────────┘
```

If there is **no sink**, rows are pretty-printed to stderr via the debug sink (useful when exploring data on a TTY).

### Resolving handlers

The CLI resolves a token in this order:

1. **Explicit source** — prefix `+` forces source mode (`+pg` → `pg-src`, even after `::`)
2. **Alias** — `pg`, `sqlite`, `yaml`, `csv`, …
3. **File extension** — `.json`, `.csv`, `.db`, `.yml`, …
4. **URI protocol** — `postgres://`, `mysql://`, `sqlserver://`, …
5. **Inline JSON** — argument starting with `[` or `{`

Dual aliases (⇄) work as source before `::` and sink after `::`. Sink-only aliases (←) and source-only aliases (→) follow the same rules as swl2.

### Data flow and errors

- **Streaming** — rows are yielded through Go `iter.Seq2`; handlers should not load entire datasets unless schema inference requires it.
- **Errors** — returned as Go `error` values with stack context (`internal/errs`); they never appear as rows in the stream.
- **Logs** — progress and diagnostics go to **stderr** (`internal/msg`, `internal/progress`), not through the data stream.

### Progress and row counts

Row statistics are centralized in `internal/progress` so all handlers behave the same:

| Event | When | Verbosity |
|-------|------|-----------|
| Handler prefix | Every handler log line | ≥ 1 — green `handler »` (source) or red `handler «` (sink) |
| `collection emitted » N lines` | End of each source collection | ≥ 1 |
| `collection received « N lines` | End of each sink collection | ≥ 1 |
| `N rows handled so far - K Krows/secs` | While sink is consuming rows | ≥ 2, once per second |

Counts reflect **exact rows passed through the stream**, not `SELECT count(*)` on the database (which could include pre-existing rows or miss in-flight batches).

Default verbosity is **2** (progress). Use `-q` / `--quiet` for silence, `-v` / `-vv` for more detail, or `SWL_VERBOSE=N`.

### SSH tunnels

Database URIs support swl2-style SSH forwarding:

```
postgres://user:pass@dbhost:5432/mydb @@ jumpuser@jumphost:22
```

The segment after `@@` opens an SSH tunnel; the database host is reached via `localhost` on an ephemeral port.

---

## Global flags

| Flag | Description |
|------|-------------|
| `-h`, `--help` | Help |
| `-q`, `--quiet` | Suppress progress (verbosity 0) |
| `-v`, `-vv` | Increase verbosity |
| `-p` | Passthrough — tee rows to debug output during transforms |

Handler-specific flags are documented with `./swl help <handler>`.

---

## Transforms

| Handler | Description | Notable flags |
|---------|-------------|---------------|
| `flatten` | Nested objects → dotted keys (`a.b.c`) | |
| `unflatten` | Dotted keys → nested objects | `-n` drop empty paths |
| `coerce` | Infer and cast types (numbers, dates, bools) | `-o` only listed columns |
| `uncoerce` | Reverse coercion to strings | `-o/-e/-b/-t/-n` |

---

## File handlers

### JSON (`json`)

**Source** — reads `.json` files or inline JSON arrays/objects. JSON5 syntax is accepted on read. Streaming array parsing avoids loading huge files whole.

| Flag | Description |
|------|-------------|
| `-c`, `--collection` | Collection name (default: filename) |
| `-e`, `--encoding` | Character encoding |

**Sink** — writes JSON to a file, directory, or `%` path pattern.

| Flag | Description |
|------|-------------|
| `-o`, `--object` | One JSON object keyed by collection name instead of row arrays |

### CSV (`csv`)

**Source** — one or more CSV files; optional gzip.

| Flag | Description |
|------|-------------|
| `-d`, `--delimiter` | Field delimiter (default `,`) |
| `-u` | Parse numbers |
| `-s`, `--simplify-headers` | Normalize header names |
| `-h`, `--headers` | Explicit header list |
| `--gunzip` | Decompress `.gz` |

**Sink** — writes CSV per collection (file, directory, or `%` pattern).

| Flag | Description |
|------|-------------|
| `-d`, `--delimiter` | Delimiter (default `;`) |
| `-n`, `--no-headers` | Omit header row |

### YAML (`yaml`) — including data **generation**

YAML is both a static data format and a **programmable generator** — the main reason to use YAML over JSON in swl.

#### Source behavior

The file root may be:

1. **Mapping** — each key is a collection name; value is a sequence of row objects.
2. **Array** — single collection (name from `-c` or the filename stem).

```yaml
users:
  - name: alice
    role: admin
  - name: bob
    role: user
```

#### Reference data (`__refs__`)

A special collection `__refs__` holds shared reference rows. It is loaded into an internal accumulator passed to generators but **never emitted** as its own collection. Use it for templates, lookup tables, or constants referenced by generated rows.

```yaml
__refs__:
  - kind: template
    name: base

static:
  - id: 1
    label: fixed
```

#### JavaScript eval tags (`!!e`)

Scalars tagged with `!!e` (also `!e` or custom `!!…:e` suffixes) are evaluated as JavaScript via an embedded runtime (goja). The result becomes the YAML value — a literal, object, array, or **generator function**.

```yaml
computed:
  - !!e |
      ({ id: 1, ts: new Date().toISOString() })
```

Function declarations are supported:

```yaml
generated:
  - !!e |
      function(refs, push) {
        push({ id: 2, label: "from-js" });
      }
```

#### Generator functions

When `!!e` evaluates to a function, it is invoked as:

```javascript
function(refs, push) { … }
```

| Argument | Content |
|----------|---------|
| `refs` | Object whose keys are **already processed** collection names; values are arrays of row objects (includes `__refs__` under `refs.__refs__`) |
| `push(row)` | Append one row to the current collection |

Generators run **in document order**. Collections defined earlier are available in `refs` when later collections (or their generators) execute. This lets you build dependent datasets entirely in YAML — synthetic test data, denormalized exports, or parameterized fixtures — without an external script.

Example (`testdata/yaml/generators.yml`):

```yaml
__refs__:
  - kind: template
    name: base

static:
  - id: 1
    label: fixed

generated:
  - !!e |
      function(refs, push) {
        push({ id: 2, label: "from-js", ref: refs.__refs__[0].name });
        push({ id: 3, label: "also-js", __meta__: { source: "gen" } });
      }
```

Rows may carry `__meta__` for generator bookkeeping; it is **stripped on emit** and does not appear in downstream handlers.

#### Flags

| Flag | Description |
|------|-------------|
| `-c`, `--collection` | Name for array-root documents |
| `-e`, `--encoding` | Encoding (utf-8 only) |

#### Sink

Writes YAML with one collection per file (or `%` / directory layout). Each row is written as `- {json}\n` under the collection key (or a single file with `collection:\n` prefix).

### Parquet (`parquet`, `pqt`)

**Source** — columnar Parquet files; merges GCS-style shard names (`orders-0000001.pqt` → collection `orders`).

| Flag | Description |
|------|-------------|
| `-c` | Column selection |

**Sink** — infers schema from row values; supports file, directory, and `%` paths.

### Excel (`xlsx`, `xl`, `xls`)

**Source** — `.xlsx`, `.xlsm` (excelize), `.xlsb` (go-xlsb), `.ods`. One collection per sheet.

| Flag | Description |
|------|-------------|
| `-r` | Row range |
| `-e` | Empty cell handling |
| `-i` | Include hidden sheets |

**Sink** — one sheet per collection; can merge into an existing workbook.

---

## Database handlers

All SQL sources share a similar pattern:

- Connect via URI (plus optional SSH tunnel)
- Emit one collection per table (or per `-q` query)
- Parse JSON-typed columns to nested maps/slices where applicable

All SQL sinks share:

- Transaction wrapping with rollback on error
- Per-collection DDL options (`-a` auto-create, `-t` truncate, `-d` drop, `-u` upsert, …)
- Batched inserts for throughput

### PostgreSQL (`pg`, `postgres`)

**Source** `postgres://…`

| Flag | Description |
|------|-------------|
| `-s`, `--schema` | Schema (default `public`) |
| `table` / `-q` | Table list or custom query per collection |
| | Wildcard `.*` for all tables; FK-aware ordering with `-s` |

**Sink** — high-throughput `COPY` + `json_populate_record`; hstore columns; upsert CTE; optional trigger disable, index drop/recreate, sequence reset.

| Flag | Description |
|------|-------------|
| `-a` | Auto-create tables from row shape |
| `-t` / `-d` | Truncate / drop before load |
| `-u` / `-U` | Upsert / update |
| `--disable-triggers` | Disable triggers for load |
| `-i` | Drop indexes during load |

### MySQL (`mysql`, `my`)

**Source** / **Sink** — `mysql://…`. Batched INSERT (512 rows). JSON columns stored as JSON DDL type.

### Microsoft SQL Server (`mssql`, `ms`, `sqlserver`)

**Source** / **Sink** — `sqlserver://…` or ADO-style connection strings. Schema-qualified tables (`schema.table` collections). `SET IDENTITY_INSERT` when inserting explicit identity values. MERGE-based upsert.

### SQLite (`sqlite`)

**Source** / **Sink** — file path (`.db`, `.sqlite`). Declared column types drive JSON parsing on read. DDL inferred from first row on write.

| Flag | Description |
|------|-------------|
| `-q` | Custom query (source) |
| `-t` / `-d` / `-u` | Truncate / drop / upsert (sink) |

### DuckDB (`duckdb`)

**Source** / **Sink** — `.duckdb` / `.ddb` files. JSON batch insert via `from_json` on sink; `to_json` rows on source.

---

## Examples

```bash
# Explore JSON on stderr (no sink)
./swl users.json

# Flatten nested API payloads into a SQLite database
./swl api.json :: flatten :: app.db

# Chain sources, coerce types, load Postgres
./swl +users.csv ++ +orders.csv :: coerce :: postgres://localhost/app

# Generate test data from YAML and load CSV
./swl fixtures/generators.yml :: users.csv

# Custom query as a collection
./swl postgres://localhost/db users -q "SELECT * FROM users WHERE active" :: out.json

# Explicit source after :: (same as ++)
./swl :: +pg postgres://localhost/db :: flatten :: dump.json
```

---

## Building and testing

```bash
make build          # → ./swl (requires CGO for SQLite/DuckDB)
make test           # unit tests
make test-pg        # Postgres integration (Docker)
make test-mysql     # MySQL integration (Docker)
make test-mssql     # SQL Server integration (Docker)
```

**Module:** `github.com/ceymard/swl-go` · **Go:** 1.26+

---

## Architecture (for contributors)

```
cmd/swl/           CLI entry (Kong)
internal/
  coll/            Collection, Row, Stream
  stream/          Concat, MapRows, TeeRows, …
  runner/          Stage folding; wires progress tracking
  progress/        Row counts, periodic sink stats, colored handler logs
  pipeline/        Parse argv → []Stage
  handlers/        Source, Transform, Sink interfaces
  msg/             stderr logging
  style/           ANSI colors (swl2 palette)
handler/           One package per format/database
```

Handler registry and aliases live in `handler/registry.go` (ported from `swl2/scripts/swl.ts`).

For implementation status and porting notes, see [`IMPLEMENTATION.md`](IMPLEMENTATION.md).
