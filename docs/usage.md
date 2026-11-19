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
| `auto` (default) | `table` on a terminal, `json` when piped |
| `json` | the API response, unchanged (pretty on a terminal, compact when piped) |
| `jsonl` | one JSON value per line, for list responses |
| `table` | aligned columns for lists, key/value view for objects; enum numbers become names, dates are shortened |
| `raw` | the response bytes exactly as received |

Set a default with `DNA_OUTPUT=json`. Commands that return nothing print `{"success":true}` (`OK` in table mode).

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
| `--dry-run` | validate, then print the HTTP request (method, URL, headers with the key masked, body) instead of sending it |
| `--yes` / `-y` | confirm billable/destructive commands without a prompt |
| `--read-only` | refuse anything that is not a read (also `DNA_READ_ONLY=1`, or a read-only profile) |

Without a terminal and without `--yes`, billable and destructive commands exit with code 9. A script cannot
accidentally buy something.

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

# Save the issued certificate
dna ssl content example.com --jq .certificate > example.crt
```
