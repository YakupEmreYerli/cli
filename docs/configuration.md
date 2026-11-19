# Configuration and credentials

## Credentials

The API authenticates every request with two headers that `dna` adds for you:

| header | value |
|---|---|
| `X-API-KEY` | your API key |
| `__reseller` | your reseller ID (UUID) |

Find both in the reseller panel under **API settings**. **Production and OTE use different API keys.** Using the
production key against OTE (or the other way round) fails with exit code 3 (`INVALID_API_KEY` or `INVALID_RESELLER`).

## Environments

| environment | base URL | select with |
|---|---|---|
| production (default) | `https://api.domainresellerapi.com` | nothing, `--env production`, `DNA_ENV=production` |
| OTE (test) | `https://ote.domainresellerapi.com` | `--ote`, `--env ote`, `DNA_ENV=ote`, `DNA_TEST_MODE=true` |

OTE is a separate deployment with its own data and balance. Use it for development, CI and AI experiments.
`DNA_TEST_MODE` matches the convention of the other DomainNameAPI SDKs (php-dna, dotnet-dna); it accepts
`true/1/on/yes`.

## Ways to provide credentials

### 1. `dna auth login` (interactive machines)

```bash
dna auth login              # production -> profile "production"
dna auth login --env ote    # OTE        -> profile "ote"
dna auth login --profile acme --env ote   # any profile name
```

The key is read without echo, verified with a real call (`account get`) and only then saved. Use `--no-verify` to skip
the check. For automation, read the key from stdin so it never appears in shell history or the process list:

```bash
printf '%s' "$DNA_OTE_KEY" | dna auth login --env ote --reseller-id "$RESELLER_ID" --api-key-stdin
```

### 2. Environment variables (CI, containers, MCP servers)

```bash
export DNA_RESELLER_ID=1b2c3d4e-...
export DNA_API_KEY=...
export DNA_ENV=ote
```

### 3. Flags (avoid)

`--reseller-id` and `--api-key` exist for one-off debugging. Flags end up in shell history.

## Profiles

Profiles live in a JSON config file:

| OS | default path |
|---|---|
| Linux | `$XDG_CONFIG_HOME/dna/config.json` (usually `~/.config/dna/config.json`) |
| macOS | `~/Library/Application Support/dna/config.json` |
| Windows | `%AppData%\dna\config.json` |

Override the path with `--config` or `DNA_CONFIG`. `dna config path` prints it.

```json
{
  "current_profile": "production",
  "profiles": {
    "production": { "environment": "production", "reseller_id": "1b2c...", "api_key": "..." },
    "ote":        { "environment": "ote",        "reseller_id": "1b2c...", "api_key": "...", "read_only": true }
  }
}
```

The file is written with owner-only permissions (`0600`) because it contains API keys. Do not commit it. On shared
machines prefer environment variables from a secret manager.

```bash
dna config show                       # profiles (keys masked) + effective settings with their sources
dna config use ote                    # make "ote" the default profile
dna config set read_only true --profile ote
dna auth logout --profile ote         # delete the profile
```

Settable keys: `environment`, `reseller_id`, `api_key`, `base_url`, `read_only`.

## Precedence

For every value the first match wins:

1. command-line flag (`--api-key`, `--reseller-id`, `--env`, `--base-url`, `--read-only`)
2. environment variable (`DNA_API_KEY`, `DNA_RESELLER_ID`, `DNA_ENV` / `DNA_TEST_MODE`, `DNA_BASE_URL`, `DNA_READ_ONLY`)
3. the selected profile
4. defaults (production)

The **profile** is selected by `--profile`, then `DNA_PROFILE`, then, **when an environment is forced** with
`--env`/`--ote`/`DNA_ENV`/`DNA_TEST_MODE`, the profile named after it (`ote` or `production`), then `current_profile`,
then `production`. So after `dna auth login --env ote`, `dna --ote …` picks the OTE key automatically.

If you force an environment onto a profile configured for the other one (`--profile production --env ote`), `dna`
warns that the key will most likely be rejected.

`dna auth status` (and `dna config show`) shows every effective value and where it came from:

```console
$ dna --ote auth status
profile:       ote  (flag --env)
environment:   ote  (flag --env)
baseUrl:       https://ote.domainresellerapi.com  (environment ote)
resellerId:    1b2c3d4e-...  (profile ote)
apiKey:        ********a1b2  (profile ote)
readOnly:      true  (profile ote)
authenticated: true
```

## Read-only mode

Read-only mode refuses every operation whose safety class is not `read` (exit code 10), before anything is sent.
`dna api` allows only GET requests in this mode.

- per command: `--read-only`
- per shell / process: `DNA_READ_ONLY=1`
- per profile: `dna config set read_only true --profile <name>`. This is a good choice for a profile you hand to an AI agent.

## All environment variables

| variable | meaning |
|---|---|
| `DNA_RESELLER_ID` | reseller ID (UUID) |
| `DNA_API_KEY` | API key |
| `DNA_ENV` | `production` or `ote` |
| `DNA_TEST_MODE` | `true` selects OTE (compatibility with the other SDKs) |
| `DNA_PROFILE` | profile name |
| `DNA_BASE_URL` | custom base URL (proxies, tests) |
| `DNA_CONFIG` | config file path |
| `DNA_OUTPUT` | default output format (`json`, `table`, …) |
| `DNA_READ_ONLY` | `1` blocks every change |
| `NO_COLOR` | disable colours |
| `HTTPS_PROXY`, `NO_PROXY` | standard proxy settings |
