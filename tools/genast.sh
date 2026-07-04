#!/usr/bin/env bash
# genast.sh — regenerate the committed schema.org protobuf type system from the
# vendored JSON-LD vocabulary by building a gluon AST and lowering it with
# gluon's compiler. This is the ONLY sanctioned way to (re)generate proto/.
#
# By default it emits the curated-core closure (fast, human-scannable). Pass
# extra flags through, e.g. `./tools/genast.sh -all` for the full vocabulary.
set -euo pipefail
cd "$(dirname "$0")/.."

go run ./cmd/genast \
  -jsonld schema/schemaorg-all-https.jsonld \
  -proto proto/schemaorg.proto \
  -fdset proto/schemaorg.fdset \
  -package schemaorg \
  -go-package 'github.com/accretional/proto-microdata/proto/pb/schemaorg;schemaorgpb' \
  "$@"
