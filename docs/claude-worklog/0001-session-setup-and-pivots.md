# Worklog 0001 — Session setup, architecture, and the pivots that shaped it

Date: 2026-07-03

## What this repo is (final, after several course-corrections this session)

proto-microdata turns the **schema.org vocabulary** into a **protobuf type
system** (one message per schema.org Type), and extracts schema.org structured
data from web pages into those typed messages. The vocabulary is
syntax-independent, so a single generated type system serves multiple input
syntaxes; we target **JSON-LD first, then Microdata**.

```
schema/schemaorg-all-https.jsonld   (vendored vocab v30.0 = source of truth)
        │  cmd/genast   (build a gluon AST in memory, lower with gluon compiler)
        ▼
proto/schemaorg.proto + proto/schemaorg.fdset   (COMMITTED; ~1841 messages, -all)
        │
   ┌────┴────┐  runtime front-ends (share the one type system)
   ▼         ▼
 JSON-LD   Microdata          → typed schema.org proto messages
 extractor  extractor (WHATWG DOM walk)
```

## The pivots (why the repo looks the way it does)

The task started as "encode schema.org Microdata like xmile handles RSS." Over
the session the approach was corrected several times by the user — recorded here
so the history isn't mysterious:

1. **Start:** modelled on xmile/proto-sitemap — a format spec compiled on demand
   (the `rss-2.0.ebnf` / `sitemap.ebnf` pattern). Scaffolded that way first.
2. **"use the proto-svg / proto-html approach instead":** proto-svg makes an
   EBNF grammar the source of truth and *commits* a generated proto type system
   (via gluon `genproto`). Rebased onto that: chose (via a design question)
   **generate the grammar from the schema.org vocabulary** rather than
   hand-author 800 types.
3. **"I don't like this genproto approach — janky string manipulation… use
   proto asts / gluon asts":** dropped the EBNF-text pipeline entirely
   (StripKeywords, prefix maps, scalarize — all the markup-terminal machinery
   proto-svg needs to re-emit SVG). schema.org's vocabulary is *already*
   structured, so we build the gluon AST **directly** and lower it with
   `compiler.Compile`. No grammar text, no string munging.
4. **"what's the scoop on json-ld — should we just do that instead?":** the live
   corpus proved microdata is largely extinct (most sites moved to JSON-LD).
   Because the proto type system is syntax-independent, adding JSON-LD costs
   nothing in the generator. Decision: **JSON-LD primary + Microdata**, both over
   the one shared type system. (Repo kept named proto-microdata for now; the
   generated package is honestly named `schemaorg`.)

Also this session: pushed proto-html's `html-grammar` branch (6 commits, the
real "HTML as a formal grammar" work) up to its `main` (was a bare README),
per request — clean fast-forward.

## How genast works (cmd/genast/main.go)

- Parse the JSON-LD `@graph`: classes (`rdfs:Class`), properties
  (`rdf:Property` with `schema:domainIncludes` / `schema:rangeIncludes`), and
  the DataType set (DataType + everything transitively `subClassOf` it).
- Filter to schema.org-native identifiers — the `-all` dump references external
  vocabularies (`bibo:issue`, `dcterms:…`) whose names carry a colon and are not
  valid proto identifiers; those are dropped.
- Per Type T: message `Schema<T>` with an `id` field (the item's @id / itemid)
  plus one **repeated** field per applicable property — the type's own domain
  **plus every ancestor's** (flattened inheritance), which is exactly what an
  item of that type may carry.
- Per property P: datatype-only range → `repeated string`; a range that admits a
  nested item → a wrapper message named after P holding a `oneof` over its class
  ranges (+ a `text` arm when a DataType is also allowed).
- Key gluon fact: `compiler.Compile` derives a field's **name** and its message
  **type** from the *same* referenced nonterminal name (snake/pascal of one
  string). So the AST references types by their exact rule name and references
  always resolve, regardless of gluon's internal casing.
- Root: `SchemaOrgDocument { repeated TopLevelItem }`, `TopLevelItem` = a oneof
  over every included `Schema<T>`.

Closure: `-all` includes every class (918 types → 1841 messages). Without it,
the closure grows from a curated core (`defaultRoots`); a `-max` cap collapses
overflow classes to the generic `SchemaThing`. Out-of-closure ranges always
collapse to `SchemaThing`, so the graph is closed either way.

## Verified this session

- `go run ./cmd/genast` (core): 230 types → 823 messages.
- `go run ./cmd/genast -all`: 936 classes / 1529 native properties → 918 types →
  **1841 messages**; `proto/schemaorg.{proto,fdset}` written (fdset 2.6 MB);
  `descriptor.ToString` clean; `go build ./...` and `go vet ./cmd/...` pass.
- Spot checks: `SchemaPerson` flattens Thing's props; `address` → `Address {
  oneof { SchemaPostalAddress; string text } }`; `SchemaRecipe` has cook_time,
  nutrition→`Nutrition`, recipe_ingredient.

## Test corpora (built by subagents this session)

- `testing/testdata/synthetic/` — **14 hand-authored HTML+JSON pairs** covering
  the WHATWG microdata algorithm exhaustively (property-value cascade, itemref,
  itemref cycles, multi-token itemprop/itemtype, itemid, nesting, composites).
  12/14 match the `extruct` reference parser exactly; the 2 differences are
  places where extruct is non-conformant and our fixtures follow the spec (see
  that dir's README — 011 cycle ERROR/tree-order, 013 missing-attr → "").
- `testing/testdata/live/` — **11 confirmed real pages** with in-DOM microdata
  (StackOverflow QAPage, Gutenberg Book/Offer, OpenLibrary, schema.org docs, …).
  Big finding: microdata is largely extinct on mainstream sites (~25+ candidates
  discarded for having only JSON-LD) — this is *why* JSON-LD became primary.

## Reference docs

- `docs/MICRODATA_SPEC.md` — the WHATWG microdata algorithm digest (extraction
  roots, property crawl, value cascade, itemref, JSON model, cycle guards).
- `docs/SCHEMAORG_VOCAB.md` — schema.org v30.0 distribution, the JSON-LD record
  structure, counts, and the generator sketch this repo implements.

## Next steps

1. **JSON-LD extractor** (`service/jsonld/`): parse `<script type="application/
   ld+json">` blocks → resolve `@type`/`@id`/`@context` → fill the generated
   `Schema<T>` messages (dynamicpb against schemaorg.fdset). Highest ROI.
2. **Microdata extractor** (`service/microdata/`): the WHATWG DOM walk over an
   HTML parse (golang.org/x/net/html) → the same messages. Gate against the
   synthetic corpus (it is the ground truth).
3. **Go bindings:** generate `proto/pb/schemaorg/*.pb.go` from the fdset (add
   protoc to setup.sh) if we want static types rather than dynamicpb.
4. Wire a corpus runner into `test.sh` (extract every synthetic fixture, compare
   to its `.json`; report over the live corpus).
5. Consider renaming the repo to `proto-schemaorg` (the generated type system is
   schema.org, not microdata-specific) — deferred; module path unchanged for now.
6. AST-walked + fuzzed testsets (as originally requested) once an extractor exists.
```

## Build discipline

Everything goes through `setup.sh` → `build.sh` → `test.sh` → `LET_IT_RIP.sh`
(strict superset chain). `proto/schemaorg.{proto,fdset}` are generated by
`tools/genast.sh` (run by build.sh, default `-all`) — never hand-edited.
