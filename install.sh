#!/usr/bin/env sh
# Build git-recap and place the binary on your PATH.
# Override the destination with PREFIX, e.g. PREFIX=/usr/local/bin ./install.sh
set -eu

PREFIX="${PREFIX:-$HOME/.local/bin}"
BIN="git-recap"

command -v go >/dev/null 2>&1 || { echo "error: Go toolchain not found (https://go.dev/dl/)" >&2; exit 1; }

cd "$(dirname "$0")"
echo "Building $BIN..."
go build -o "$BIN" .

mkdir -p "$PREFIX"
mv "$BIN" "$PREFIX/$BIN"
echo "Installed $PREFIX/$BIN"

case ":$PATH:" in
  *":$PREFIX:"*) ;;
  *) echo "note: $PREFIX is not on your PATH — add it to use \`git-recap\` / \`git recap\`." ;;
esac
