# proto-schemaorg

A **grammar and protobuf type system for schema.org**, generated directly from
the [schema.org](https://schema.org) vocabulary, plus extractors that turn
web-page structured data into typed messages.

schema.org data appears on the web in three syntaxes — **JSON-LD** (dominant
today), **Microdata**, and RDFa — but they all encode the *same* vocabulary.
proto-schemaorg generates **one** grammar/type system (a production and message
per schema.org Type) from the vocabulary, and reads each syntax into it. Input
scope: **JSON-LD first, then Microdata.**

```
schema/schemaorg-all-https.jsonld        (vendored schema.org v30.0)
        │  cmd/genast   (build a gluon grammar AST; project ↓)
        ├──────────────┬─────────────────┐
        ▼              ▼                 ▼
proto/schemaorg.proto  proto/….fdset   lang/schemaorg.ebnf
  (~1841 messages)                      (readable grammar view)
        │
   ┌────┴────┐
   ▼         ▼
 JSON-LD   Microdata     → typed Schema<Type> messages
```

The **grammar is central** — each Type is a production and each property is a
reference to its range production (the cross-entity links). It's built as a
gluon AST rather than round-tripped through EBNF text and the markup-terminal
compile machinery the grammar-driven siblings (proto-svg, proto-html) need to
re-emit markup — schema.org's vocabulary is already structured, so that
machinery buys nothing here. See [`CLAUDE.md`](CLAUDE.md) and
[`docs/decisions/0001-grammar-role.md`](docs/decisions/0001-grammar-role.md).

## Quick start

```sh
./build.sh            # setup + regenerate proto/schemaorg.* + go build
./tools/genast.sh -all   # regenerate the full type system on demand
```

Requires Go 1.26 and the sibling checkouts `../gluon` and `../proto-merge`
(cloned automatically by `setup.sh`).

## Status

- ✅ schema.org vocabulary → committed protobuf type system (all ~918 types).
- ⏳ JSON-LD extractor, then Microdata extractor (`service/`).

Built with [gluon](https://github.com/accretional/gluon) and the
[accretional](https://github.com/accretional) proto ecosystem.

## License

MIT — see [LICENSE](LICENSE).
