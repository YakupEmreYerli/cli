<p align="center">
  <a href="README-TR.md"><img src="./assets/lang/tr.svg" height="32" alt="Türkçe"></a>
  <a href="README.md"><img src="./assets/lang/en-active.svg" height="32" alt="English"></a>
  <a href="README-DE.md"><img src="./assets/lang/de.svg" height="32" alt="Deutsch"></a>
  <a href="README-SA.md"><img src="./assets/lang/sa.svg" height="32" alt="العربية"></a>
  <a href="README-NL.md"><img src="./assets/lang/nl.svg" height="32" alt="Nederlands"></a>
  <a href="README-AZ.md"><img src="./assets/lang/az.svg" height="32" alt="Azərbaycanca"></a>
  <a href="README-CN.md"><img src="./assets/lang/cn.svg" height="32" alt="中文"></a>
  <a href="README-FR.md"><img src="./assets/lang/fr.svg" height="32" alt="Français"></a>
  <a href="README-IT.md"><img src="./assets/lang/it.svg" height="32" alt="Italiano"></a>
</p>

<!--
  dna: the official Domain Name API command line client and MCP server, built for AI agents and developers.
  Every animation on this page is a hand-made SVG in ./assets. No external widgets, no trackers.
-->

<a href="https://github.com/domainreseller/cli/releases/latest"><img src="./assets/hero.svg" width="1200" alt="dna, the Domain Name API CLI driven by AI agents: an agent searches a domain, previews the billable registration with --dry-run, asks for approval and registers it."></a>

<h1 align="center">dna: the Domain Name API CLI for AI agents</h1>

<p align="center">
  <a href="https://github.com/domainreseller/cli/releases/latest"><img src="https://img.shields.io/github/v/release/domainreseller/cli?label=version&style=flat-square&color=12B385" alt="Latest release"></a>
  <a href="https://github.com/domainreseller/cli/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/domainreseller/cli/ci.yml?branch=main&label=CI&style=flat-square" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-0B8DB5?style=flat-square" alt="Apache-2.0"></a>
</p>

<p align="center">
  <b>dna</b> drives the <a href="https://www.domainnameapi.com">Domain Name API</a> reseller platform from the command line and from AI agents.
  It covers all 64 API operations: domain search, registration, renewal, transfer, DNS, DNSSEC, contacts, SSL and hosting.
  Plug it into <b>Claude Code, Claude Desktop, Cursor, Codex</b> or any MCP client, and every operation becomes a safe, well-described tool.
</p>

<img src="./assets/stats.svg" width="1200" alt="64 API operations, 65 MCP tools for AI agents, 4 enforced safety classes, 6 platforms from one binary.">

## <img src="./assets/h-agent.svg" width="30" height="30" alt=""> Built for AI agents

|  | What an agent gets |
|---|---|
| 🧰 **One tool per operation** | `dna mcp serve` exposes 65 MCP tools with JSON Schemas built from the API spec, so models fill inputs correctly the first time. |
| 🛡️ **A safety gate** | Billable and destructive calls never run on their own. The CLI needs `--yes`, MCP needs `confirm=true`, and without it the agent gets a preview to show you. |
| 🧾 **Machine-readable everything** | JSON output when piped, JSON errors with stable exit codes, `--dry-run` for the exact request, and `dna schema` for every input. |
| 🧪 **A sandbox** | `--env ote` points at the test environment: real API, no real money. `--read-only` blocks every change. |
| 📘 **Self-documenting** | `dna agent guide` teaches an agent the whole CLI in one call. `dna agent skill --install` adds a Claude Code skill. |

## <img src="./assets/h-install.svg" width="30" height="30" alt=""> Install

> [!IMPORTANT]
> **Always install from the _latest release_.** Release binaries are versioned, tested and published with SHA-256 checksums.

**Linux / macOS**

```bash
curl -fsSL https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.ps1 | iex
```

**With Go 1.26+**

