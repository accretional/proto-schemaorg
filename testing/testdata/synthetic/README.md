# Synthetic Microdata Test Corpus

Hand-crafted HTML fixtures paired with their expected extraction output, exercising
the WHATWG HTML Microdata algorithm (see `docs/MICRODATA_SPEC.md`) exhaustively.
This corpus is ground truth for testing the extractor.

## File format

Each test case is a pair of files sharing the stem `NNN-slug`:

- **`NNN-slug.html`** — a small, complete, well-formed HTML5 document containing microdata.
- **`NNN-slug.json`** — the expected extraction result.

## JSON schema

The expected output uses the WHATWG "microdata to JSON" structured-data form:

```json
{
  "_base": "https://example.com/",
  "_desc": "what this case tests",
  "items": [ <item-object>, ... ]
}
```

where each `<item-object>` is:

```json
{
  "type": ["<itemtype-url>", ...],   // present only when the item has >=1 itemtype
  "id":   "<itemid-identifier>",     // present only when the item has an itemid (and a type)
  "properties": {
    "<name>": [ <value>, ... ]       // ALWAYS present; every name maps to an ARRAY
  }
}
```

A `<value>` is either a **string** or a nested `<item-object>`. A nested item that is
already on the recursion stack (an `itemref` cycle) is replaced by the string `"ERROR"`.

### Conventions

- **`_base`** and **`_desc`** are corpus-only helper fields (not part of the WHATWG form).
  `_base` records the document base URL used to resolve URL-valued properties; `_desc`
  names what the case exercises.
- **Base URL** is `https://example.com/` for every case. URL-valued property elements
  (`a`, `area`, `audio`, `embed`, `iframe`, `img`, `link`, `object`, `source`, `track`,
  `video`) have their `src`/`href`/`data` resolved against this base and emitted as an
  absolute URL (or `""` on absence/failure). All other property values are literal.
- **`type`** preserves authored `itemtype` token order; it is omitted when the item is untyped.
- **`id`** is the `itemid`, resolved as a URL relative to the document; omitted when absent.
- **`properties`** is always an object; every name maps to an array, even single values.
  Property order follows the section-3 crawl result sorted into **tree order** (document
  order), which is not necessarily `itemref` order.
- **Text values** are the element's raw `textContent`. Fixtures deliberately place text
  immediately adjacent to its tags (no surrounding whitespace) so the raw `textContent`
  equals its trimmed form and there is no trimming ambiguity.

## Cases

| # | Slug | What it tests |
|---|------|---------------|
| 001 | minimal-person | Minimal single item: itemscope + itemtype=Person + one `name`. |
| 002 | person-multiple-properties | Several properties on one item: text, `a[href]` (url), `img[src]` (image). |
| 003 | nested-address | Nested item: Person.address -> PostalAddress. |
| 004 | value-cascade | Every element type in the value cascade (meta, img, a, link, object, data, meter, time, audio, video, embed, iframe, source, track, area, plain text); URL resolution vs. literals. |
| 005 | itemref | Properties pulled from a non-descendant element by `itemref` id. |
| 006 | multiple-top-level | Two independent top-level items in one document. |
| 007 | multi-token-itemprop | `itemprop="name additionalName"` files one value under both names. |
| 008 | multi-token-itemtype | Two type URLs on one item; `type` array order. |
| 009 | itemid | Global identifier via `itemid` (with `itemtype`). |
| 010 | orphan-itemprop | `itemprop` with no itemscope ancestor -> no items emitted (`items: []`). |
| 011 | itemref-cycle | `itemref` cycle -> nested item already in memory becomes `"ERROR"`. |
| 012 | product-composite | Product with brand, offers (Offer), aggregateRating (AggregateRating). |
| 013 | empty-and-missing-values | itemscope with no properties; missing value attributes yield `""`. |
| 014 | deeply-nested-recipe | 3-level nesting: Recipe -> nutrition / recipeInstructions -> itemListElement. |
