#!/usr/bin/env sh
# Install the dna CLI from the latest GitHub release (Linux and macOS).
#
#   curl -fsSL https://raw.githubusercontent.com/domainreseller/cli/main/scripts/install.sh | sh
#
# Environment:
#   DNA_VERSION      version to install, e.g. 0.2.0 (default: latest)
#   DNA_INSTALL_DIR  target directory (default: /usr/local/bin if writable, else ~/.local/bin)
set -eu

REPO="domainreseller/cli"

say() { printf '%s\n' "$*" >&2; }
fail() { say "error: $*"; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }

need curl
need tar
need uname

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux|darwin) ;;
  *) fail "unsupported OS $os (on Windows use scripts/install.ps1)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $arch" ;;
esac

version="${DNA_VERSION:-}"
if [ -z "$version" ]; then
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p' | head -n 1)
  [ -n "$version" ] || fail "could not determine the latest release"
fi
version="${version#v}"

archive="dna_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/v${version}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "Downloading dna $version ($os/$arch)..."
curl -fsSL -o "$tmp/$archive" "$base/$archive" || fail "download failed: $base/$archive"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" || fail "could not download checksums.txt"

expected=$(grep " $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || fail "$archive is not listed in checksums.txt"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$archive" | awk '{print $1}')
fi
[ "$expected" = "$actual" ] || fail "checksum mismatch for $archive"

tar -xzf "$tmp/$archive" -C "$tmp" dna

dir="${DNA_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin; else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir"
# Copy next to the target, then rename over it: a rename also works while dna
# is running (an MCP server started by an AI app), where writing into the
# running file would fail with "Text file busy".
cp "$tmp/dna" "$dir/.dna.new" && chmod 0755 "$dir/.dna.new" && mv -f "$dir/.dna.new" "$dir/dna" \
  || { rm -f "$dir/.dna.new"; fail "could not write $dir/dna (set DNA_INSTALL_DIR to a directory you can write)"; }

say "Installed $("$dir/dna" version 2>/dev/null | head -n 1) to $dir/dna"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) say "Note: $dir is not on your PATH. Add it, e.g.: export PATH=\"$dir:\$PATH\"" ;;
esac
say "Next: dna auth login --env ote"
