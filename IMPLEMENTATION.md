# swl-go implementation overview

**Living document.** Update this file whenever the port changes (new handlers, API shifts, completed phases). For checkpoint notes and deep dive history see [`PORT.md`](PORT.md). For original goals see [`plan.md`](plan.md).

*Last updated: 2026-06-30 — colored CLI output (debug rows, handler list).*

---

## Status at a glance

| Area | State |
|------|--------|
| Core runtime (coll, stream, runner) | ✅ Done |
| CLI (Kong + pipeline parse + empty handler list + **colors**) | ✅ Done |
| Transforms (flatten, unflatten, coerce, uncoerce) | ✅ Done |
| JSON source + sink | ✅ Done |
| CSV, SQLite, PG, mysql, duckdb, yaml, xlsx, parquet, fn | ⏳ Stubs (fail at run) |
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

**Sink driver:** `runner.ConsumeHooks` — first row passed to `Open`, not `Write` (swl2 parity).

---

## CLI

```bash
make build
./swl                          # lists handlers/extensions/protocols, exit 1
./swl users.json :: flatten    # source → transform → debug sink
./swl users.json :: out.json     # source → json sink
./swl -h                       # Kong help
```

Pipeline tokens: `++` chains sources, `::` separates source side from transforms/sink.

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
4. `handler.ParseOptions(id, target, tail)` — Participle grammars per handler

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
| `csv-src/sink` | — | stub | |
| `sqlite-src/sink` | — | stub | Next priority |
| others | — | stub | pg, mysql, duckdb, yaml, xlsx, parquet, fn |

Registry: `handler/registry.go` (aliases mirror `swl2/scripts/swl.ts`).

---

## Package map

```
cmd/swl/                 CLI entry (Kong)
internal/
  coll/                  Collection, Row, Stream
  stream/                Concat, MapRows, TeeRows, Of, Empty, CheckContext
  runner/                Run, ConsumeHooks
  handlers/              Source, Transform, Sink interfaces
  pipeline/              Parse, stageTarget, resolveHandler
  cli/                   ExpandFlags, BuildParser, ParseArgs, BaseOpts
  debug/                 Default stderr sink (colored when TTY)
  style/                 ANSI colors for CLI (NO_COLOR / non-TTY safe)
  errs/, msg/, schema/, stage/
handler/
  registry.go, register.go, stub.go, reg.go
  flatten/, coerce/, unflatten/, json/
test/swltest/            Integration helpers (not in prod binary)
testdata/json/           JSON fixture files
```

---

## Dependencies

| Package | Use |
|---------|-----|
| `github.com/alecthomas/kong` | Global CLI |
| `github.com/alecthomas/participle/v2` | Handler argv |
| `github.com/samber/oops` | Error stacks |
| `github.com/aeolun/json5` | JSON5 source read |
| `github.com/fatih/color` | Terminal colors (`internal/style`) |

---

## Tests

```bash
go test ./...
make test
```

| Location | Covers |
|----------|--------|
| `internal/stream`, `cli`, `pipeline`, `errs` | Unit |
| `handler/json`, `handler/registry` | JSON + aliases |
| `test/swltest` | Pipeline integration (mem source, flatten) |

---

## Next work

1. **SQLite** — `handler/sqlite/`, `ConsumeHooks`, participle table specs
2. **CSV** — `handler/csv/`
3. **sonic sink** — when compatible with Go 1.26
4. **SSH** — `internal/ssh/tunnel.go`
5. **Polish** — per-handler help, golden vs swl2

---

## Known gaps

- `BaseOpts` (`-p`, `-a`, `-v`) parsed but not fully merged into `runner.Config`
- Global `-p` passthrough only on transform path in runner
- Multi-collection single-file json sink (non-`-o`) emits concatenated arrays (swl2 parity)
- Collection name from path: basename without extension (e.g. `dir/foo.json` → `foo`)

---

## swl2 reference

TypeScript reference: [`swl2/`](swl2/) — do not port `optparse.ts` verbatim; use Participle grammars per handler.
