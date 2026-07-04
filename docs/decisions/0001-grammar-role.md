# ADR 0001 — The grammar is central; EBNF text and markup machinery are not

## Status

Accepted (2026-07-03). Supersedes the "no EBNF" framing in worklog 0001.

## Context

An earlier framing said proto-schemaorg has "no grammar / no EBNF." That was
wrong, and this record corrects it. What was actually rejected was the *janky
string-manipulation pipeline* proto-svg/proto-html need (`genproto`: markup
terminals baked into the grammar, `StripKeywords`, prefix maps, scalarize) —
machinery whose only purpose is to let a grammar **re-emit real markup** (an
`.svg` / `.html` byte stream). schema.org's vocabulary is not markup we need to
re-emit; it is a structured graph. So that machinery buys nothing here.

But a **grammar** absolutely has a role — especially for the structure that
links entities *out of the context of parsing tokens*: the subClassOf hierarchy,
the property `domain → range` references, enumerations, and cross-references
(microdata `itemref`, JSON-LD `@id`). Those relationships are the whole point of
schema.org, and they belong in a grammar, not scattered through Go.

## Decision

**The grammar is the model; we build it as a gluon AST directly and project
everything else from it.**

1. **The canonical grammar is the gluon AST** that `cmd/genast` builds
   (`pb.ASTDescriptor`). Each schema.org Type is a production; each property is a
   *reference to another production* — so `author → Person`, `address →
   PostalAddress` are literally grammar references. This is where the
   cross-entity links live. We build it in memory rather than as EBNF text
   because the vocabulary is already structured data — round-tripping through
   EBNF text (and the compile machinery that entails) would add nothing.

2. **proto/schemaorg.{proto,fdset}** is the AST lowered by gluon's compiler — the
   machine-consumable form of the grammar.

3. **lang/schemaorg.ebnf** is a generated, human-readable *projection* of the
   same AST, with the subClassOf hierarchy annotated. It makes the grammar (and
   its entity links) legible and composable; it is not re-parsed. Its EBNF
   punctuation is read from gluon's canonical `LexDescriptor`
   (`metaparser.EBNFLexV2`), not hardcoded. This projection is **interim**: gluon
   has no descriptor→EBNF serializer (the pipeline is one-way,
   EBNF→GrammarDescriptor→AST→proto), so we filed
   [accretional/gluon#10](https://github.com/accretional/gluon/issues/10) to add
   a `LexDescriptor`-driven `RenderEBNF`; once it lands, `emitEBNF` collapses to a
   call into gluon and the hand-rolled walk goes away.

## What is context-free grammar vs. overlay

Following proto-svg's "context-free core + constraint overlay" split:

- **In the grammar (context-free):** an item's shape — its type, its `id`, and
  its properties; each property's value is one of its ranges (a nested item
  production or a datatype leaf); enumerations as an alternation of members
  (planned). The subClassOf hierarchy is currently *flattened* into each type's
  property set and *annotated* in the EBNF; a future option is to model it
  structurally (a base-group production per supertype).

- **In an overlay (context-sensitive, resolved in a second pass):** the
  genuinely out-of-context links a CF tree can't express —
  - microdata **`itemref`** (properties pulled in from elsewhere by id),
  - JSON-LD **`@id` / `@reverse`** (the document is a *graph*, not a tree; nodes
    reference each other by identifier),
  - IDREF-style resolution and required/at-most-once cardinality.
  The grammar carries the *reference points* (an item has an `id`; a property may
  be a reference); the overlay resolves the actual edges.

## Front-ends as embedded / projection grammars

Each input syntax is a projection into this one vocabulary grammar:

- **Microdata** is attributes embedded in HTML → naturally an *embedded /
  projection grammar over proto-html*: recognize `itemscope`/`itemprop` within
  the HTML parse and project to items (the "second pass"). Integrating with
  `accretional/proto-html` is the intended path rather than re-parsing HTML here.
- **JSON-LD** is the same over a JSON grammar: parse the `@graph`, resolve
  `@context`/`@type`/`@id`, project into the Schema<Type> productions.

## Consequences

- The "grammar" objection is resolved: the grammar is first-class (AST + emitted
  EBNF), and the entity-linking structure is encoded in it.
- No markup-terminal / StripKeywords machinery — that stays retired.
- Next: enumerations as member alternations in the grammar; the reference overlay
  for `itemref`/`@id`; composition with proto-html / a JSON grammar.
