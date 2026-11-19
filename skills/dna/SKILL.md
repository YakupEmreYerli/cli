---
name: dna
description: Manage domains, DNS records, DNSSEC, SSL certificates and hosting through the DomainNameAPI reseller API with the dna CLI. Use when the user wants to search, register, renew or transfer domains, change nameservers or DNS records, check balances, prices or TLD rules, or manage SSL/hosting on DomainNameAPI (domainresellerapi.com).
---

# dna CLI - guide for AI agents

dna drives the DomainNameAPI reseller API (domains, DNS, DNSSEC, SSL, hosting).
This guide is generated from the CLI itself, so it matches the installed version.

## Rules

1. **Environment.** `--env ote` (or `--ote`) is the test environment, the default is production (real money).
   Prefer OTE while experimenting. `dna auth status -o json` shows what is in effect.
2. **Output.** Piped output is JSON. Pass `-o json` explicitly to be safe; use `--jq` to select fields
   (`--jq .items[].domainName`). Errors are a JSON object `{"error":{...}}` on stderr.
   The colored terminal UI is for people only: when `AI_AGENT`, `CLAUDECODE` or `DNA_AGENT=1` is set, dna prints
   JSON, never prompts and shows no spinner, even in a pseudo-terminal.
3. **Safety classes.** Every command is `read`, `write`, `destructive` or `billable` (see the table).
   Billable and destructive commands fail with exit code 9 unless `--yes` is given.
   **Never add --yes on your own:** show the user the plan (`--dry-run` prints the exact request), get explicit approval, then re-run with `--yes`.
4. **Read-only mode.** `--read-only` or `DNA_READ_ONLY=1` blocks every change (exit code 10). Use it for exploration.
5. **Inputs.** Positional args and flags are listed in `dna <command> --help`. `dna schema <operation>` prints the full
   JSON Schema plus an example; the same JSON can be passed with `--body @file.json` or `--body -` (stdin).
   Inputs are validated before anything is sent (exit code 2 with a field list).
6. **Coded values.** Statuses and types are numbers in responses; `dna enums <Name>` explains them. Flags accept names
   (`--dnssec-mode DisableDnssec`, `--status-ids Active`).
7. **Lists** are paged: `--max-result-count` / `--skip-count`, or `--all` for everything.
8. **Retries.** The CLI already retries rate limits (429) and transient errors on reads. Never blindly retry a billable
   command after a timeout or exit code 8: check the result first (e.g. `dna domain info <domain>`).
9. **Typical flows**
   - Register: `domain search` -> (`contact create` if needed) -> `domain register --dry-run` -> approval -> `--yes`.
   - Renew: `domain renew-check` -> approval -> `domain renew --period N --yes`.
   - Transfer in: `domain transfer-check <domain> <authcode>` -> approval -> `domain transfer --yes` -> `domain transfer-status`.
   - DNS: `dns list`, `dns add --name www --type A --content 192.0.2.1`, then `dns apply` if the domain is not on the platform nameservers.
   - SSL: `product ssls` -> `ssl initialize` (billable) -> `ssl generate-csr` -> `ssl configure` -> `ssl status` -> `ssl content`.

## Exit codes

| code | name | meaning |
|---:|---|---|
| 0 | ok | Success |
| 1 | error | Unexpected error |
| 2 | usage | Invalid flags, arguments or input; nothing was sent to the API |
| 3 | auth | Authentication or authorization failed (bad API key, unknown or suspended reseller ID, wrong environment) |
| 4 | not_found | The resource does not exist |
| 5 | rejected | The API rejected the request (validation error or business rule) |
| 6 | rate_limited | Rate limited even after automatic retries |
| 7 | server | API server error (5xx); may be transient |
| 8 | network | Network failure or timeout |
| 9 | confirmation_required | Billable/destructive operation needs --yes (or was declined at the prompt) |
| 10 | read_only | Blocked because read-only mode is on |

Error JSON (stderr): `{"error":{"kind","code","message","status","fields","hint","correlationId","retryable","exitCode"}}`

## Commands

