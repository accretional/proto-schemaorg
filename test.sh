#!/usr/bin/env bash
# test.sh — build (regenerating the proto type system), then run the tests.
# Chains: test.sh -> build.sh -> setup.sh.
set -euo pipefail
cd "$(dirname "$0")"
./build.sh

echo "[test] go test ./..."
go test ./...
echo "[test] OK"
