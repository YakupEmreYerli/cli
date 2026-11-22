# Usage: inputs, output and scripting

## Command shape

```
dna <group> [<subgroup>] <action> [positional args] [flags]
```

Every API command maps to one API operation and has an ID (`domain.register`), an MCP tool name (`domain_register`) and
a safety class. `dna <command> --help` shows all three, plus the HTTP route, body and result types and examples.

## Giving input

There are four ways to give input, and they combine:

1. **Positional arguments** for the obvious thing, usually the domain:
   `dna domain info example.com`, `dna domain transfer-check example.com 'AUTHCODE'`.
2. **Flags** for every field: `--period 2`, `--ns ns1.example.net` (repeatable, or comma separated), `--currency USD`.
   Field names follow the API in kebab-case (`nameServers` becomes `--name-servers`). Some fields have a shorter alias,
   such as `--ns`, `--content` or `--to`.
3. **Convenience flags** that build complex fields:

   | flag | builds |
   |---|---|
   | `--handle ROLE=HANDLE` / `--handle HANDLE` | contact handle list (a bare handle fills all four roles) |
   | `--contact @contact.json` | four role contacts from one contact |
   | `--ip 192.0.2.1` | child nameserver IPs (v4/v6 detected) |
   | `--ds "KEYTAG ALG DIGESTTYPE DIGEST"` | DS records |
   | `--document IdentityCard=./id.pdf` | registry document upload (base64 for you) |

4. **`--body` / `-d`** with the complete input as JSON: inline, `@file.json`, or `-` for stdin. Flags and positional
   arguments override fields from the body. The JSON shape is exactly `dna schema <operation>`:

   ```bash
   dna schema dns.add              # inputSchema + example
   cat > record.json <<'EOF'
   {"domainName":"example.com","zoneStruct":{"name":"www","type":"A","ttl":300,"contents":["192.0.2.10"]}}
   EOF
   dna dns add --body @record.json --ttl 600
   ```

JSON-typed flags (`--contacts`, `--handles`, …) also accept `@file` and `-`. String fields that hold PEM data
(`--csr`, `--private-key`) accept `@file`.

### Validation

Input is checked against the API schema **before anything is sent**:
- required fields
- types and ranges, such as `--period 1-10` and TTL `1-86400`
- maximum lengths
- enum values
- unknown fields, with a "did you mean" hint

Problems exit with code 2 and list every field. Enum names are normalized for you:
- `--type txt` becomes `TXT`
- `--dnssec-mode DisableDnssec` becomes `2`
- `--status-ids Active,PendingDelete` becomes `0,7`
- `--order-type Renew` becomes `2`

`dna enums` lists every coded value and what it means.

## Output

