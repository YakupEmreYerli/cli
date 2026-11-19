# Examples

| path | what it shows |
|---|---|
| [`scripts/expiring-domains.sh`](scripts/expiring-domains.sh) | shell + `--jq`: find expiring domains and check renewability without renewing |
| [`input/`](input) | JSON inputs for `--body @file` |

```bash
DNA_ENV=ote ./examples/scripts/expiring-domains.sh 45
dna --ote domain register-with-contacts example.com --body @examples/input/register-with-contacts.json --dry-run
```
