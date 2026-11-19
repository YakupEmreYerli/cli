# Architecture

`dna` is **spec-driven**. The API's OpenAPI document is embedded in the binary. On top of it sits a small hand-written
registry that adds what the spec lacks: names, documentation, safety classes and CLI ergonomics. Everything
user-facing is generated from these two sources at runtime or by `make`.

```
internal/openapi/openapi.json   (API spec, embedded)
        │
        ▼
internal/ops   (registry: 1 entry per operation — ID, summary, safety, positional args, flag tweaks, examples)
        │
        ├── internal/cli   ── cobra commands + flags, validation, confirmation, dry-run, output
        ├── internal/mcp   ── MCP tools (JSON Schema + annotations), confirm protocol
        ├── dna schema / dna agent guide / skills/dna/SKILL.md
        └── docs/reference/*.md, docs/enums.md  (make docs)

internal/dna   (HTTP client: credentials, retries, pacing, quota, error normalization, enum documentation)
```

## Packages

| package | role |
|---|---|
| `cmd/dna` | `main`; build metadata via `-ldflags` |
| `internal/dna` | HTTP client. `client.go` (transport, retries, pacing, quota), `errors.go` (error normalization), `enums.go` (meaning of coded values) |
| `internal/openapi` | embedded spec, parser (keeps property order), JSON Schema conversion, example generation |
| `internal/ops` | the operation registry (`registry.go`), input model and validation (`ops.go`), converters for convenience flags, MCP input schemas |
| `internal/cli` | the cobra command tree built from the registry, plus `auth`, `config`, `api`, `schema`, `enums`, `agent`, `mcp`, `doctor` |
| `internal/mcp` | stdio JSON-RPC MCP server |
| `internal/output` | json / jsonl / table / raw rendering and `--jq` (gojq) |
| `internal/config` | profiles, config file, precedence resolution |

## The input model

Every operation has one canonical JSON input:
- path and query parameters, by camelCase name (`domainName`, `skipCount`)
- plus the request body's top-level properties (`nameServers`, `zoneStruct`)

This is the shape of `--body` and of the MCP tool arguments, and it is what `dna schema` describes.

`Operation.BuildRequest(input)` does the following:
1. Runs the operation's `Prepare` hook. Example: bulk search turns a list of names into the array body.
2. Validates and normalizes the input against the spec: required fields, types, ranges, lengths, enums and unknown keys.
3. Splits the input into path, query and body.

CLI flags are a flattened view of the same input:
- `Flatten` maps nested objects to top-level flags. `zoneStruct.type` becomes `--type`.
- `Converters` build arrays of objects from simple strings, such as `--handle`, `--ip` and `--ds`.

## Adding or changing an operation

1. Update the spec: `make spec`. This downloads both environments and warns if they differ.
2. Run `go test ./...`. `TestRegistryCoversSpec` fails for any operation without a registry entry.
3. Add the entry to `internal/ops/registry.go`:
   - ID, method, path, summary and safety are required.
   - Add positional `Args`, `Fields` overrides, `Examples` and table `Columns` where useful.
4. Add an enum to `internal/dna/enums.go` if the API added one. `TestEveryIntegerEnumIsDocumented` fails until you do.
5. `make docs` regenerates `docs/reference`, `docs/enums.md` and `skills/dna/SKILL.md`.

The tests also check that:
- every summary exists and every tool name is unique
- query parameters and body fields do not collide
- flags are unique and every command's `--help` renders
- every operation's `dna schema` output is valid

## Design decisions

- **Raw passthrough output.** The CLI prints the API response unchanged (JSON). Tables are only a view of it. Scripts and agents see exactly what the API returns, and nothing is lost to model mapping.
- **Safety from the registry, not the HTTP method.** `POST /domains/search` is a read; `PUT /ssls/check-status` is a read; `POST /domains/renew` is billable.
- **Retries only where safe.** 429s are rejected by the gateway before the backend runs, so they are always retried. Network errors are retried only for reads.
- **No prompts without a terminal.** Automation fails fast with exit code 9 instead of hanging.
- **CLI, not a library.** All code is under `internal/`. Responses are passed through as JSON, so there are no hand-maintained Go models to keep in sync with the API.
- **Few dependencies.** cobra/pflag, gojq and x/term; the HTTP client uses only the standard library.
