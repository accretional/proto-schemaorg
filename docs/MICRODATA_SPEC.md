# HTML Microdata — Implementation Digest

Working reference for building a Microdata extractor that walks a parsed DOM/element tree.
Every element is assumed to expose: **tag name (lowercased)**, an **attributes map**, and **ordered children** (each child is text or an element).

Sources (authoritative):
- WHATWG HTML Living Standard, Microdata: https://html.spec.whatwg.org/multipage/microdata.html
- W3C Microdata (WD 2017-05-04, has the same normative algorithms in one place): https://www.w3.org/TR/2017/WD-microdata-20170504/
- schema.org "Getting started using Microdata": https://schema.org/docs/gs.html

Terminology note: "the microdata to JSON" algorithm here is the spec's *JSON structured-data form* (an `{"items":[...]}` object). This is the model to encode into protobuf.

---

## 1. The five microdata attributes

All five are string/boolean HTML attributes. Tokenization uses **ASCII whitespace** (space, tab, LF, FF, CR) as the separator; empty tokens are discarded; duplicates are removed keeping first occurrence.

| Attribute | May appear on | Value form | Meaning |
|---|---|---|---|
| `itemscope` | **any** HTML element | boolean (presence only; value ignored) | Presence creates a **new item** (a group of name-value pairs) rooted at this element. |
| `itemtype` | only elements that **also have `itemscope`** | unordered set of unique space-separated tokens; each token is a valid **absolute URL**; all tokens must share the same vocabulary | The item's **item types**. |
| `itemid` | only elements that have **both `itemscope` and `itemtype`** | a valid URL (potentially surrounded by spaces), parsed relative to the element's node document | The item's **global identifier**. Only meaningful when `itemtype`'s vocabulary supports global identifiers. |
| `itemprop` | **any** HTML element | unordered set of unique space-separated tokens (the **property names**) | Declares that this element contributes one or more name-value pairs to the item it belongs to. |
| `itemref` | only elements that **also have `itemscope`** | unordered set of unique space-separated tokens; each token is the **ID** of an element **in the same tree/home subtree** | Extra elements (not descendants) to crawl when collecting this item's properties. Not itself a property list. |

Tokenization rules to implement:
- `itemtype` → split value on ASCII whitespace → list of type-URL strings (order preserved as authored; used later for the JSON `"type"` array).
- `itemprop` → split value on ASCII whitespace → **property names** (an element may declare several names → the same value is filed under each name).
- `itemref` → split value on ASCII whitespace → list of ID strings; for each ID, resolve to the **first** element in the tree with that `id`.

Property-name token validity (per spec): a token is valid if the item is typed and the token is a defined property of the vocabulary, OR the token is an absolute URL, OR (for an untyped item — no `itemtype`) it is a plain string containing no `.` and no `:`. An extractor generally records names as-is regardless of validity.

---

## 2. Definitions

- **Item**: any element with an `itemscope` attribute. "An element with the `itemscope` attribute specified creates a new item, a group of name-value pairs."
- **Item types**: tokens from splitting `itemtype` on ASCII whitespace; unique, case-sensitive, each an absolute URL, all in one vocabulary. An item may have **zero** types (untyped item).
- **Global identifier**: the `itemid` value, parsed as a URL relative to the element's node document. Only permitted/meaningful when the element has both `itemscope` and `itemtype`.
- **Property**: a name-value pair. The **name(s)** come from an element's `itemprop` tokens; the **value** is computed from that element (section 4 below). An element with `itemprop` belongs to exactly one item — the nearest item that crawls to it.
- **The properties of an item**: the ordered set of `itemprop`-bearing elements found by the crawl in section 3.

---

## 3. Algorithm — "the properties of an item"

Given an item's root element `root`, collect the `itemprop` elements that belong to it. Nested items are **not** descended into: a child `itemscope` element is itself a property value and its subtree is not crawled for `root`.

Verbatim step structure (W3C/WHATWG):

```
1. Let results, memory, and pending be empty lists of elements.
2. Add the element root to memory.
3. Add the child elements of root, if any, to pending.
4. If root has an itemref attribute, split the value of that itemref attribute on ASCII whitespace.
   For each resulting token ID, if there is an element in the tree of root with the ID ID,
   then add the first such element to pending.
5. Loop: If pending is empty, jump to the step labeled "end of loop".
6. Remove an element from pending and let current be that element.
7. If current is already in memory, there is a microdata error; return to the step labeled "loop".
8. Add current to memory.
9. If current does not have an itemscope attribute, then: add all the child elements of current to pending.
10. If current has an itemprop attribute specified and has one or more property names,
    then add current to results.
11. Return to the step labeled "loop".
12. End of loop: Sort results in tree order.
13. Return results.
```

