# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org).

## [Unreleased]

### Added

- `dna` CLI covering all 64 operations of the DomainNameAPI Public Gateway API (production and OTE):
  - account, domains, DNS zones, DNSSEC, child nameservers
  - contacts, SSL, hosting, products, orders
- Safety classes (`read`, `write`, `destructive`, `billable`), with confirmation, `--yes`, `--dry-run` and read-only mode.
- Output formats `json`, `jsonl`, `table` and `raw`, a built-in `--jq`, and `--all` pagination.
- Structured JSON errors with stable exit codes.
- `dna schema` and `dna enums` for machine-readable descriptions of operations, inputs and coded values.
- MCP server (`dna mcp serve`) with one tool per operation, safety annotations and a `confirm` protocol for billable and destructive tools.
  - `dna mcp config` prints client snippets.
- `dna agent guide` and a Claude Code skill (`dna agent skill --install`).
- Profiles, `dna auth login|status|logout`, `dna config` and `dna doctor`.
- Generated command reference, enum documentation and skill file.
