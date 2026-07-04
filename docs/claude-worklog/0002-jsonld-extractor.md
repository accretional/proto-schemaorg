# Worklog 0002 — JSON-LD extractor (parse via proto-json, no byte-level parsing)

Date: 2026-07-03

## What landed

`service/jsonld` — extracts schema.org entities from raw JSON-LD into the
generated `Schema<Type>` messages. `jsonld.Extract(src)`:

1. Parses JSON via **accretional/proto-json** (`jsonparse.Parse` → the
   `json.proto` AST). We do **not** parse JSON ourselves.
2. Walks that AST: top-level node(s), following `@graph`; a node's `@type`
   selects the `Schema<T>` descriptor (via `proto.MessageForType`); `@id` fills
   the `id` field; each other key is a property — a scalar goes into a datatype
   (repeated string) field, a nested node into a property-wrapper message
   (matching the wrapper's `oneof` arm by the nested node's `@type`, else the
   sole message arm; a bare string falls into the wrapper's text arm).
3. Fills messages by reflection (`dynamicpb` against `schemaorg.fdset`), so new
   vocabulary needs only a `genast` regen, no code change.

## The correction that drove this

An earlier draft used `encoding/json` + `golang.org/x/net/html`. That is exactly
the byte-level parsing this ecosystem does not do — parsing belongs to the
grammar repos. Reworked to consume proto-json instead. `service/` now imports
neither `encoding/json` nor `x/net/html`.

## proto-json change (upstream)

proto-json's parser lived in `cmd/parse` (package `main`), so it wasn't
importable. Refactored it into an importable package **`jsonparse`** with
`Parse(src) (*jsonpb.JsonText, error)` plus `StringValue`/`NumberLiteral` helpers
to read Go scalars back out of the character-level AST; `cmd/parse` is now a thin
wrapper. Committed to proto-json `main`. (Named `jsonparse`, not `parse`, because
proto-json's `.gitignore` ignores `/parse` — the built binary — which also
swallowed a `parse/` package dir.)

Note: proto-json's parser is itself a hand-written recursive descent; that is
proto-json's internal concern (it owns the JSON grammar, `lang/json.ebnf`). Our
rule is that *this* repo doesn't hand-roll parsing — it consumes proto-json.

## Tests

`service/jsonld/jsonld_test.go`: Person with nested PostalAddress (+ the
wrapper-text case for `jobTitle`, whose range DefinedTerm|Text makes it a wrapper
field), an `@graph` array, and a Product with a nested Offer. All green through
`./LET_IT_RIP.sh`.

## Deps

Added `replace github.com/accretional/proto-json => ../proto-json` and the
require; `setup.sh` now clones proto-json as a sibling.

## Next

- HTML plumbing (extract `<script type=ld+json>` blocks; the Microdata DOM walk)
  is blocked on a **proto-html** parser library — do that in proto-html, then
  wire it here. Do not reach for `x/net/html`.
- Enumerations as value constraints in the grammar; the reference overlay
  (`itemref` / `@id`); Go bindings from the fdset; corpus runner.
