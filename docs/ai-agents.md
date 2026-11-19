# AI agents and MCP

`dna` is designed to be operated by AI assistants as safely as by people. There are three ways to connect an agent.

| way | best for | how |
|---|---|---|
| **MCP server** | chat assistants and IDEs (Claude Desktop, Claude Code, Cursor, VS Code, Codex, Gemini CLI, Windsurf) | `dna mcp serve` |
| **CLI + skill** | coding agents with a shell (Claude Code, Codex, Aider, …) | `dna agent skill --install`, or `dna agent guide` |
| **CLI + schema** | your own agent framework / tool definitions | `dna schema`, `dna mcp tools` |

All three are generated from the same operation registry, so they always describe the installed version exactly.

## Guarantees an agent can rely on

| property | detail |
|---|---|
| Machine-readable output | JSON whenever stdout is not a terminal (`-o json` to force); the API response is passed through unchanged |
| Machine-readable errors | `{"error":{"kind","code","message","status","fields","hint","correlationId","retryable","exitCode"}}` on stderr |
| Stable exit codes | 0 ok, 2 invalid input, 3 auth, 4 not found, 5 rejected, 6 rate limited, 7 server, 8 network, 9 confirmation required, 10 read-only ([errors.md](errors.md)) |
| No hidden prompts | prompts only appear on an interactive terminal; otherwise commands fail fast with a clear code |
| Money and data safety | `billable` and `destructive` commands need `--yes` (CLI) or `confirm=true` (MCP) |
| Preview | `--dry-run` prints the exact request; MCP calls without `confirm` return a preview |
| Self-description | `dna schema <op>` gives the input JSON Schema plus an example; `dna enums` explains every coded value |
| Early validation | inputs are validated before sending, with per-field messages and "did you mean" hints |
| Idempotent retries | rate limits are retried automatically; billable calls are never re-sent after a network error |

## MCP server

### Setup

```bash
dna auth login --env ote                 # once; or provide env vars in the client config
dna mcp config                           # prints snippets for every client
dna mcp config --client claude-code --env ote --read-only
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

## Example prompts

Copy-paste prompts for setting up dna in Claude Code or Codex, and for everyday tasks, are in the README under [Ready-made prompts](../README.md#ready-made-prompts) (also translated in every README language). A few more ideas:

- "Using the dna tools on OTE, check whether *brandname* is available as .com, .net and .com.tr and show prices."
- "List my domains that expire within 45 days and check whether each can be renewed. Don't renew anything."
- "Point www.example.com to 192.0.2.10 with a 5 minute TTL and show me the zone afterwards."
- "Enable DNSSEC for example.com with this DS record: 2371 13 2 1F98…"
