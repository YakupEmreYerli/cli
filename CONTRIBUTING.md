# Contributing

Thanks for helping improve `dna`.

## Setup

- Go 1.26 or newer
- `make` (optional; every target is a plain `go` command, see the [Makefile](Makefile))

```bash
git clone https://github.com/domainreseller/cli && cd cli
make check          # regenerate, lint, test
make build && ./bin/dna --help
```

Tests never call the real API. To try changes against the API, use the OTE test environment:

```bash
dna auth login --env ote
./bin/dna --ote -v domain search example.com
```

## Where things live

Read [docs/architecture.md](docs/architecture.md) first. Most changes fall into one of these:

| change | files |
|---|---|
| command help, examples, flag names, safety class | `internal/ops/registry.go`, then `make docs` |
| validation, input handling | `internal/ops/ops.go`, `internal/ops/converters.go` |
| HTTP behaviour (retries, errors, pacing) | `internal/dna/client.go`, `internal/dna/errors.go` |
| enum meanings | `internal/dna/enums.go`, then `make docs` |
| API changes | `make spec`, then registry entries and enums (the tests tell you what is missing) |
| output formats | `internal/output` |
| MCP | `internal/mcp` |
| README animations and language buttons (`assets/`) | `scripts/readme-assets/build_assets.py` (layout) and `strings.py` (texts per language), then run it (needs `pip install fonttools`) |
| README text | `README.md` first, then the same change in every `README-XX.md` translation |

## Pull requests

- Keep generated files in sync. CI runs `make check` and fails on a diff.
- Add or update tests for behaviour changes.
- Exit codes, the error JSON shape and flag names are a public interface. Discuss breaking changes in an issue first.
- Add a line to `CHANGELOG.md` under "Unreleased".

## Releasing

Push a tag like `v0.2.0`. The release workflow runs GoReleaser:
- builds binaries for Linux, macOS and Windows (amd64/arm64)
- publishes archives and `checksums.txt`
- writes the release notes

The install scripts download from the latest release.
