# AI agents and MCP

`dna` is designed to be operated by AI assistants as safely as by people. There are three ways to connect an agent.

| way | best for | how |
|---|---|---|
| **MCP server** | chat assistants and IDEs (Claude Desktop, Claude Code, Cursor, VS Code, Codex, Gemini CLI, Windsurf) | `dna mcp serve` |
| **CLI + skill** | coding agents with a shell (Claude Code, Codex, Aider, …) | `dna agent skill --install`, or `dna agent guide` |
| **CLI + schema** | your own agent framework / tool definitions | `dna schema`, `dna mcp tools` |

All three are generated from the same operation registry, so they always describe the installed version exactly.

**Fastest start:** copy one of the [ready-made prompts](#ready-made-prompts) into Claude Code, Codex, ChatGPT or Claude chat and let the assistant set dna up for you.

## Guarantees an agent can rely on

| property | detail |
|---|---|
| Machine-readable output | JSON whenever stdout is not a terminal or an agent is detected (`AI_AGENT`, `CLAUDECODE`, `DNA_AGENT=1`, [more](usage.md#ai-agents-and-the-terminal-ui)); `-o json` to force; the API response is passed through unchanged |
| No terminal UI for agents | the colored screens, the spinner and the interactive shell are for people only; `dna` on its own prints the help for agents; agents get the same bytes as before |
| Machine-readable errors | `{"error":{"kind","code","message","status","fields","hint","correlationId","retryable","exitCode"}}` on stderr |
| Stable exit codes | 0 ok, 2 invalid input, 3 auth, 4 not found, 5 rejected, 6 rate limited, 7 server, 8 network, 9 confirmation required, 10 read-only ([errors.md](errors.md)) |
| No hidden prompts | prompts only appear on an interactive terminal and never under an agent, even in a pseudo-terminal; otherwise commands fail fast with a clear code |
| Money and data safety | `billable` and `destructive` commands need `--yes` (CLI) or `confirm=true` (MCP) |
| Preview | `--dry-run` prints the exact request; MCP calls without `confirm` return a preview |
| Self-description | `dna schema <op>` gives the input JSON Schema plus an example; `dna enums` explains every coded value |
| Early validation | inputs are validated before sending, with per-field messages and "did you mean" hints |
| Idempotent retries | rate limits are retried automatically; billable calls are never re-sent after a network error |

## MCP server

### Setup

The easiest way: run `dna` on its own and choose **Connect an AI agent** (or type `/agent`). Pick your agent from
the list, choose the environment (OTE or production) and how much it may do (read-only, no purchases, or everything
with confirmations), and dna:

- shows the exact command or config snippet for that agent, with the absolute path of the dna program (AI apps often
  start with a different `PATH` than your terminal),
- runs the setup for you where it can (`claude mcp add`, `code --add-mcp`, Cursor's install link, or adding itself
  to the agent's JSON / TOML config file while keeping your other servers and saving a `.bak` copy),
- checks that the credentials for that environment are stored where the agent's dna will find them (not only in
  your terminal's environment variables),
- copies the snippet or one of the ready-made prompts to the clipboard.

Agents in the list: Claude Code, ChatGPT (through OpenAI Codex, or ready-made prompts), Cursor, Claude Desktop,
OpenAI Codex, VS Code (GitHub Copilot), Windsurf, Gemini CLI, Zed, Cline, Roo Code, Continue, Goose, opencode,
Amazon Q Developer, JetBrains AI Assistant, LM Studio, Warp, GitHub Copilot CLI, any other MCP client, and chat
assistants without local tools.

The same instructions without the shell:

```bash
dna auth login --env ote                 # once; or provide env vars in the client config
dna mcp config                           # prints snippets for every client
dna mcp config --client claude-code --env ote --read-only
dna mcp config --client claude-desktop --env ote --full-path   # absolute path of the dna program
```

**Claude Code**

```bash
claude mcp add dna -- dna mcp serve --env ote
claude mcp add dna --scope project -- dna mcp serve --env ote --read-only   # shared via .mcp.json
```

**Claude Desktop / Cursor / Windsurf / Gemini CLI** (`mcpServers` JSON):

```json
{
  "mcpServers": {
    "dna": {
      "command": "dna",
      "args": ["mcp", "serve", "--env", "ote"],
      "env": { "DNA_RESELLER_ID": "…", "DNA_API_KEY": "…" }
    }
  }
}
```

**VS Code** (`.vscode/mcp.json`): the same entry under `"servers"` with `"type": "stdio"`.

**Codex CLI** (`~/.codex/config.toml`):

```toml
[mcp_servers.dna]
command = "dna"
args = ["mcp", "serve", "--env", "ote"]
```

If the client cannot find `dna`, use the absolute path of the binary as `command`.

### What the assistant gets

- **One tool per API operation (64)**, named after the command: `domain_search`, `domain_register`, `dns_add`, `ssl_configure`, …
  - Each tool has a JSON Schema built from the API spec, with enum values, ranges and lengths.
  - Each tool carries MCP annotations (`readOnlyHint`, `destructiveHint`, `idempotentHint`).
- **`dna_context`**: reports the environment (production or OTE), whether read-only mode is on, and which tools are enabled. It makes no API call.
- **Server `instructions`**: tell the model the environment and the safety rules when it connects.
- **Paged tools**: take `skipCount`/`maxResultCount`, or `all=true` to fetch every page.

### Confirmation protocol

Tools whose class is `billable` or `destructive` take an extra `confirm` boolean.

1. The model calls `domain_register` without `confirm`. Nothing is executed. The result has `isError: true`, kind
   `confirmation_required` and a `preview` of the exact request.
2. The model shows the preview to the user and asks for approval.
3. After approval it calls again with the same arguments plus `"confirm": true`.

Most MCP clients also ask the user before every tool call; the annotations help them flag dangerous tools.

### Limiting what the assistant can do

```bash
dna mcp serve --read-only                 # only read tools are exposed (also DNA_READ_ONLY=1 or a read-only profile)
dna mcp serve --no-billable               # everything except purchases
dna mcp serve --groups domain,dns         # only these command groups
dna mcp serve --env ote                   # test environment only
```

Recommended starting point: `--env ote`, then `--read-only` in production, and full access only for trusted
workflows. The full tool list is about 55 KB of schema; `--groups` and `--read-only` also make it smaller, which saves
context tokens.

### Inspecting the tools

```bash
dna mcp tools | jq '.[].name'
dna mcp tools --read-only | jq length
```

## Coding agents with a shell

### Claude Code skill

```bash
dna agent skill --install                       # ~/.claude/skills/dna/SKILL.md (personal)
dna agent skill --install --dir .claude/skills  # project skill, commit it
```

The skill is in this repository at [`skills/dna/SKILL.md`](../skills/dna/SKILL.md). Claude Code loads it when a task
involves domains, DNS, SSL or hosting.

### Any agent: the guide

```bash
dna agent guide
```

prints a compact Markdown guide: rules, exit codes, error format, typical workflows and a table of every command with
its safety class. Paste it into a system prompt, an `AGENTS.md`, or let the agent run it itself.

### Rules worth putting in your agent's instructions

1. Use `--env ote` unless the user explicitly works on production.
2. Always pass `-o json` and parse JSON; use `--jq` to keep outputs small.
3. Never add `--yes` on your own. Run with `--dry-run`, show the plan, get approval, then run with `--yes`.
4. After exit code 8 (network) or a timeout on a billable command, check the state (`domain info`) before retrying.
5. Use `dna schema <op>` when unsure about inputs; `dna enums <Name>` to explain numeric codes.

## Building your own tools

`dna schema` returns every operation:

```json
[{"id":"domain.register","command":"dna domain register","mcpTool":"domain_register","summary":"…",
  "safety":"billable","requiresConfirmation":true,"method":"POST","path":"/api/v1/domains/register",
  "args":["domainName"],"returns":"DomainOperationResponse"}, …]
```

and `dna schema domain.register` returns `inputSchema` (JSON Schema), `example` and, with `--result`, the
`resultSchema`. Execute an operation from a tool call by passing the arguments as the body:

```bash
echo '{"domainName":"example.com"}' | dna domain search --body - -o json
```

<a id="ready-made-prompts"></a>

## Ready-made prompts for Claude and ChatGPT

Copy a prompt, paste it into your AI assistant and let it do the work. Every prompt starts in the OTE test environment and stops for your approval before anything billable.

### Set up dna in Claude Code

```text
Set up the dna CLI for me (https://github.com/domainreseller/cli).
1. Install it with the official install script for my operating system, then run
   `dna version` and `dna doctor`.
2. Install the Claude Code skill with `dna agent skill --install` and add the MCP
   server with `claude mcp add dna -- dna mcp serve --env ote`.
3. Don't ask me to paste my API key into this chat. Tell me to run
   `dna auth login --env ote` myself, then check the connection with
   `dna --ote account get`.
Work only in the OTE test environment and never use `--yes` without my approval.
```

### Set up dna in ChatGPT Codex

```text
Set up the dna CLI for me (https://github.com/domainreseller/cli).
1. Install it with the official install script for my operating system, then run
   `dna version` and `dna doctor`.
2. Add it to my Codex config (`~/.codex/config.toml`) as an MCP server named `dna`
   with the command `dna` and the arguments `mcp serve --env ote`.
3. Don't ask me to paste my API key into this chat. Tell me to run
   `dna auth login --env ote` myself, then check the connection with
   `dna --ote account get`.
Work only in the OTE test environment and never use `--yes` without my approval.
```

### No terminal? Ask ChatGPT or Claude chat

```text
Read https://raw.githubusercontent.com/domainreseller/cli/main/llms.txt and the dna
docs it links to. Then give me the exact dna commands, step by step, to register a
domain in the OTE test environment: install, log in, search, preview with `--dry-run`
and register. Explain what each command does.
```

### More prompts for everyday work

#### Find and register a domain

```text
With dna, check whether brandname.com, brandname.net and brandname.io are available.
Show the results with prices in a table, then preview the registration of the cheapest
available one with `--dry-run`. Don't register anything until I say yes.
```

#### Upcoming renewals

```text
With dna, list my domains that expire in the next 60 days with their renewal prices,
sorted by expiry date. Don't renew anything.
```

#### Point a domain to a server

```text
With dna, point example.com to 203.0.113.10: add A records for `@` and `www`, then
show me the final DNS zone. Ask me before changing or deleting any existing record.
```

#### Transfer a domain in

```text
With dna, check whether example.org can be transferred to my account, tell me what I
still need (auth code, unlocking) and preview the transfer with `--dry-run`.
```

#### Order an SSL certificate

```text
With dna, find the cheapest DV SSL certificate for example.com, create a CSR and
preview the order. Wait for my approval before buying.
```

#### Enable DNSSEC

```text
With dna, show me the DNSSEC status of example.com, then add this DS record from my
DNS provider: 2371 13 2 1F987CC6583E92DF0890718C42… Preview the change with
`--dry-run` first and wait for my approval before applying it.
```

The same prompts, translated, are in every README language: [EN](../README.md#ready-made-prompts) · [TR](../README-TR.md#ready-made-prompts) · [DE](../README-DE.md#ready-made-prompts) · [AR](../README-SA.md#ready-made-prompts) · [NL](../README-NL.md#ready-made-prompts) · [AZ](../README-AZ.md#ready-made-prompts) · [ZH](../README-CN.md#ready-made-prompts) · [FR](../README-FR.md#ready-made-prompts) · [IT](../README-IT.md#ready-made-prompts)
