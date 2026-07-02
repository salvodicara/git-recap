#!/usr/bin/env sh
# Install git-recap: downloads the latest prebuilt release binary for your
# platform (checksum-verified). No Go toolchain needed.
#
#   curl -fsSL https://raw.githubusercontent.com/salvodicara/git-recap/main/install.sh | sh
#
# Override the destination with PREFIX, e.g. PREFIX=/usr/local/bin
set -eu

REPO="salvodicara/git-recap"
PREFIX="${PREFIX:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
arch=$(uname -m)
case "$arch" in
  x86_64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "error: unsupported architecture $arch — try: go install github.com/$REPO@latest" >&2; exit 1 ;;
esac
case "$os" in
  darwin | linux) ;;
  *) echo "error: unsupported OS $os — Windows zips: https://github.com/$REPO/releases" >&2; exit 1 ;;
esac

asset="git-recap_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/latest/download"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $asset..."
curl -fsSL "$base/$asset" -o "$tmp/$asset"
curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"

if command -v shasum >/dev/null 2>&1; then
  sum() { shasum -a 256 -c -; }
else
  sum() { sha256sum -c -; }
fi
(cd "$tmp" && grep " $asset\$" checksums.txt | sum >/dev/null) || {
  echo "error: checksum verification failed" >&2
  exit 1
}

tar -xzf "$tmp/$asset" -C "$tmp" git-recap
mkdir -p "$PREFIX"
mv "$tmp/git-recap" "$PREFIX/git-recap"
echo "Installed $PREFIX/git-recap ($("$PREFIX/git-recap" version))"

# Completions: fish auto-loads from its user dir, so install those directly;
# bash/zsh rc files are yours — we only print the line to add.
if command -v fish >/dev/null 2>&1; then
  fish_dir="$HOME/.config/fish/completions"
  mkdir -p "$fish_dir"
  "$PREFIX/git-recap" completion fish > "$fish_dir/git-recap.fish" && echo "Installed fish completions."
fi
echo "Completions (bash/zsh): add to your shell rc — see \`git-recap completion\` or the README."

case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) echo "note: $PREFIX is not on your PATH — add it to use \`git-recap\` / \`git recap\`." ;;
esac
