# swl-go implementation overview

**Living document.** Update this file whenever the port changes (new handlers, API shifts, completed phases). For checkpoint notes and deep dive history see [`PORT.md`](PORT.md). For original goals see [`plan.md`](plan.md).

*Last updated: 2026-06-30 — CSV source/sink implemented.*

---

## Status at a glance

| Area | State |
|------|--------|
| Core runtime (coll, stream, runner) | ✅ Done |
| CLI (Kong + pipeline parse + empty handler list + **colors**) | ✅ Done |
| Transforms (flatten, unflatten, coerce, uncoerce) | ✅ Done |
| JSON source + sink | ✅ Done |
| SQLite source + sink | ✅ Done |
| CSV source + sink | ✅ Done |
| PG, mysql, duckdb, yaml, xlsx, parquet, fn | ⏳ Stubs (fail at run) |
| SSH tunnels | ⏳ Not started |
| sonic JSON writer | ⏳ Deferred (Go 1.26 + sonic incompatibility; using `encoding/json`) |

**Module:** `github.com/ceymard/swl-go` · **Go:** 1.26.4 · **Binary:** `make build` → `./swl`

---

## Architecture

Single binary, in-process pipelines. No wire protocol (replaces swl2 v8 protocol).

```
argv → pipeline.Parse → []Stage → runner.Run → coll.Stream folded stage-by-stage
```

Data model:

```
coll.Stream     = iter.Seq2[Collection, error]
Collection.Rows = iter.Seq2[Row, error]     // Row = map[string]any
```

- **Errors:** Go `error` + `github.com/samber/oops` (`internal/errs`) — never in stream
- **Logs:** `internal/msg` → stderr
- **No terminal sink:** `internal/debug` prints rows to stderr

### Stage folding (`internal/runner`)

| Kind | Behavior |
|------|----------|
| Source | `stream.Concat` onto upstream |
| Transform | `stream.MapRows` / handler wrap |
| Sink | terminal consume |
| (none) | `debug.Sink` |

**Sink driver:** `handlers.ConsumeHooks` — first row passed to `Open`, not `Write` (swl2 parity).

---

## CLI

```bash
make build
./swl                          # lists handlers/extensions/protocols, exit 1
./swl users.json :: flatten    # source → transform → debug sink
./swl users.csv :: out.db       # csv → sqlite
./swl data.json :: out.csv      # json → csv
./swl -h                       # Kong help
```

Pipeline tokens: `::` separates sources from transforms/sink. Prefix a handler with `+` for an explicit source (`+pg` → pg-src); `:: +src` chains another source (same as legacy `++ src`). Dual aliases without `+` default to sink help; `+handler` always shows/resolves as source.

**Terminal colors** (`internal/style`, palette from swl2 `debug.ts`):

| Element | Color |
|---------|--------|
| Collection name | yellow |
| Row number | green |
| Field keys | cyan |
| Strings | green |
| Numbers | bright green |
| Booleans | magenta |
| null | red |
| Handler ⇄/←/→ | magenta / green / red |

Disabled when `NO_COLOR` is set or output is not a TTY (pipes, redirects).

**Parsing flow:**

1. Kong — global flags (`-v`, `--quiet`, `-h`)
2. `internal/pipeline` — split segments, resolve handler (alias / extension / protocol / inline JSON)
3. `stageTarget` — path vs flags (`data.json`, `json file.json`, `flatten -o x`)
4. `handler.ParseOptions(id, target, tail)` — `internal/optparse` (port of swl2 `optparse.ts`)

---

## Handlers

| ID | Package | Status | Notes |
|----|---------|--------|-------|
| `flatten` | `handler/flatten` | ✅ | Also contains `Unflatten` logic |
| `unflatten` | `handler/unflatten` | ✅ | `-n` drop empty |
| `coerce` | `handler/coerce` | ✅ | `-o` only columns |
| `uncoerce` | `handler/coerce` | ✅ | `-o/-e/-b/-t/-n` |
| `json-src` | `handler/json` | ✅ | json5 read, inline `[`/`{`, `-c`/`-e` |
| `json-sink` | `handler/json` | ✅ | file / dir / `%` paths, `-o` object mode |
| `sqlite-src` | `handler/sqlite` | ✅ | auto tables, `-q` query per table |
| `sqlite-sink` | `handler/sqlite` | ✅ | `-t/-d/-u`, transaction + rollback |
| `csv-src` | `handler/csv` | ✅ | multi-file, `-u` numbers, `-s` headers, gunzip |
| `csv-sink` | `handler/csv` | ✅ | `-d` (default `;`), dir / `%` / `.csv` paths |
| `pg-src` | `handler/pg` | ✅ | URI + SSH `@@`, `-s` schema FK order, `-q`, `.*` wildcard |
| `pg-sink` | `handler/pg` | ✅ | INSERT/upsert, `-t/-d/-u`, auto-create, transactions |
| others | — | stub | mysql, duckdb, yaml, xlsx, parquet, fn |

