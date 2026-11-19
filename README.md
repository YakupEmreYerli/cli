<div align="center">

<a href="README.md">EN</a> | <a href="README.tr.md">TR</a>

# dna — DomainNameAPI CLI

**Domains, DNS, DNSSEC, SSL and hosting from your terminal, your scripts and your AI agents.**

[![CI](https://github.com/domainreseller/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/domainreseller/cli/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

</div>

`dna` is the official command line client for the [DomainNameAPI](https://www.domainnameapi.com) reseller REST API
(`api.domainresellerapi.com`, and the OTE test environment `ote.domainresellerapi.com`). It covers **all 64 API
operations** and has a built-in **MCP server**, so AI agents such as Claude Code, Claude Desktop, Cursor and Codex can
drive it as safely as people do.

```console
$ dna domain search example.com
$ dna domain list --all --jq '.items[] | select(.remainingDay < 30) | .domainName'
$ dna dns add example.com --name www --type A --content 192.0.2.10
$ dna domain renew example.com --period 1 --dry-run
```

## Why dna

**Built for AI agents**
- JSON output when piped. Errors are JSON on stderr, with [stable exit codes](docs/errors.md).
- `dna schema <op>` prints every input as JSON Schema with an example.
- `dna mcp serve` exposes every operation as an MCP tool, with safety annotations.
- Billable and destructive operations refuse to run without `--yes`, or `confirm=true` over MCP.
- `--dry-run` shows the exact HTTP request, `--read-only` blocks every change, and `dna agent guide` teaches an agent the whole CLI in one call.

**Built for developers**
- Tables on a terminal, plus `--jq`, `-o jsonl`, `--all` pagination, `--body @file.json` and stdin.
- Profiles for production and OTE, and shell completion.
- Readable errors with hints and a correlation id.
- Automatic retries on rate limits, and client-side pacing under the gateway limits.

**Always complete**
- Commands, flags, MCP tools and the [reference docs](docs/reference/README.md) are generated from the API's OpenAPI document.
- A test fails if any API operation is missing.

## Install

**Go 1.26+:**

```bash
go install github.com/domainreseller/cli/cmd/dna@latest
```

**Prebuilt binaries** for Linux, macOS and Windows (amd64/arm64) are on the [releases page](https://github.com/domainreseller/cli/releases/latest). Or use the install scripts:

```bash
curl -fsSL https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.sh | sh
```

```powershell
irm https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.ps1 | iex
```

**From source:** `git clone https://github.com/domainreseller/cli && cd cli && make install`

Check the install with `dna version` and `dna doctor`.

## Quick start

You need a **Reseller ID** (a UUID) and an **API key**. Both are in the reseller panel under **API settings**.
Production and OTE (test) are separate environments with **different API keys**.

```bash
# 1. Log in to the test environment first. The key is read hidden and the credentials are verified.
dna auth login --env ote

# 2. Look around (all read-only)
dna --ote account get                       # balances
dna --ote domain search example.com         # availability + price
dna --ote product tlds --filter-text com.tr # TLD rules and prices

# 3. Change things. Billable commands ask for confirmation; --dry-run previews the request.
dna --ote domain register example.com --ns ns1.example.net,ns2.example.net --handle DNA-12345 --dry-run
dna --ote domain register example.com --ns ns1.example.net,ns2.example.net --handle DNA-12345

# 4. Production
dna auth login              # stored as profile "production"
dna domain list
```

For CI and containers, skip the config file and use environment variables:

```bash
export DNA_RESELLER_ID=xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
export DNA_API_KEY=...        # keep it in your secret store
export DNA_ENV=ote            # or production (default)
dna domain list -o json
```

See [configuration](docs/configuration.md) for profiles, precedence and every environment variable.

## Commands

| group | what it does |
|---|---|
| `dna account` | balance, transactions |
| `dna domain` | search, bulk-search, list, info, register, renew, transfer, restore, lock, privacy, auth code, contacts, documents, forwarding, nameservers, child hosts |
| `dna dns` | zone records: list, add, update, delete, apply |
| `dna dnssec` | DS records: get, add, remove, disable |
| `dna contact` | contact handles and verification |
| `dna ssl` | CSR, buy, configure, status, download, reissue, revoke |
| `dna hosting` | packages, plans, create, status |
| `dna product` | products, prices, TLD catalogue, SSL catalogue |
| `dna order` | order details |
| `dna api` | raw request to any endpoint (like `gh api`) |
| `dna schema`, `dna enums` | machine-readable operation and enum descriptions |
| `dna mcp`, `dna agent` | MCP server, agent guide, Claude Code skill |
| `dna auth`, `dna config`, `dna doctor` | credentials, profiles, diagnostics |

Every command has `--help` with its safety class, API route and examples. The full list is in the
[command reference](docs/reference/README.md).

## Safety model

Every operation has a safety class:

| class | examples | behaviour |
|---|---|---|
| `read` | search, list, info, prices | runs immediately |
| `write` | DNS changes, lock, privacy, contacts | runs immediately (free, reversible) |
| `destructive` | delete record, revoke SSL, disable DNSSEC | asks for confirmation; non-interactive use needs `--yes` |
| `billable` | register, renew, transfer, restore, SSL/hosting purchase | asks for confirmation; non-interactive use needs `--yes` |

- Without a terminal and without `--yes`, a billable or destructive command exits with **code 9** and sends nothing.
- `--dry-run` prints the request that would be sent, with the API key masked.
- `--read-only`, `DNA_READ_ONLY=1` or a read-only profile blocks every change (exit code 10).

## Use with AI assistants

```bash
# Claude Code: register the MCP server (OTE, look-only is a good start)
claude mcp add dna -- dna mcp serve --env ote --read-only

# Snippets for Claude Desktop, Cursor, VS Code, Codex, Gemini CLI, Windsurf
dna mcp config

# Or give the agent the CLI itself plus the skill / guide
dna agent skill --install          # ~/.claude/skills/dna/SKILL.md
dna agent guide                    # compact usage guide for any agent
```

How the MCP server handles safety:
- Billable and destructive tools only execute with `confirm=true`. Without it they return a preview the assistant can show you.
- `--read-only`, `--no-billable` and `--groups domain,dns` limit what the assistant can reach.

Read [AI agents](docs/ai-agents.md) for the full picture.

## Output and scripting

```bash
dna domain info example.com                  # table on a terminal
dna domain info example.com -o json          # JSON (default when piped)
dna domain list --all -o jsonl               # one domain per line
dna domain info example.com --jq .authCode   # jq filter; strings print raw
dna dns add --body @record.json              # full input from a file (or --body - for stdin)
dna enums DomainStatus                       # what status 7 means
```

Exit codes are stable (see [errors](docs/errors.md)):

| code | meaning |
|---|---|
| 0 | success |
| 2 | invalid input; nothing was sent |
| 3 | authentication |
| 4 | not found |
| 5 | rejected by the API |
| 6 | rate limited |
| 7 | server error |
| 8 | network |
| 9 | confirmation required |
| 10 | read-only |

## Documentation

- [Getting started](docs/getting-started.md)
- [Configuration and credentials](docs/configuration.md)
- [Usage: inputs, output, scripting](docs/usage.md)
- [AI agents and MCP](docs/ai-agents.md)
- [Errors and exit codes](docs/errors.md)
- [Command reference](docs/reference/README.md) and [enums](docs/enums.md) (generated)
- [Architecture and contributing](docs/architecture.md), [CONTRIBUTING.md](CONTRIBUTING.md)

## License

[Apache 2.0](LICENSE)
