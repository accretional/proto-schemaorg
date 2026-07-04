#!/usr/bin/env bash
# build.sh — set up dependencies, regenerate the schema.org proto type system
# from the vendored vocabulary (tools/genast.sh), and build everything.
# The generated proto/schemaorg.{proto,fdset} are committed; genast regenerates
# them in place. Chains: build.sh -> setup.sh.
set -euo pipefail
cd "$(dirname "$0")"
./setup.sh

echo "[build] regenerating proto/schemaorg.* (gluon AST -> descriptor)"
# Default to the FULL schema.org vocabulary (-all); pass -roots/-max to subset.
if [ "$#" -eq 0 ]; then set -- -all; fi
./tools/genast.sh "$@"

echo "[build] go build ./..."
go build ./...
echo "[build] OK"