Registry: `handler/registry.go` (aliases mirror `swl2/scripts/swl.ts`).

---

## Package map

```
cmd/swl/                 CLI entry (Kong)
internal/
  coll/                  Collection, Row, Stream
  stream/                Concat, MapRows, TeeRows, Of, Empty, CheckContext
  runner/                Run
  handlers/              Source, Transform, Sink interfaces, ConsumeHooks
  pipeline/              Parse, stageTarget, resolveHandler
  optparse/              Port of swl2 optparse.ts (flag, param, arg, oneof, expand_flags)
  ssh/                   URI @@ SSH tunnel forwarding (swl2 uri_maybe_open_tunnel)
  cli/                   BaseOpts, ExpandFlags re-export
  debug/                 Default stderr sink (colored when TTY)
  style/                 ANSI colors for CLI (NO_COLOR / non-TTY safe)
  errs/, msg/, schema/, stage/
handler/
  registry.go, register.go, stub.go, reg.go
  flatten/, coerce/, unflatten/, json/, sqlite/, csv/, pg/
test/swltest/            Integration helpers (not in prod binary)
testdata/json/           JSON fixture files
testdata/csv/            CSV fixture files
```

---

## Dependencies

| Package | Use |
|---------|-----|
| `github.com/alecthomas/kong` | Global CLI |
| `github.com/samber/oops` | Error stacks |
| `github.com/aeolun/json5` | JSON5 source read |
| `github.com/fatih/color` | Terminal colors (`internal/style`) |
| `modernc.org/sqlite` | SQLite driver (pure Go) |
| `github.com/jackc/pgx/v5` | PostgreSQL driver |
| `golang.org/x/crypto/ssh` | SSH tunnel forwarding |
| `github.com/kevinburke/ssh_config` | `~/.ssh/config` lookup |
| `golang.org/x/text` | CSV header normalization (NFD) |

---

## Tests

```bash
go test ./...
make test
make test-pg   # handler/pg integration (Docker + testcontainers)
```

Set `SKIP_TESTCONTAINERS=1` to skip Docker-backed pg tests.

| Location | Covers |
|----------|--------|
| `internal/stream`, `cli`, `pipeline`, `errs`, `handlers` | Unit |
| `handler/json`, `handler/sqlite`, `handler/csv`, `handler/pg`, `handler/registry` | Handlers |
| `handler/pg` (integration) | testcontainers Postgres, FK schema order, sink round-trip |
| `testdata/pg/fixtures.sql` | `app` schema FK chain (accounts → users → posts) |
| `test/swltest` | Pipeline integration (mem source, flatten) |

---

## Next work

1. **PG sink polish** — COPY/json_populate_record path from swl2 (current: INSERT)
2. **mysql** — database handlers
3. **sonic sink** — when compatible with Go 1.26
4. **Polish** — per-handler help, golden vs swl2

---

## Known gaps

- `BaseOpts` (`-p`, `-a`, `-v`) parsed but not fully merged into `runner.Config`
- Global `-p` passthrough only on transform path in runner
- PG sink uses INSERT + ON CONFLICT (not swl2 COPY/json_populate_record yet)
- CSV sink default delimiter is `;` (swl2 parity); source default is `,`
- SSH tunnel uses `InsecureIgnoreHostKey` (match swl2/node-ssh; use known_hosts for production)
- Multi-collection single-file json sink (non-`-o`) emits concatenated arrays (swl2 parity)

---

## swl2 reference

TypeScript reference: [`swl2/`](swl2/) — handler argv uses a Go port of [`swl2/src/optparse.ts`](swl2/src/optparse.ts) in `internal/optparse`.
