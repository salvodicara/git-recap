# git-recap dev tasks — run `just` to list them.
# Requires: go, git, gh (logged in). Release also needs: goreleaser.

repo := "salvodicara/git-recap"

# list available recipes
default:
    @just --list

# build the binary to ./git-recap
build:
    go build -o git-recap .

# install to your Go bin (~/go/bin)
install:
    go install .

# run from source, e.g. `just run --period week`
run *args:
    go run . {{args}}

# format the code in place
fmt:
    gofmt -w .

# fast gate: build, vet, test, formatting (offline)
check:
    go build ./...
    go vet ./...
    go test ./...
    @test -z "$(gofmt -l .)" || { echo "unformatted (run: just fmt):"; gofmt -l .; exit 1; }

# extra static analysis (downloads tools on first run)
lint:
    go run honnef.co/go/tools/cmd/staticcheck@latest ./...
    go run golang.org/x/tools/gopls/internal/analysis/modernize/cmd/modernize@latest ./...

# preview the prebuilt binaries without publishing (needs goreleaser)
release-dry:
    goreleaser release --snapshot --clean --skip=publish

# cut a release locally: tag, GitHub release, update the brew tap. No Actions
# minutes used. Assumes the tap is checked out at ../homebrew-tap.
# e.g. `just release 0.2.0`
release version:
    #!/usr/bin/env bash
    set -euo pipefail
    [ -z "$(git status --porcelain)" ] || { echo "working tree not clean — commit first"; exit 1; }
    v="v{{version}}"
    tap="$(git rev-parse --show-toplevel)/../homebrew-tap"
    formula="$tap/Formula/git-recap.rb"
    [ -f "$formula" ] || { echo "tap formula not found at $formula"; exit 1; }
    just check
    echo "==> tagging $v"
    git tag "$v"
    git push origin "$v"
    gh release create "$v" --title "$v" --generate-notes
    echo "==> bumping Homebrew formula"
    url="https://github.com/{{repo}}/archive/refs/tags/$v.tar.gz"
    sha="$(curl -fsSL "$url" | shasum -a 256 | awk '{print $1}')"
    sed -i '' \
      -e "s|archive/refs/tags/v[^\"]*\.tar\.gz|archive/refs/tags/$v.tar.gz|" \
      -e "s|sha256 \"[a-f0-9]*\"|sha256 \"$sha\"|" \
      "$formula"
    git -C "$tap" add Formula/git-recap.rb
    git -C "$tap" commit -m "git-recap $v"
    git -C "$tap" push
    echo "==> done: brew update && brew upgrade git-recap"
