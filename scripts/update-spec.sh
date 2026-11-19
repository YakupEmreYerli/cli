#!/usr/bin/env sh
# Download the Public Gateway API OpenAPI document from production and OTE,
# make sure both environments publish the same API, and store it as
# internal/openapi/openapi.json. Then regenerate and test.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

fetch() {
  curl -fsSL --retry 3 -A "DomainNameAPI-CLI-spec-update" -o "$2" "$1/swagger/v1/swagger.json"
}

fetch https://api.domainresellerapi.com "$TMP/prod.json"
fetch https://ote.domainresellerapi.com "$TMP/ote.json"

if ! cmp -s "$TMP/prod.json" "$TMP/ote.json"; then
  echo "warning: production and OTE publish different specs; using production." >&2
  echo "         Review the difference before releasing: diff the two files in $TMP" >&2
fi

if cmp -s "$TMP/prod.json" "$ROOT/internal/openapi/openapi.json"; then
  echo "spec unchanged"
  exit 0
fi

cp "$TMP/prod.json" "$ROOT/internal/openapi/openapi.json"
echo "spec updated; regenerating"
cd "$ROOT"
go test ./... || {
  echo "tests fail: add registry entries for the new operations (see docs/architecture.md)" >&2
  exit 1
}
go run ./cmd/dna gen-docs --dir .