Implementation notes:
- **`pending`** starts with root's direct children plus the `itemref`-referenced elements. It is a work list; order of removal does not matter because `results` is sorted into tree order at the end.
- **Termination at nested items**: step 9 only enqueues children of `current` when `current` has no `itemscope`. So when the crawl reaches a nested `itemscope` element, that element can still be added to `results` (step 10, if it also has `itemprop`) but its **subtree is not explored** for this item — its own properties belong to *it*.
- **`memory`** deduplicates; if the same element is reached twice (possible via `itemref`), step 7 records a **microdata error** and skips — this is the cycle/duplicate guard at the crawl level. It does not abort extraction.
- **`itemref`** attaches non-descendant elements. Referenced elements are crawled exactly like children (their own descendants get enqueued in step 9 unless they have `itemscope`).
- Results are **sorted in tree order** before use — property ordering in output follows document order, not `itemref` order.

An `itemprop` element with an `itemscope` sibling/ancestor structure but **no item that crawls to it** is a document conformance error ("A document must not contain any elements that have an `itemprop` attribute that would not be found to be a property of any of the items"), but an extractor simply never emits it.

---

## 4. Property value determination

For an element `element` that is a property (has `itemprop`), the **property value** is given by the **first matching case**:

| # | Condition (first match wins) | Value | URL? |
|---|---|---|---|
| 1 | `element` has `itemscope` | the **item** created by the element (a nested item object) | — |
| 2 | `meta` | value of `content` attribute, or `""` if absent | string |
| 3 | `audio`, `embed`, `iframe`, `img`, `source`, `track`, `video` | the absolute URL from resolving `src` against the element's node document; `""` if absent or resolve fails | **URL** |
| 4 | `a`, `area`, `link` | the absolute URL from resolving `href`; `""` if absent/fails | **URL** |
| 5 | `object` | the absolute URL from resolving `data`; `""` if absent/fails | **URL** |
| 6 | `data` | value of `value` attribute, or `""` | string |
| 7 | `meter` | value of `value` attribute, or `""` | string |
| 8 | `time` | the element's **datetime value**: value of `datetime` attribute if present, otherwise the element's `textContent` | string |
| 9 | otherwise | the element's `textContent` (descendant text content) | string |

Verbatim (WHATWG/W3C) for the URL/string cases, e.g. case 3:
> "The value is the absolute URL that results from resolving the value of the element's `src` attribute relative to the element at the time the attribute is set, or the empty string if there is no such attribute or if resolving it results in an error."

**URL property elements** (whose value is a URL resolved against the base URL): `a`, `area`, `audio`, `embed`, `iframe`, `img`, `link`, `object`, `source`, `track`, `video`. For these, the extractor must **resolve the relative URL against the document base URL** and emit the absolute URL (or `""` on error). All other string cases are taken literally with **no** URL resolution.

`time` special case (case 8): if `datetime` attribute exists use it verbatim; else fall back to `textContent`. This is the "datetime value".

`textContent` = concatenation of all descendant text nodes in document order (the DOM `textContent`), **not** trimmed by the spec. (schema.org guidance and real-world extractors commonly trim surrounding whitespace, but the spec value is the raw descendant text content.)

---

## 5. Top-level microdata items

> "An item is a top-level microdata item if its element does not have an `itemprop` attribute."

These are the **roots of extraction**. An `itemscope` element that **also** has `itemprop` is a *nested* item (it appears only as a property value of some parent item), so it is **not** emitted at the top level. Extraction iterates the nodes in tree order and collects every `itemscope`-without-`itemprop` element as a top-level item.

---

## 6. The "microdata to JSON" extraction algorithm (structured-data form)

Output shape:
```json
{
  "items": [ <item-object>, ... ]
}
```
where each `<item-object>` is:
```json
{
  "type": ["<itemtype-url>", ...],        // present only if item has >=1 type
  "id":   "<itemid-global-identifier>",   // present only if item has a global id
  "properties": {
    "<name>": [ <value>, ... ],           // always present; each name maps to an ARRAY
    ...
  }
}
```
A `<value>` is either a **string** or a **nested `<item-object>>`**.

