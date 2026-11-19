# Getting started

This walks through installing `dna`, connecting it to the **OTE test environment**, and doing a first registration
and DNS change without touching anything live.

## 1. Install

```bash
go install github.com/domainreseller/cli/cmd/dna@latest   # Go 1.26+
```

…or download a binary from the [releases page](https://github.com/domainreseller/cli/releases/latest), or run
`scripts/install.sh` / `scripts/install.ps1` (see the [README](../README.md)). Then:

```bash
dna version
dna doctor        # checks config, network and credentials
```

Enable shell completion if you like: `dna completion --help` (bash, zsh, fish, PowerShell).

## 2. Get credentials

In the DomainNameAPI reseller panel open **API settings**. You need:

| value | looks like | note |
|---|---|---|
| Reseller ID | `1b2c3d4e-....-....-....-............` | a UUID, the same for both environments' panels |
| API key (production) | random string | used against `api.domainresellerapi.com` |
| API key (OTE) | random string | a **different** key, used against `ote.domainresellerapi.com` |

Panel logins (user name / password) do not work with the API.

## 3. Log in to OTE

```bash
dna auth login --env ote
```

`dna` asks for the reseller ID and the API key (hidden), verifies them with a real API call and stores them in the
profile `ote`. Check what is in effect at any time:

```bash
dna --ote auth status
```

`--ote` is short for `--env ote`; it also selects the `ote` profile.

## 4. Explore (read-only)

```bash
dna --ote account get                          # balances
dna --ote product tlds --filter-text com       # .com rules and prices
dna --ote domain search my-test-domain.com     # available? price?
dna --ote domain list
```

Nothing here changes anything. If you want a guarantee, add `--read-only`.

## 5. Register a domain

Registering needs a contact handle (one handle can serve all four roles: registrant, admin, tech, billing).
Create one once; dna asks for each detail and saves the handle as your default contact:

```bash
dna --ote contact create
#   First name: Ada
#   Phone, with country code: +90 555 123 45 67
#   Country (2 letters) [TR]:            <- taken from +90
#   ...
# ✓ Saved D-123456789 as your default contact
```

The same works with flags (for scripts). `--phone` takes the full number, so `--phone-country-code` and
`--country` can be left out when the calling code belongs to one country:

```bash
dna --ote contact create --first-name Ada --last-name Lovelace --email ada@example.com \
  --phone "+90 555 123 45 67" --address "1 Example Street" --city Istanbul --state Istanbul --postal-code 34000
```

Already have handles on your domains? `dna --ote contact list --scan` finds them, and
`dna --ote contact default <handle>` picks the default.

Preview, then register. Without `--handle` dna uses your default contact for all four roles, and without `--ns` the
platform nameservers (tr.apiname.com, eu.apiname.com):

```bash
dna --ote domain register my-test-domain.com --period 1 --dry-run

dna --ote domain register my-test-domain.com --period 1
# Contact: D-123456789 (Ada Lovelace) for all four roles, your default contact. --handle picks another.
# BILLABLE: Register a domain ... on OTE
# Continue? [y/N]
```

Another contact for one domain: `--handle D-987654321` (all roles) or `--handle Registrant=D-1 --handle Admin=D-2 …`.
With no contact at all, dna asks at the terminal whether to create one, find the ones on your domains or use a handle
you type; scripts and agents get exit code 2 with these options instead.

Registration is **billable**. On a terminal `dna` asks for confirmation; in scripts pass `--yes`.

## 6. Manage DNS

```bash
dna --ote dns add my-test-domain.com --name www --type A --content 192.0.2.10
dna --ote dns add my-test-domain.com --name @ --type MX --content "10 mail.example.net."
dna --ote dns list my-test-domain.com
dna --ote dns apply my-test-domain.com        # switch to the platform nameservers so the zone is served
```

## 7. Go live

```bash
dna auth login                 # production key, stored in profile "production"
dna domain list                # production is the default
```

## Next

- [Configuration](configuration.md): profiles, environment variables, read-only profiles
- [Usage](usage.md): every way to pass input, output formats, jq, pagination
- [AI agents](ai-agents.md): MCP server, Claude Code skill, safe automation