```bash
go install github.com/domainreseller/cli/cmd/dna@latest
```

<details>
<summary><b>Manual download</b></summary>
<br>

Pick your archive on the [latest release](https://github.com/domainreseller/cli/releases/latest), check it against `checksums.txt`, unpack it and put `dna` (or `dna.exe`) on your `PATH`.

| OS | x64 | ARM |
|---|---|---|
| Linux | `dna_<version>_linux_amd64.tar.gz` | `dna_<version>_linux_arm64.tar.gz` |
| macOS | `dna_<version>_darwin_amd64.tar.gz` | `dna_<version>_darwin_arm64.tar.gz` |
| Windows | `dna_<version>_windows_amd64.zip` | `dna_<version>_windows_arm64.zip` |

</details>

Check the setup with `dna version` and `dna doctor`.

## <img src="./assets/h-terminal.svg" width="30" height="30" alt=""> Quick start

```bash
# 1. Log in to the test environment first (it has its own API key)
dna auth login --env ote

# 2. Look around: nothing here changes anything
dna --ote account get
dna --ote domain search example.com
dna --ote product tlds --filter-text com

# 3. Preview a purchase, then make it (it asks for confirmation)
dna --ote domain register example.com --ns tr.apiname.com,eu.apiname.com --handle <HANDLE> --dry-run
dna --ote domain register example.com --ns tr.apiname.com,eu.apiname.com --handle <HANDLE>

# 4. Production
dna auth login
dna domain list
```

## <img src="./assets/h-mcp.svg" width="30" height="30" alt=""> Connect your AI agent

<img src="./assets/mcp.svg" width="1200" alt="Model Context Protocol: dna mcp serve exposes 65 tools with safety annotations. A billable tool call without confirm returns a preview; with confirm=true after approval it runs.">

```bash
claude mcp add dna -- dna mcp serve --env ote      # Claude Code
dna mcp config                                     # snippets for every other client
```

<details>
<summary><b>Claude Desktop · Cursor · Windsurf · Gemini CLI</b></summary>
<br>

```json
{
  "mcpServers": {
    "dna": {
      "command": "dna",
      "args": ["mcp", "serve", "--env", "ote"]
    }
  }
}
```

Claude Desktop: `claude_desktop_config.json` · Cursor: `~/.cursor/mcp.json` · Windsurf: `~/.codeium/windsurf/mcp_config.json` · Gemini CLI: `~/.gemini/settings.json`
</details>

<details>
<summary><b>VS Code</b></summary>
<br>

`.vscode/mcp.json`:

```json
{
  "servers": {
    "dna": { "type": "stdio", "command": "dna", "args": ["mcp", "serve", "--env", "ote"] }
  }
}
```
</details>

<details>
<summary><b>OpenAI Codex CLI</b></summary>
<br>

`~/.codex/config.toml`:

```toml
[mcp_servers.dna]
command = "dna"
args = ["mcp", "serve", "--env", "ote"]
```
</details>

<details>
<summary><b>Agents with a shell (skill and guide)</b></summary>
<br>

```bash
dna agent skill --install                       # Claude Code skill: ~/.claude/skills/dna/SKILL.md
dna agent skill --install --dir .claude/skills  # project skill, commit it with your repo
dna agent guide                                 # compact usage guide for any agent
```
</details>

You choose how far the agent can reach:

```bash
dna mcp serve --read-only            # look, don't touch
dna mcp serve --no-billable          # everything except purchases
dna mcp serve --groups domain,dns    # only these command groups
```

## <img src="./assets/h-flow.svg" width="30" height="30" alt=""> How it works

<img src="./assets/flow.svg" width="1200" alt="An AI agent or you call dna over MCP or the CLI; dna validates the input, enforces the safety gate and talks to the Domain Name API (production or the OTE sandbox), which reaches the registries over EPP.">

## <img src="./assets/h-shield.svg" width="30" height="30" alt=""> Safety gate

<img src="./assets/safety.svg" width="1200" alt="Read and write commands run immediately; destructive and billable commands stop until they are confirmed with --yes in the CLI or confirm=true over MCP.">

| class | examples | behaviour |
|---|---|---|
| `read` | search, list, info, prices | runs immediately |
| `write` | DNS records, lock, privacy, contacts | runs immediately (free, reversible) |
| `destructive` | delete a record, revoke SSL, disable DNSSEC | stops until confirmed |
| `billable` | register, renew, transfer, restore, buy SSL/hosting | stops until confirmed |

- **In a script or agent shell:** without `--yes`, a gated command exits with code **9** and sends nothing.
- **Over MCP:** without `confirm=true`, the tool returns a preview of the exact request.
- **Previewing:** `--dry-run` prints the HTTP request with the API key masked.
- **Locking it down:** `--read-only`, `DNA_READ_ONLY=1` or a read-only profile blocks every change (exit code **10**).

## <img src="./assets/h-output.svg" width="30" height="30" alt=""> Output, errors and exit codes

```bash
dna domain info example.com                   # table on a terminal
dna domain info example.com -o json           # JSON (the default when piped)
dna domain list --all -o jsonl                # one domain per line
dna domain info example.com --jq .authCode    # built-in jq
dna dns add --body @record.json               # full input from a file, or --body - for stdin
dna schema domain.register                    # JSON Schema of any input
dna enums DomainStatus                        # what status 7 means
```

Errors are JSON on stderr: `{"error":{"kind","code","message","status","fields","hint","correlationId","retryable","exitCode"}}`

| code | meaning | code | meaning |
|---:|---|---:|---|
| 0 | success | 6 | rate limited |
| 1 | unexpected error | 7 | server error |
| 2 | invalid input, nothing sent | 8 | network |
| 3 | authentication | 9 | confirmation required |
| 4 | not found | 10 | read-only mode |
| 5 | rejected by the API | | |

## <img src="./assets/h-commands.svg" width="30" height="30" alt=""> Commands

| group | what it does |
|---|---|
| `dna domain` | search, bulk search, list, info, register, renew, transfer, restore, lock, privacy, auth code, contacts, documents, forwarding, nameservers, child hosts |
| `dna dns` · `dna dnssec` | zone records, DS records |
| `dna contact` | contact handles and verification |
| `dna ssl` | CSR, buy, configure, status, download, reissue, revoke |
| `dna hosting` · `dna product` · `dna order` · `dna account` | hosting, catalogue and prices, orders, balance and transactions |
| `dna api` | raw request to any endpoint |
| `dna schema` · `dna enums` · `dna agent` · `dna mcp` | for agents and tools |
| `dna auth` · `dna config` · `dna doctor` | credentials, profiles, diagnostics |

Every command's `--help` shows its safety class, API route and examples. See the full [command reference](docs/reference/README.md).

## <img src="./assets/h-key.svg" width="30" height="30" alt=""> API credentials

`dna` needs the same two values as every Domain Name API module:

| value | what it is |
|---|---|
| **Reseller ID** | a UUID like `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` |
| **API Key** | your API key; production and OTE have **different** keys |

> [!TIP]
> Find both in your Domain Name API panel under **API Settings**. Store them with `dna auth login`, or use the `DNA_RESELLER_ID` and `DNA_API_KEY` environment variables in CI.

> [!CAUTION]
> These are **API credentials**. Your panel login e-mail and password will **not** work here.

## <img src="./assets/h-faq.svg" width="30" height="30" alt=""> FAQ

<details>
<summary><b>Is it safe to let an AI agent use dna?</b></summary>
<br>
Billable and destructive operations are gated: the CLI refuses them without <code>--yes</code>, and the MCP server returns a preview unless the call carries <code>confirm=true</code>, which agents are instructed to set only after you approve. Start agents on <code>--env ote</code>, and use <code>--read-only</code> or <code>--no-billable</code> to limit them further.
</details>

<details>
<summary><b>Can I try it without spending money?</b></summary>
<br>
Yes. <code>--env ote</code> uses the OTE test environment with its own API key and balance. Nothing there touches live domains or your real balance.
</details>

<details>
<summary><b>Which AI tools work with it?</b></summary>
<br>
Any MCP client: Claude Code, Claude Desktop, Cursor, VS Code, Windsurf, OpenAI Codex CLI, Gemini CLI and others. Agents that only have a shell can call the CLI directly; <code>dna agent guide</code> and the Claude Code skill explain it to them.
</details>

<details>
<summary><b>Where are my credentials stored?</b></summary>
<br>
<code>dna auth login</code> keeps them in a profile file in your user config folder (<code>dna config path</code> shows where), written with owner-only permissions on Linux and macOS. Environment variables always win over the file, and the API key is masked in every output.
</details>

<details>
<summary><b>Do I need Go?</b></summary>
<br>
No. The install scripts and the release archives contain a single binary. Go 1.26+ is only needed for <code>go install</code> or building from source.
</details>

<details>
<summary><b>Is there a library for my own code?</b></summary>
<br>
<code>dna</code> is a CLI and MCP server. To call the API from code, use the official <a href="https://github.com/domainreseller/php-dna">PHP</a>, <a href="https://github.com/domainreseller/nodejs-dna">Node.js</a>, <a href="https://github.com/domainreseller/python-dna">Python</a>, <a href="https://github.com/domainreseller/ruby-dna">Ruby</a> or <a href="https://github.com/domainreseller/go-dna">Go</a> libraries.
</details>

## <img src="./assets/h-docs.svg" width="30" height="30" alt=""> Documentation

| guide | for |
|---|---|
| [Getting started](docs/getting-started.md) | install, OTE login, first registration and DNS change |
| [Configuration](docs/configuration.md) | credentials, environments, profiles, environment variables, read-only mode |
| [Usage](docs/usage.md) | inputs, `--body`, output formats, `--jq`, pagination, rate limits, recipes |
| [AI agents and MCP](docs/ai-agents.md) | MCP setup per client, confirmation protocol, skill, agent guide |
| [Errors and exit codes](docs/errors.md) | error JSON, error codes, troubleshooting |
| [Command reference](docs/reference/README.md) · [Enums](docs/enums.md) | every command, flag and coded value (generated) |
| [Architecture](docs/architecture.md) · [Contributing](CONTRIBUTING.md) | how the code is organized |

## <img src="./assets/h-connect.svg" width="30" height="30" alt=""> Connect

<p align="center">
  <a href="https://www.domainnameapi.com"><img src="./assets/btn-web.svg" height="46" alt="Domain Name API: domain reseller program"></a>
  <a href="https://www.domainnameapi.com/domain-reseller"><img src="./assets/btn-signup.svg" height="46" alt="Become a domain reseller"></a>
  <a href="https://www.domainnameapi.com/api-integration"><img src="./assets/btn-api.svg" height="46" alt="Domain reseller API & modules"></a>
  <a href="https://www.domainnameapi.com/contacts"><img src="./assets/btn-support.svg" height="46" alt="Support"></a>
</p>

<p align="right">
  <sub>Follow us</sub><br>
  <a href="https://www.atakdomain.com"><img src="./assets/pill-atak.svg" height="34" alt="atakdomain.com"></a>
  <a href="https://www.linkedin.com/company/domain-name-reseller/"><img src="./assets/pill-linkedin.svg" height="34" alt="LinkedIn"></a>
  <a href="https://x.com/dna_resellerapi"><img src="./assets/pill-x.svg" height="34" alt="X"></a>
  <a href="https://www.youtube.com/DomainReseller"><img src="./assets/pill-youtube.svg" height="34" alt="YouTube"></a>
</p>

<p align="center"><sub>Apache-2.0 · made for people and the agents working with them</sub></p>
