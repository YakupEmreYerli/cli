# AGENTS.md

Guidance for AI coding agents. There are two audiences:
- **Using** the `dna` CLI to manage domains: read the first section.
- **Working on** this repository: read the rest.

## Using dna (operating the API)

Run `dna agent guide` for the authoritative, version-matched guide, or read [`skills/dna/SKILL.md`](skills/dna/SKILL.md).

Non-negotiable rules:
- Prefer the test environment: `--env ote` (or `--ote`). Production (the default) moves real money and real domains.
- Use `-o json` and `--jq`. Errors are JSON on stderr with stable exit codes ([docs/errors.md](docs/errors.md)).
- **Never pass `--yes` on your own initiative.**
  - Billable or destructive commands exit with code 9 without it.
  - Run them with `--dry-run`, show the user the exact request, get explicit approval, then re-run with `--yes`.
  - Over MCP the same rule applies to `confirm=true`.
- Use `--read-only` while exploring.
- Do not retry a billable command after a network error or timeout (exit 8) before checking the result (`dna domain info <domain>`).

## Working on this repository

### Commands

```bash
make build        # bin/dna
make test         # go test ./...
make lint         # go vet + gofmt check
make docs         # regenerate docs/reference, docs/enums.md, skills/dna/SKILL.md
make check        # docs + lint + test, and fail if generated files changed
make spec         # download the latest OpenAPI document (prod + OTE)
```

Go is at least 1.26 (see `go.mod`). On Windows the same commands work with `go` directly, for example
`go run ./cmd/dna gen-docs --dir .` instead of `make docs`.

### Layout

See [docs/architecture.md](docs/architecture.md). In short:
- `internal/openapi/openapi.json` (the API spec) and `internal/ops/registry.go` (one entry per operation) are the sources of truth.
- `internal/cli` builds commands from them.
- `internal/mcp` builds MCP tools from them.
- `internal/dna` is the HTTP client: credentials, retries, pacing, errors and enum documentation.

### Rules

- **Generated files: do not edit by hand.**
  - `docs/reference/*`, `docs/enums.md` and `skills/dna/SKILL.md` come from `make docs`.
  - Regenerate them after changing the spec, the registry, enums or command help, and commit the result.
- **New API operation:**
  - add a registry entry (ID, method, path, summary, safety)
  - add an enum if the API added one
  - see "Adding or changing an operation" in the architecture doc
  - `TestRegistryCoversSpec` and `TestEveryIntegerEnumIsDocumented` fail until all of this is done.
- **Safety class** is a product decision:
  - `billable` for anything that charges the balance
  - `destructive` for deletes and hard-to-undo changes
  - `write` for free, reversible changes
  - `read` only for operations with no side effects, even when the HTTP method is POST or PUT
- **Exit codes and the error JSON shape are a public contract** ([docs/errors.md](docs/errors.md)). Do not change their meaning.
- **This repository is a CLI and MCP server, not a Go library.** Everything lives under `internal/`; do not add a public Go API.
- **Tests** never call the real API. Use `httptest` servers, as in `internal/cli/cli_test.go`.
- **Style:**
  - `gofmt`
  - English for code, comments and docs
  - user-facing strings say what happened and what to do next
- **Commits:** imperative subject line. Do not add `Co-Authored-By` trailers for AI tools.
