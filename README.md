# proto-microdata

A **protobuf type system for schema.org**, generated directly from the
[schema.org](https://schema.org) vocabulary, plus extractors that turn web-page
structured data into those typed messages.

schema.org data appears on the web in three syntaxes — **JSON-LD** (dominant
today), **Microdata**, and RDFa — but they all encode the *same*
vocabulary. proto-microdata generates **one** protobuf type system (a message per
schema.org Type) from the vocabulary, and reads each syntax into it. Input scope:
**JSON-LD first, then Microdata.**

```
schema/schemaorg-all-https.jsonld        (vendored schema.org v30.0)
        │  cmd/genast   (build a gluon AST, lower it with gluon's compiler)
        ▼
proto/schemaorg.proto + proto/schemaorg.fdset     (~1841 committed messages)
        │
   ┌────┴────┐
   ▼         ▼
 JSON-LD   Microdata     → typed Schema<Type> messages
```

Unlike the grammar-driven sibling projects (proto-svg, proto-html), there is no
EBNF and no string-manipulation pipeline: schema.org's vocabulary is already
structured, so the type system is built as **gluon proto ASTs directly**. See
[`CLAUDE.md`](CLAUDE.md) for the architecture and
[`docs/`](docs/) for the schema.org and microdata reference digests.

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
