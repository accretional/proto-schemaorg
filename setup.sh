#!/usr/bin/env bash
# setup.sh — make a clean checkout buildable with no manual steps: check the Go
# toolchain and clone the local-module dependencies as sibling repos. gluon (the
# AST compiler) and proto-merge (descriptor pretty-printer) are pinned via
# `replace => ../<dep>`, so a clean clone needs them checked out next to this
# repo. Idempotent: anything already present is updated, not re-cloned.
# Chains: build.sh -> setup.sh.
set -euo pipefail
cd "$(dirname "$0")"

export PATH="$PATH:$(go env GOPATH 2>/dev/null || echo "$HOME/go")/bin"

command -v go >/dev/null || { echo "[setup] FATAL: install Go (https://go.dev/dl/) and re-run"; exit 1; }

# Sibling dependency repos (go.mod replaces => ../<dep>). proto-json parses
# JSON-LD into the json.proto AST; gluon compiles the grammar; proto-merge
# pretty-prints descriptors.
for dep in gluon proto-merge proto-json; do
  if [ ! -d "../$dep/.git" ]; then
    echo "[setup] cloning $dep -> ../$dep"
    git clone --depth 1 "https://github.com/accretional/$dep" "../$dep"
  else
    echo "[setup] updating $dep"
    git -C "../$dep" pull --ff-only --quiet 2>/dev/null \
      || echo "[setup] WARN: $dep not fast-forwarded (diverged or local changes) — using current state"
  fi
done

go mod tidy
echo "[setup] OK"
