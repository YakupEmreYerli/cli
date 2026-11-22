# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses [Semantic Versioning](https://semver.org).

## [Unreleased]

## [1.3.0] - 2026-11-22

### Changed

- **Breaking, forced by the API:** the SSL API now identifies a certificate by its `certificateId` instead of the domain
  name, because a domain can have several certificates. The API changed on 2026-11-20 in production and OTE, so the
  affected commands of earlier versions no longer work.
  - `ssl configure`, `ssl status`, `ssl get`, `ssl content`, `ssl reissue` and `ssl revoke` take the certificate ID as
    their argument: `dna ssl status 3fa85f64-5717-4562-b3fc-2c963f66afa6`. The same goes for the MCP tools, whose
    `domainName` input became `certificateId`.
  - A domain name passed where the ID belongs is rejected before anything is sent (exit code 2), and the message says how
    to find the ID: `dna ssl list example.com`.
  - `ssl approvers` takes `--certificate-id` for a certificate you bought, or the domain name with `--package-code`
    before buying. The package code is now required with a domain name.
  - `ssl initialize` is unchanged. Keep the `certificateId` it returns.

### Added

- `ssl list [domain]` lists your certificates with their IDs, newest first.
- `ssl renew <certificateId>` renews a certificate for another year at the renewal price (billable). It returns the ID
  of the new certificate, which you then configure. Do not renew by buying again with `ssl initialize`.
- `account profile` shows the reseller profile: name, company, price group, account currency, status and whether MCP
  access is switched on in the reseller panel.
- The `AccountCurrency` and `ResellerStatus` enums. dna now covers 67 API operations and exposes 68 MCP tools.

## [1.2.1] - 2026-11-20

### Fixed

- Shell: a name typed without an extension and Enter (for example `acme`) checked all eight popular extensions at once,
  and on OTE the shell waited close to a minute for the registries that do not answer. Enter now checks `acme.com` alone,
  which answers in about a second. A name with an extension is checked as typed, and `/search acme` still compares the
  popular extensions.
- Shell: the live list under the prompt checks the first name (the `.com`, or the domain typed) on its own as well, so it
  shows within a second while the other extensions load. If they do not answer in time, the `.com` stays with a note.

## [1.2.0] - 2026-11-20

### Added

- A contact book per profile, so registering no longer means hunting for handles:
  - `domain register` without `--handle` uses the default contact for all four roles, and the platform nameservers
    without `--ns`. At a terminal with no default contact, dna offers to create one on the spot, find the ones on your
    domains or check a typed handle. Scripts and agents get exit code 2 with these options.
  - `contact create` asks for the details you leave out at a terminal, and `--phone` takes the full number
    (`+90 555 123 45 67`): the calling code is split off and the country filled in. The new handle is saved, and becomes
    the default when there is none.
  - New `contact list [--scan]`, `contact default [handle]` and `contact forget <handle>`. `domain info` and
    `contact get` also save the handles they show.
  - Shell: the registration form's Contact field lists the saved contacts (Enter opens a searchable list) plus
    "New contact…" and "Another handle…". With an empty book it reads a few domains in the background. New `/contact`
    and `/contacts`.

### Fixed

- Shell: live search waited only 20 seconds, so on OTE (which can take about 50 seconds when a registry does not answer)
  it never showed results. It now waits up to 90 seconds.
- Shell: extensions whose registry did not answer were shown as "not offered"; they now read "no answer".
- Shell: narrow terminals no longer wrap or cut lines (welcome box, status line, forms, domain browser), long menu
  labels no longer push the descriptions off, and the registration result lost a stray gap.
- The install scripts can update dna while it is running (for example as an MCP server started by Claude, Cursor or another AI app). On Windows the running `dna.exe` is renamed out of the way instead of failing with "the file is being used by another process"; on Linux and macOS the new binary is renamed over the old one instead of failing with "Text file busy".

## [1.1.0] - 2026-11-19

### Added

- A terminal UI for people, only when a person is at a terminal:
  - tailored screens for `domain search`, `domain bulk-search`, `domain list`, `domain info`, `dns list` and `account get`
  - colored statuses, relative expiry dates, money formatting and tables that fit the terminal width for every other command
  - suggested next commands, readable error blocks, a confirmation panel for billable and destructive commands and a readable `--dry-run`
  - a spinner while requests run, colored `--help` and syntax-colored JSON
  - the Domain Name API brand colors (logo green `#1CCC99`, navy `#212A4D`, slate) and a "DOMAIN NAME API" ASCII logo
- An interactive shell when you run `dna` on its own in a terminal, usable without memorising commands:
  - quick actions under an empty prompt (↓): your domains, register, DNS records, balance, environment, AI agent setup
  - live availability and prices across popular extensions while you type a name; ↓ picks a result to register it
  - a domain browser with search and a menu per domain: details, DNS, renew, lock, WHOIS privacy, nameservers, auth code
  - a searchable picker of your domains wherever a domain is needed, and completion of your domain names
  - short forms for registering, renewing, adding DNS records, nameservers and transfers, with prices
  - a confirmation box for billable and destructive steps, built from a dry run of the exact request (Cancel by default)
  - a `/` command menu, environment switching, a status line with environment and balance, history; Esc goes back
  - Connect an AI agent (`/agent`): 20+ agents to choose from, environment and access level, the exact setup with the
    absolute dna path, one-step setup where possible (`claude mcp add`, `code --add-mcp`, Cursor install link, or the
    agent's config file with a backup), a credentials check and ready-made prompts copied to the clipboard
- `dna mcp config` knows 20+ clients (Zed, Cline, Roo Code, Continue, Goose, opencode, Amazon Q, JetBrains, LM Studio,
  Warp, Copilot CLI, ...), gains `--full-path`, writes JSON with `command` before `args`, and an unknown `--client` is
  now a usage error (exit code 2)
- Agent mode: when `AI_AGENT`, `CLAUDECODE`, `GEMINI_CLI`, `CODEX_SANDBOX`, `CURSOR_AGENT` or `DNA_AGENT=1` is set, dna prints JSON, never prompts and shows no spinner, even in a pseudo-terminal. `DNA_AGENT=0` turns detection off.
- `FORCE_COLOR` / `CLICOLOR_FORCE` for colored `-o table` output on a pipe (JSON is never colored on a pipe).

### Changed

- README rewritten around AI agents, with hand-made animated SVGs (generated by `scripts/readme-assets`), translated into 9 languages (EN, TR, DE, AR, NL, AZ, ZH, FR, IT), including the animations themselves and a language switcher with animated buttons.
- "Ready-made prompts for Claude and ChatGPT" in every README language, the same prompts as the domainreseller profile, and stable `#connect-your-ai-agent` / `#ready-made-prompts` anchors.
- An "interactive shell" section with its own animation in every README language, a tip for connecting an AI agent from the shell, and a `dna` row in the command table.

## [1.0.0] - 2026-11-19

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
