# Security

## Reporting a vulnerability

Please do not open a public issue for security problems. Report them privately through
[GitHub security advisories](https://github.com/domainreseller/cli/security/advisories/new). Include steps to
reproduce and the affected version (`dna version`).

## How dna handles credentials

- The API key is sent only to the selected API base URL, in the `X-API-KEY` header. It is never logged.
  - `-v` traces omit it.
  - `--dry-run` and `dna config show` mask it.
- `dna auth login` reads the key without echo, or from stdin with `--api-key-stdin`, so it stays out of shell history.
- Profiles are stored in a config file with owner-only permissions (`0600`). Treat the file as a secret. Prefer environment variables from a secret manager on shared or CI machines.
- Redirects are never followed, so credentials cannot be forwarded to another host.
- Read-only mode (`--read-only`, `DNA_READ_ONLY=1`, or a read-only profile) protects accounts that are handed to automation or AI agents.