| command | safety | what it does |
|---|---|---|
| `dna account get` | read | Show the reseller account and its TRY/USD balances |
| `dna account transactions` | read | List balance transactions (charges, refunds, deposits) |
| `dna contact create` | write | Create a reusable contact handle |
| `dna contact get <handle>` | read | Show a contact handle and its verification state |
| `dna contact resend-verification <handle>` | write | Send the contact verification notice again |
| `dna contact update <handle>` | write | Update a contact handle (and the domains that use it) |
| `dna contact verification <handle>` | read | Show e-mail/phone verification status of a contact |
| `dna dns add <domain>` | write | Add values to a DNS record set (creates the set if needed) |
| `dna dns apply <domain>` | write | Switch the domain to the platform nameservers so its DNS zone is served |
| `dna dns delete <domain>` | destructive | Remove one value from a DNS record set |
| `dna dns list <domain>` | read | List the DNS records of a domain |
| `dna dns update <domain>` | write | Replace a DNS record set with new values |
| `dna dnssec add <domain>` | write | Publish DS records at the registry (enable DNSSEC) |
| `dna dnssec disable <domain>` | destructive | Remove every DS record (disable DNSSEC) |
| `dna dnssec get <domain>` | read | Show DNSSEC status, DS records and allowed algorithms |
| `dna dnssec remove <domain>` | destructive | Remove one DS record |
| `dna domain bulk-search <domains>...` | read | Check availability and prices of several domains at once |
| `dna domain contacts set-handles <domain>` | write | Point a domain's contact roles at existing contact handles |
| `dna domain contacts update <domain>` | write | Replace a domain's contacts with inline contact details |
| `dna domain documents list <domain>` | read | List registry documents of a domain (mostly .tr) |
| `dna domain documents upload <domain>` | write | Upload registry documents for a domain |
| `dna domain forward delete <domain>` | destructive | Remove the URL forwarding of a domain |
| `dna domain forward get <domain>` | read | Show the URL forwarding of a domain |
| `dna domain forward set <domain> <url>` | write | Forward a domain to a URL |
| `dna domain host add <domain> <host>` | write | Create a child nameserver (glue record) such as ns1.example.com |
| `dna domain host delete <domain> <host>` | destructive | Delete a child nameserver |
| `dna domain host update <domain> <host>` | write | Change a child nameserver's name or IP addresses |
| `dna domain info <domain>` | read | Show full details of a domain |
| `dna domain list` | read | List your domains |
| `dna domain lock <domain>` | write | Enable the transfer lock |
| `dna domain nameservers set <domain> <nameserver>...` | write | Replace the nameservers of a domain |
| `dna domain privacy <domain> <on|off>` | write | Turn WHOIS privacy protection on or off |
| `dna domain register <domain>` | billable | Register a domain using existing contact handles (charges balance) |
| `dna domain register-with-contacts <domain>` | billable | Register a domain with inline contact details (charges balance) |
| `dna domain renew <domain>` | billable | Renew a domain (charges balance) |
| `dna domain renew-check <domain>` | read | Check whether a domain can be renewed and at what price |
| `dna domain restore <domain>` | billable | Restore a deleted domain from redemption (charges balance) |
| `dna domain search <domain>` | read | Check if a domain is available and get its price |
| `dna domain transfer <domain>` | billable | Transfer a domain in from another registrar (charges balance) |
| `dna domain transfer-check <domain> <auth-code>` | read | Check whether a domain can be transferred in with an auth code |
| `dna domain transfer-status <domain>` | read | Show the state of a pending transfer |
| `dna domain unlock <domain>` | write | Disable the transfer lock (needed before transferring out) |
| `dna domain update-auth-code <domain>` | write | Update the transfer auth (EPP) code and return it |
| `dna hosting create <domain>` | billable | Order a hosting for a domain (charges balance) |
| `dna hosting get <domain>` | read | Show a hosting by domain name or service ID |
| `dna hosting get-by-token <token>` | read | Show a hosting by the token returned when it was created |
| `dna hosting packages` | read | List hosting packages and their service plans |
| `dna hosting plans` | read | List hosting service plans with their limits |
| `dna hosting status <domain>` | read | Show hosting provisioning status (and panel access once ready) |
| `dna order get <order-code>` | read | Show an order and its lines |
| `dna product info <product-name>` | read | Show the price of a product for an order type and period |
| `dna product list` | read | List products |
| `dna product ssls` | read | List SSL certificate products with prices |
| `dna product tlds` | read | List TLDs with prices, periods, rules and required attributes |
| `dna ssl approvers <domain>` | read | List e-mail addresses allowed for e-mail validation |
| `dna ssl check-csr` | read | Decode a CSR and show its subject |
| `dna ssl configure <domain>` | write | Submit CSR, validation method and contact for issuance |
| `dna ssl content <domain>` | read | Download the issued certificate and chain (PEM) |
| `dna ssl generate-csr <domain>` | read | Generate a CSR and private key |
| `dna ssl get <domain>` | read | Show certificate details (subscription, configuration, contacts) |
| `dna ssl initialize <domain>` | billable | Buy an SSL certificate package for a domain (charges balance) |
| `dna ssl reissue <domain>` | write | Reissue a certificate with a new CSR/key or approver |
| `dna ssl revoke <domain>` | destructive | Revoke a certificate (irreversible) |
| `dna ssl status <domain>` | read | Refresh and show certificate status |

Other commands: `dna auth login|status|logout`, `dna config show|use|set`, `dna api <path>` (raw request),
`dna schema [op]`, `dna enums [name]`, `dna doctor`, `dna mcp serve` (MCP server), `dna version`.