| `-o` | output |
|---|---|
| `auto` (default) | `table` when a person is at a terminal; `json` when piped or run by an AI agent ([below](#ai-agents-and-the-terminal-ui)) |
| `json` | the API response, unchanged (pretty and syntax-colored on a terminal, compact and plain when piped) |
| `jsonl` | one JSON value per line, for list responses |
| `table` | the human view described below; enum numbers become names, dates are shortened |
| `raw` | the response bytes exactly as received |

Set a default with `DNA_OUTPUT=json`. Commands that return nothing print `{"success":true}` (`✓ Done` in table mode).

### The terminal UI

When you run dna yourself in a terminal, the table format is a small UI rather than a log:

- **Tailored screens** for the commands people run most: `domain search` answers "available or taken, and for how
  much", `domain bulk-search` compares several names, `domain list` shows status dots and how many days are left
  (amber within 30 days, red once expired), `domain info` is a card with dates, lock, privacy, nameservers and contacts,
  `dns list` shows one value per line, `account get` shows both balances.
- **Every other command** gets an aligned table (lists) or a card (objects) with colored statuses, money with two
  decimals, `✓` for true and relative expiry dates. Tables shrink to the terminal width.
- **Next steps:** most screens end with the command you probably want next (`→ dna domain register ... --dry-run`).
- **Errors** are a short block: what went wrong, the API code, the fields to fix and the next step.
- **Billable and destructive commands** show what will be charged or deleted, on which environment, before asking
  `Continue? [y/N]`. `--dry-run` shows the request as a readable panel.
- **A spinner** on stderr while a request runs (it erases itself, so it never ends up in captured output).
- **`--help`** is colored.

Colors are the Domain Name API brand: the green of the logo (`#1CCC99`), the navy of the website (`#212A4D`, used
for badges) and its slate grey for secondary text.

### The interactive shell

Run `dna` on its own in a terminal to open the shell (wide terminals get the big block-letter logo):

```text
╭─ Domain Name API · dna ──────────────────────────────────────────────╮
│              ╔╦╗┌─┐┌┬┐┌─┐┬┌┐┌  ╔╗╔┌─┐┌┬┐┌─┐  ╔═╗╔═╗╦                │
│               ║║│ ││││├─┤││││  ║║║├─┤│││├┤   ╠═╣╠═╝║                │
│              ═╩╝└─┘┴ ┴┴ ┴┴┘└┘  ╝╚╝┴ ┴┴ ┴└─┘  ╩ ╩╩  ╩                │
╰──────────────────────────────────────────────────────────────────────╯
╭──────────────────────────────────────────────────────────────────────╮
│ › acme                                                               │
╰──────────────────────────────────────────────────────────────────────╯
  acme · 2 free of 4
   ✗ acme.com          taken
 › ✓ acme.net      11.49 USD / year
   ✓ acme.io       39.00 USD / year  premium
   – acme.zz     not offered
  ↑↓ choose · enter what next (register, transfer, …) · esc back
   OTE  test  ·  Acme Digital Ltd.  ·  1,284.50 USD      ? keys · ctrl+c ×2 exit
```

Nothing has to be memorised. Everything is a choice with ↑↓ and Enter, and **Esc always goes back one step**:

| start with | what happens |
|---|---|
| **↓ on an empty line** | quick actions: *My domains*, *Register a domain*, *DNS records*, *Balance*, *Switch environment*, *Connect an AI agent*, *Keys and help* (or press their number) |
| **typing a name** (`acme`) | availability and prices across .com .net .org .io .co .app .dev .ai appear while you type; ↓ picks a result and offers what to do: register a free one, check a transfer for a taken one |
| **`/`** | every shell command with a description: `/domains`, `/manage`, `/register`, `/search`, `/info`, `/dns`, `/renew`, `/balance`, `/transactions`, `/env`, `/login`, `/status`, `/doctor`, `/mcp`, `/help`, `/clear`, `/exit` |
| **a dna command** | `domain info acme.com`, `dns list acme.com --jq ...`; Tab completes command names and your own domain names |

- **Your domains** (`/domains`): type to search, Enter opens the domain's menu: *Details*, *DNS records*, *Renew…*,
  *Lock / Unlock transfers*, *WHOIS privacy on / off*, *Nameservers…*, *Auth code*. Each also has a one-key shortcut
  shown next to it.
- **Connect an AI agent** (`/agent`): choose from 20+ agents (Claude Code, ChatGPT, Cursor, Claude Desktop, Codex,
  VS Code, Windsurf, Gemini CLI, Zed, …), the environment and how much the agent may do; dna shows the exact setup,
  can run it or add itself to the agent's config file, checks the credentials and copies ready-made prompts. See
  [AI agents](ai-agents.md#mcp-server).
- **Anything that needs a domain** (`/dns`, `/renew`, *DNS records*, …) opens a searchable list of your domains; a
  domain that is not in the list can be typed in full.
- **Short forms instead of flags:** *Register* (years, nameservers, contact, with the total price), *Renew*
  (years), *Add a DNS record* (type, name, value, TTL, with an example for each type), *Nameservers*, *Transfer in*
  (auth code). ←→ change a choice, Enter moves on and submits on the last field. The nameservers you used are
  remembered for the next registration.
- **Contacts without copying handles:** the *Contact* field of *Register* offers your saved contacts, the default first
  (Enter lists them all; type to search), plus *New contact…*, a short form that creates the handle on the spot
  (the phone may include its country code, the country then fills itself in), and *Another handle…*. With no saved
  contact, the shell reads the contacts of a few of your domains in the background and offers them. `/contact`
  creates a contact, `/contacts` lists them.
- **Billable and destructive steps** end in a confirmation box built from a dry run of the exact request (fields,
  price, environment, method and path). *Cancel* is selected by default; nothing is sent until you choose the action
  or press `y`. The balance in the status line updates afterwards.
- **`/env ote`** and **`/env production`** (or the quick action) switch the environment for the session; the status
  line always shows which one is active (PRODUCTION in red) and your balance.
- ↑ browses the history (kept next to the config file, without lines that contain `--api-key`), Esc cancels a running
  request, Ctrl+L clears the screen, Ctrl+C twice exits, `?` shows every key.

The shell only opens for a person at a real terminal. Piped, under an AI agent or with `-o json`, `dna` on its own
prints the help as before.

Colors follow the usual conventions: `NO_COLOR=1` or `--no-color` turns them off (the layout stays),
`FORCE_COLOR=1` keeps them for `-o table` when stdout is not a terminal, and `TERM=dumb` disables them. On Windows
dna enables ANSI support in the console itself (Windows 10 and newer).

### AI agents and the terminal UI

The UI never reaches scripts or agents:

- When stdout is not a terminal, the output is plain JSON, whatever `FORCE_COLOR` says.
- Some agents run commands in a pseudo-terminal, which looks like a person at a terminal. dna detects them through
  `AI_AGENT` (set by Claude Code and other agents), `CLAUDECODE`, `GEMINI_CLI`, `CODEX_SANDBOX` or `CURSOR_AGENT`, and
  then behaves exactly as on a pipe: JSON output, JSON errors, no spinner and no prompt (billable and destructive
  commands exit with code 9 instead of waiting for an answer).
- `DNA_AGENT=1` forces this agent mode; `DNA_AGENT=0` turns detection off.

### jq

`--jq` / `-q` runs a [jq](https://jqlang.org) expression on the JSON result. It is built in, so jq does not need to be
installed. Strings are printed raw, as with `jq -r`:

```bash
dna domain info example.com --jq .authCode
dna domain list --all --jq '.items[] | select(.remainingDay < 30) | "\(.domainName) \(.expirationDate)"'
dna product tlds --all --jq '[.items[] | select(.isDnssecAvailable) | .name] | length'
dna schema --jq '.[] | select(.safety == "billable") | .command'
```

### Pagination

List commands accept `--skip-count` and `--max-result-count`. Add `--all` to fetch every page; the output is then a
single `{"items": [...], "totalCount": N}` object.

## Safety switches

| flag | effect |
|---|---|
| `--dry-run` | validate, then print the HTTP request (method, URL, headers with the key masked, body) instead of sending it; JSON when piped, a readable panel on a terminal |
| `--yes` / `-y` | confirm billable/destructive commands without a prompt |
| `--read-only` | refuse anything that is not a read (also `DNA_READ_ONLY=1`, or a read-only profile) |

Without a terminal (or under an AI agent) and without `--yes`, billable and destructive commands exit with code 9. A
script cannot accidentally buy something.

## Debugging

```bash
dna -v domain info example.com      # requests, responses, retries and quota info on stderr
dna doctor                          # config, credentials, connectivity, authentication
dna api /api/v1/domains/info -f DomainName=example.com   # raw call to any endpoint
```

`dna api` sends authenticated requests to any path, like `gh api`:
- The method defaults to GET, or POST with `--body`. Use `-X` to pick another.
- Add query parameters with `-f key=value`.
- It still honours `--read-only` (GET only), `--dry-run`, `--jq` and `-o`.

## Rate limits, quota and retries

The gateway enforces, per API key:

| limit | value |
|---|---|
| general | 5 requests/second |
| `domain search` / `bulk-search` | 2 requests/second (counts towards the general limit) |
| DNS zone endpoints | 20 requests/second (separate budget) |
| `domain info`, `product tlds` | not rate limited |

Beyond a rate limit the gateway answers 429. Separately, a **soft quota** slows requests down instead of rejecting them:
- Searches and info lookups cost 1 point; everything else costs 10.
- Past 200 points each request is delayed by up to 5 s.
- Points decay by 20% a minute.

What `dna` does about it:
- Paces requests on the client to stay under the limits. DNS zone calls are spaced 1 s apart.
- Retries 429 responses automatically, honouring `Retry-After`. This is safe even for billable calls, because the gateway rejects the request before it reaches the backend.
- Retries network errors and 502/503/504 only for reads.
- Prints a warning when the quota delayed a request. `-v` shows quota usage on every call.

## Shell completion

```bash
dna completion bash > /etc/bash_completion.d/dna
dna completion zsh > "${fpath[1]}/_dna"
dna completion fish > ~/.config/fish/completions/dna.fish
dna completion powershell | Out-String | Invoke-Expression
```

## Recipes

```bash
# Domains expiring in the next 30 days, as CSV
dna domain list --all --jq '.items[] | select(.remainingDay <= 30) | [.domainName, .expirationDate] | @csv'

# Renew them, after a review (the prompt asks for each one)
for d in $(dna domain list --all --jq '.items[] | select(.remainingDay <= 30) | .domainName'); do
  dna domain renew-check "$d" --period 1 --jq '"\(.renewabilityStatus) \(.premiumPrice // "")"'
done

# Check many names at once
dna domain bulk-search brand.com brand.net brand.io --jq '.infos[] | "\(.domainName)\t\(.status)\t\(.price)"'

# Save the issued certificate (certificates are addressed by ID, not by domain name)
id=$(dna ssl list example.com --jq '.items[0].certificateId')
dna ssl content "$id" --jq .certificate > example.crt
```