### 6a. Top-level extraction algorithm (verbatim, W3C §6.1 / WHATWG)
Given a list of nodes `nodes` in a Document, extract microdata into a JSON form:
```
1. Let result be an empty object.
2. Let items be an empty array.
3. For each node in nodes, in tree order, if the node is an element and is a
   top-level microdata item, then get the object for that element and add it to items.
4. Add an entry to result called "items" whose value is the array items.
5. Return the result of serializing result to JSON (shortest form: no whitespace between
   tokens, no unnecessary zero digits, Unicode escapes only for characters without a
   dedicated escape sequence, lowercase "e" in numbers where appropriate).
```

### 6b. "Get the object for an item" (verbatim, W3C §6.1 / WHATWG)
Run for an item `item`, optionally with a list of elements `memory`:
```
1. Let result be an empty object.
2. If no memory was passed to the algorithm, let memory be an empty list.
3. Add item to memory.
4. If the item has any item types, add an entry to result called "type" whose value is an
   array listing the item types of item, in the order they were specified on the itemtype attribute.
5. If the item has a global identifier, add an entry to result called "id" whose value is the
   global identifier of item.
6. Let properties be an empty object.
7. For each element element that has one or more property names and is one of the properties of
   the item item, in the order those elements are given by the algorithm that returns the
   properties of an item, run the following substeps:
     1. Let value be the property value of element.
     2. If value is an item, then:
          - If value is in memory, then let value be the string "ERROR".
          - Otherwise, get the object for value, passing a copy of memory, and then replace value
            with the object returned from those steps.
     3. For each name name in element's property names, run the following substeps:
          1. If there is no entry named name in properties, then add an entry named name to
             properties whose value is an empty array.
          2. Append value to the entry named name in properties.
8. Add an entry to result called "properties" whose value is the object properties.
9. Return result.
```

Key structural facts to encode:
- **`type`** = array of `itemtype` tokens in authored order; **omitted entirely if the item has no `itemtype`**.
- **`id`** = the `itemid` global identifier string; **omitted if no `itemid`** (and only meaningful with `itemtype`).
- **`properties`** = always present object; **every** name maps to a **JSON array**, even for a single value.
- A value is:
  - a **nested object** when the property element has `itemscope` (recurse via "get the object for an item" with a **copy** of `memory`), or
  - the string `"ERROR"` if that nested item is already in `memory` (cycle guard), or
  - a **string** for all other property-value cases (with URL resolution applied for URL property elements).
- A single element with multi-token `itemprop` (e.g. `itemprop="a b"`) files the **same value** under each name (substep 7.3 loops over all names).

---

## 7. Edge cases & error handling

- **itemref cycles / re-entry**: two guards. (a) Crawl-level: step 7 of §3 flags a "microdata error" and skips if an element is reached twice within one item's crawl. (b) JSON-level: substep 7.2 replaces a nested item already present in `memory` with the string `"ERROR"`, preventing infinite recursion when item A's property is item B and B's property is A. The spec also states conformance-wise that documents *must not* contain `itemref` cycles, but a robust extractor must still handle them (via these guards).
- **itemprop with no itemscope ancestor / unreachable itemprop**: a conformance error; such elements are never collected by any item's crawl, so they simply do not appear in output.
- **Duplicate property names (allowed)**: multiple properties with the same name and different values are legal; in JSON each name maps to an **array** and values are appended in tree order. Single values are still wrapped in a one-element array.
- **itemid only meaningful with itemtype**: only emit `id` when the item is typed (has `itemtype`); `itemid` on an untyped item is not meaningful.
- **Untyped items**: an `itemscope` element with no `itemtype` is valid; omit `type`; property names may be any tokens without `.`/`:`.
- **Whitespace / tokenization**: split `itemtype`, `itemprop`, `itemref` on ASCII whitespace; discard empty tokens; dedupe (first wins). The spec's property value is the raw `textContent` (untrimmed); trimming leading/trailing whitespace is a common practical extension, not spec-mandated.
- **URL resolution failures**: for URL property elements, a missing/unresolvable `src`/`href`/`data` yields `""` (empty string), not omission of the property.
- **Missing attributes on value elements**: `meta` without `content`, `data`/`meter` without `value` → `""`.
- **`time` without `datetime`**: falls back to `textContent`.
- **Ordering**: item objects emitted in tree order of their top-level elements; properties emitted in the tree order produced by the §3 crawl (results sorted in tree order), NOT in `itemref` order.
```
