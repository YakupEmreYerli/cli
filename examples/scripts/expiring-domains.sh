#!/usr/bin/env sh
# List domains that expire within N days (default 30) and check whether each
# one can be renewed. It never renews anything: review the output, then run
# `dna domain renew <domain> --period 1` yourself.
#
#   ./examples/scripts/expiring-domains.sh 45
set -eu

DAYS="${1:-30}"

dna domain list --all -o json \
  --jq ".items[] | select(.remainingDay <= ${DAYS}) | .domainName" |
while read -r domain; do
  status=$(dna domain renew-check "$domain" --period 1 -o json \
    --jq '"renewable=\(.renewabilityStatus) premium=\(.isPremium) price=\(.premiumPrice // "-")"') || status="check failed (exit $?)"
  printf '%-40s %s\n' "$domain" "$status"
done
