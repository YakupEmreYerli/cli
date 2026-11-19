# Errors and exit codes

## Exit codes

Exit codes are part of the CLI's contract and do not change between minor versions.

| code | name | meaning | typical cause |
|---:|---|---|---|
| 0 | ok | success | |
| 1 | error | unexpected error | bug, unreadable config file |
| 2 | usage | invalid flags, arguments or input; **nothing was sent** | missing required field, bad enum value, unknown flag, bad `--jq` |
| 3 | auth | authentication or authorization failed | wrong key, wrong environment, unknown/suspended reseller ID, no credentials |
| 4 | not_found | resource not found | wrong handle, order code |
| 5 | rejected | the API refused the request | validation failure, business rule (domain locked, not renewable, insufficient balance, …) |
| 6 | rate_limited | still rate limited after automatic retries | burst of requests |
| 7 | server | API server error (5xx) | registry or backend failure, may be transient |
| 8 | network | network failure or timeout | DNS, TLS, proxy, connection reset |
| 9 | confirmation_required | billable/destructive command without `--yes` in a non-interactive session, or declined at the prompt | |
| 10 | read_only | blocked by read-only mode | `--read-only`, `DNA_READ_ONLY`, read-only profile |

## Error output

When the output is not a table (piped, `-o json`, …), errors are printed to **stderr** as one JSON object:

```json
{
  "error": {
    "kind": "validation",
    "code": "Dna.DomainService:Domain:1018",
    "message": "Invalid forward type",
    "status": 400,
    "details": "…",
    "fields": [{ "field": "forwardType", "message": "…" }],
    "correlationId": "2f815e94e8b84b74a11ab47caa42b49f",
    "hint": "…",
    "retryable": false,
    "exitCode": 5
  }
}
```

On a terminal (and not under an AI agent) the same information is shown as a short colored block: a headline, the
message, the API code and HTTP status, the fields to fix and the suggested next step (`→`).

| field | meaning |
|---|---|
| `kind` | `auth`, `forbidden`, `not_found`, `validation`, `conflict`, `rate_limited`, `server`, `network`, `timeout`, `invalid_input`, `confirmation_required`, `declined`, `read_only`, `usage`, `error` |
| `code` | the API's error code when it sends one, otherwise a code from the table below |
| `status` | HTTP status (absent for errors raised before sending) |
| `fields` | per-field problems: CLI validation (exit 2) or the API's `validationErrors` |
| `correlationId` | the gateway's `x-correlation-id`; quote it when contacting support |
| `retryable` | whether repeating the identical request later can succeed |

## Codes set by dna

| code | meaning | fix |
|---|---|---|
| `NO_CREDENTIALS` | no reseller ID / API key configured | `dna auth login`, or set `DNA_RESELLER_ID` and `DNA_API_KEY` |
| `INVALID_RESELLER` | the gateway redirected to `/Error/Suspend`, so the `__reseller` value is unknown, inactive or suspended **in this environment** | check the reseller ID and `--env` |
| `INVALID_API_KEY` | the key does not match the reseller | production and OTE keys differ; check `--env` |
| `MISSING_API_KEY`, `MISSING_RESELLER_ID` | a header was empty | configure credentials |
| `SUBRESELLER_NOT_ALLOWED` | sub-reseller accounts cannot use the API | use the main reseller's credentials |
| `UNAUTHORIZED` | other 401 | |
| `RATE_LIMITED` | 429 after retries | slow down; see [usage.md](usage.md#rate-limits-quota-and-retries) |
| `HTTP_<status>` | the API sent no error code | see `message` / `details` |

API error codes look like `Dna.DomainService:Domain:10007`. They come from the platform's error catalogue and identify
the exact reason (domain not found, TLD not supported, …), so branch on them rather than on the message text.

## Troubleshooting

| symptom | what to check |
|---|---|
| exit 3, `INVALID_RESELLER` right after `auth login --env ote` | the reseller ID must exist in OTE; ask support to enable OTE for your account |
| exit 3, `INVALID_API_KEY` | you are using the production key on OTE or vice versa: `dna auth status` shows where the key comes from |
| exit 9 in a script or agent | expected for billable/destructive commands; add `--yes` after review, or `--dry-run` first |
| exit 8 on a billable command | the request may have reached the API. Check state (`dna domain info <d>`, `dna account transactions`) before retrying |
| HTTP 5xx with a correlation id | retry later; if it persists, contact support with the correlation id |
| a request feels slow | the soft quota may be delaying it. `-v` shows `quota … delayed Xs`; points recover within minutes |
| anything else | `dna doctor` checks config, credentials, network and authentication |
