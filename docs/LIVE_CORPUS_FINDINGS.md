# Live corpus findings — why real-world schema.org extraction currently yields nothing

**Date:** 2026-07-15
**Scope:** `testing/testdata/live/*.html` (11 real fetched pages) run through both
extractors — `service/microdata.Extract` and `service/jsonld.ExtractHTML`.
**Harness:** `testing/livecheck` (`go run ./testing/livecheck`), non-gating.
**Verdict up front:** **0 of 11 real pages parse.** Both extractors are blocked at
the very first step — `github.com/accretional/proto-html/htmlparse.ParseDOM` —
so neither microdata nor JSON-LD is ever reached. This is a **proto-html parser
problem**, not a proto-schemaorg extractor problem: the extraction logic is never
exercised because there is no DOM to walk.

---

## 1. The 0/11 result

Every page fails with the identical error:

```
parsing "HtmlDocument": production "HtmlDocument" did not match at offset 0
```

| Page | Parse | Microdata items | JSON-LD items |
|---|---|---|---|
| 001-schemaorg-person.html | **FAIL** | 0 | 0 |
| 002-schemaorg-product.html | **FAIL** | 0 | 0 |
| 003-schemaorg-recipe.html | **FAIL** | 0 | 0 |
| 004-schemaorg-event.html | **FAIL** | 0 | 0 |
| 005-stackoverflow-question.html | **FAIL** | 0 | 0 |
| 006-gutenberg-book.html | **FAIL** | 0 | 0 |
| 007-openlibrary-work.html | **FAIL** | 0 | 0 |
| 008-w3resource-recipe-tutorial.html | **FAIL** | 0 | 0 |
| 009-humanlevel-product-guide.html | **FAIL** | 0 | 0 |
| 010-nps-yellowstone.html | **FAIL** | 0 | 0 |
| 011-npr-error-page.html | **FAIL** | 0 | 0 |

**SUMMARY: 11 pages | parse OK: 0/11 | pages w/ microdata: 0/11 | pages w/ JSON-LD: 0/11**

(The 14 synthetic fixtures in `testing/testdata/synthetic/` all parse — they were
hand-authored to fit the grammar. Only real-world pages fail.)

`offset 0` is **not** where the problem is. proto-html's grammar has one start
production, `HtmlDocument = { Comment } , Doctype , { Comment } , HTMLHtmlElement , { Comment }`,
and gluon reports the offset where backtracking finally unwinds — the start of the
whole document — regardless of where inside the page the first unmatchable
construct actually sits. The single-line, position-free error is itself a
diagnosability problem (see §5).

---

## 2. Method — how the failures were isolated

Because the error carries no position, the pages were reduced against a
**known-good minimal document** rather than by trusting the error offset. Two
independent techniques agreed:

1. **Construct probing.** Each page was parsed with a tolerant reference parser
   (`golang.org/x/net/html`, used only in a throwaway diagnostic — never in the
   product) to enumerate its distinct elements and attributes, and each construct
   was then re-tested **alone**, inserted into a minimal valid skeleton
   (`<!DOCTYPE html><html><head><title>t</title></head><body>…</body></html>`)
   that is known to parse. A construct that flips that skeleton from OK to FAIL is
   a confirmed grammar/parser gap.
2. **Normalization experiment.** Each page was parsed and re-serialized by
   `x/net/html` (which fixes case, quoting, unclosed tags, entity escaping — every
   *lexical* deviation a browser silently repairs) and the **normalized** output
   was fed back to `htmlparse.ParseDOM`.

   **Result: 0/11 normalized pages parse either.** Making every page perfectly
   well-formed does *not* let the strict grammar accept it. That isolates the
   failures as **structural grammar gaps** (constructs the grammar does not model
   at all), not merely lax syntax the grammar could shrug off.

Minimal repros below are complete documents: each parses when the flagged
construct is removed and fails when it is present.

---

## 3. The failure categories (with minimal repros and corpus frequency)

Frequencies are counts of pages (out of 11) that contain the construct — i.e. how
much of the corpus each gap blocks on its own.

### A. Unbounded range-leaf seams with no token matcher — **the single biggest blocker**

The grammar models a handful of opaque values as an unbounded character range
`{ " " ... "￿" }` (space 0x20 … 0xFFFF). That range **includes the closing
delimiter itself** (`"` is 0x22; `<` is 0x3C), and — unlike `CssStyleSheet`
(`<style>` body) and `script_text` — these leaves have **no token matcher
registered** in `htmlparse.Matchers()`. With nothing to stop it, the range
greedily eats the closing quote / end-tag and everything after, so the following
terminal can never match. **These values can never parse — not even when empty.**

Affected surfaces (grammar rule → source):
- `style="…"` — `StyleAttr → css_declaration_list` (`lang/globals.ebnf:59-60`)
- `media="…"` — `MediaQueryListType` on `<link>`/`<style>`/`<meta>` (`lang/metadata.ebnf`)
- inline `<svg>…</svg>` — `SVGSVGElement = "<svg" , { " " ... "￿" } , "</svg>"` (`lang/content.ebnf:30`)

Minimal repros (all FAIL):
```html
<!DOCTYPE html><html><head><title>t</title></head><body><div style="display: none"></div></body></html>
<!DOCTYPE html><html><head><title>t</title><style media="screen">.a{}</style></head><body></body></html>
<!DOCTYPE html><html><head><title>t</title></head><body><svg viewBox="0 0 1 1"><path d="M0 0"/></svg></body></html>
```
Even `style=""` (empty) fails, confirming the delimiter-swallow root cause rather
than a value-content issue.

**Frequency:** `style=` appears in **11/11** pages; inline `<svg` in **10/11**.
Either one, alone, guarantees the whole corpus fails. This is the highest-leverage
fix. Contrast: `<style>…</style>` element bodies and `<script>…</script>` bodies
*do* parse, because they have `rawUntilCI` matchers — the same fix pattern applies
here (a `</svg>`-terminated matcher for SVG; a quote-terminated matcher for the
`style`/`media` attribute leaves).

### B. Case-sensitive matching of case-insensitive HTML constructs

Grammar terminals/enums are matched case-sensitively, but the constructs are
ASCII-case-insensitive per WHATWG. The grammar comments even claim
case-insensitivity ("The keyword is ASCII-case-insensitive to the parser") but the
literal terminals do not implement it.

- **DOCTYPE:** only `<!DOCTYPE html>` (uppercase) matches; `<!doctype html>` FAILS
  (`StandardDoctype`, `lang/html.ebnf:36`).
- **charset:** `EncodingLabel = "utf-8"` only (`lang/datatype.ebnf:176`) —
  `charset="UTF-8"` FAILS, and so does every non-UTF-8 label (e.g. `iso-8859-1`).
- **http-equiv:** the keyword set is lowercase-only
  (`lang/metadata.ebnf:63-65`) — `http-equiv="X-UA-Compatible"` and
  `http-equiv="Content-Type"` FAIL; their lowercased forms pass.

Minimal repros (FAIL):
```html
<!doctype html><html><head><title>t</title></head><body></body></html>
<!DOCTYPE html><html><head><title>t</title><meta charset="UTF-8"></head><body></body></html>
<!DOCTYPE html><html><head><title>t</title><meta http-equiv="Content-Type" content="text/html; charset=UTF-8"></head><body></body></html>
```
**Frequency:** lowercase `<!doctype html>` in **2/11** (010, 011); uppercase
`UTF-8` charset in **4/11**; `http-equiv` in **2/11**.

### C. Missing attributes — RDFa / OpenGraph and legacy attributes

Whole attributes real pages universally emit are absent from both the element
attribute groups and `GlobalAttribute`:

- **OpenGraph / RDFa `property`** on `<meta>` and elements — not in
  `MetaAttribute` (`lang/metadata.ebnf:60-61`) nor in `GlobalAttribute`.
  `<meta property="og:title" …>` FAILS.
- **RDFa globals** `typeof`, `about`, `resource`, `vocab`, `datatype`, and
  `content`/`rel` on non-`<meta>`/`<link>` hosts — all FAIL (Project Gutenberg's
  book markup is RDFa-based and trips every one of these).
- **`charset` on `<script>`** (legacy but common) — FAILS.

Minimal repros (FAIL):
```html
<!DOCTYPE html><html><head><title>t</title><meta property="og:title" content="Hi"></head><body></body></html>
<!DOCTYPE html><html><head><title>t</title></head><body><div typeof="Book" property="dc:title" about="#x">t</div></body></html>
```
**Frequency:** `<meta property=` in **6/11**.

### D. Closed enumerations narrower than the real web

Enumerated attribute value sets reject common real values:
- `<link rel>` — `LinkRelToken` (`lang/metadata.ebnf:41-45`) is a closed set that
  omits `apple-touch-icon`, `mask-icon`, `image_src`, `dns-prefetch` variants,
  etc. `<link rel="apple-touch-icon" …>` FAILS.
- `EncodingLabel` (see §B) — a one-element enum (`utf-8`).

### E. Attribute quoting — only double quotes

The grammar bakes attributes as the literal ` name="` … `"`. **Single-quoted**
(`class='x'`) and **unquoted** (`class=x`) attribute values both FAIL.

Minimal repros (FAIL):
```html
<!DOCTYPE html><html><head><title>t</title></head><body><div class='x'></div></body></html>
<!DOCTYPE html><html><head><title>t</title></head><body><div class=x></div></body></html>
```
**Frequency:** single-quoted attributes in **10/11** pages.

### F. Structural / lexical strictness

- **Mandatory DOCTYPE:** a page with no doctype FAILS (`Doctype` is required).
- **Bare `<` in text** (e.g. `a < b`) FAILS — the `text` matcher treats `<` as a
  tag open.
- **Over-strict content models** — several bare structural elements are rejected
  in positions browsers accept, and foreign-content nesting is limited.

Constructs that **do** parse (for the record): double-quoted `class` (including
`:`-bearing tokens like Tailwind `md:flex`), `data-*`, `id`, `aria-*`, `role`,
`itemscope`/`itemtype`/`itemprop`, boolean attrs (`hidden`, `disabled`), HTML
entities (`&nbsp;`, `&amp;`, `&#169;`), `<a>`/`<img>`/`<ul>`/`<table>`/`<nav>`,
`tabindex`, `<script>` (src/module/text-javascript/ld+json/async/defer),
`<style>…</style>` bodies, `<template>`, `<noscript>`, comments (incl. conditional
comments), and simple `<math>`.

---

## 4. Root-cause verdict

**Confirmed:** the mechanism is a **strict, whole-document context-free grammar
with no error recovery.** `HtmlDocument` must match the *entire* byte stream
against one production; a single unmatchable construct anywhere fails the whole
document, and gluon reports the unwind point (`offset 0`) rather than the true
site. This is exactly why 0/11 real pages parse while 14/14 hand-fitted synthetic
fixtures do.

**Refuted (important nuance):** it is **not** the case that each page has one
localized construct that, once removed, lets the rest parse. Real pages hit **many
independent gaps simultaneously.** Distinct failing element/attribute signatures
per page (construct-probe, with some benign methodology noise from context
nesting):

| Page | distinct failing element-signatures |
|---|---|
| 006-gutenberg-book.html | ~53 |
| 007-openlibrary-work.html | ~80 |
| 010-nps-yellowstone.html | ~57 |
| 011-npr-error-page.html | ~32 |

These collapse to roughly **six distinct construct-categories** (§3 A–F), and each
page trips **several** of them. Because the highest-frequency gaps —
category A (`style=`: 11/11) and inline `<svg>` (10/11) — each block essentially
the whole corpus on their own, the corpus is *multiply* blocked: closing any one
gap changes nothing observable until *all* the gaps a given page hits are closed.

**Diagnosability corollary:** every failure (including an empty document) returns
the byte-identical `offset 0` error. There is no stable, position-bearing oracle,
so even automated delta-debugging cannot localize a failure — a greedy reducer
happily shrinks any page to the empty string, which "fails" identically. A parser
used against real-world input needs to report *where* and *what* it choked on.

**Bottom line:** the long tail of real-world HTML deviation is effectively
unbounded. Enumerating and closing grammar gaps one by one will not make
real-world schema.org extraction robust — the next fetched page brings new
constructs. The WHATWG parsing algorithm exists precisely because HTML cannot be
handled as a strict CFG over raw bytes.

---

## 5. Recommendation (ordered by leverage)

1. **Give proto-html a tolerant / error-recovery parse path for extraction
   (the real fix).** Microdata and JSON-LD extraction do not need a validating,
   round-trippable structural parse — they need a *forgiving* DOM. proto-html's
   own `dom` package already advertises this ("This generic DOM is the output of
   the tolerant parser (package htmlparse)"), but `htmlparse` is in fact the
   strict CFG; that intent is unmet. A WHATWG-style tokenizer + tree-construction
   pass (skip/annotate unknown attributes, keep unknown elements as generic
   nodes, never fail the whole document) would let extraction run over any page.
   This is the only path that scales to the open web.
2. **Fix the unbounded range-leaf matchers (category A) — cheap, enormous
   payoff.** Register token matchers for `css_declaration_list` /
   `MediaQueryListType` (stop at the closing `"`) and for the `<svg>` body (a
   `rawUntilCI("</svg>")` matcher, mirroring `CssStyleSheet`/`script_text`).
   `style=` alone blocks 11/11 pages; inline `<svg>` 10/11. Even without full
   error recovery, this unblocks the two most pervasive constructs.
3. **Make case-insensitive constructs case-insensitive (category B).** DOCTYPE,
   `charset` (accept the full WHATWG encoding-label set, case-insensitively), and
   `http-equiv` keywords. Low effort, removes a whole class of avoidable failures.
4. **Model RDFa/OpenGraph attributes (category C).** Add `property` (and the RDFa
   globals `typeof`/`about`/`resource`/`vocab`/`datatype`) as recognized global
   attributes. Directly relevant: schema.org data on Gutenberg-style pages is
   RDFa, and OpenGraph `<meta property>` is near-universal.
5. **Loosen quoting and the narrow enumerations (categories D/E)** — accept
   single-quoted/unquoted attribute values and widen `LinkRelToken`. Lower
   priority than 1–4, and largely subsumed by a tolerant parser (item 1).
6. **Emit position-bearing parse errors** (offset + the failing construct), so
   future regressions are diagnosable instead of a uniform `offset 0`.

Items 2–4 are worth doing regardless (they also harden the strict grammar), but
**item 1 is the one that makes real-world extraction actually work.** Until then,
`service/microdata` and `service/jsonld` will extract nothing from live pages,
however correct their post-DOM logic is.

---

## 6. Filed upstream issues (accretional/proto-html)

- **[#8](https://github.com/accretional/proto-html/issues/8)** — Unbounded
  range-leaf seams (`style`/`media` attrs, inline `<svg>`) lack token matchers and
  can never match a non-empty value (category A; blocks 11/11 live pages). The
  highest-leverage fix.
- **[#9](https://github.com/accretional/proto-html/issues/9)** — Case-sensitive
  matching of case-insensitive HTML (DOCTYPE, charset, http-equiv) (category B).
- **[#10](https://github.com/accretional/proto-html/issues/10)** — Strict
  whole-document grammar with no error recovery: 0/11 real pages parse; extraction
  needs a tolerant parse path (the dominant finding, §4).

Each references that these block real-world schema.org extraction in
proto-schemaorg.

---

## Reproduce

```
go run ./testing/livecheck        # the per-page table in §1 (non-gating)
```
The isolation harness used for §2–§3 is a throwaway diagnostic (depends on
`x/net/html`) and is intentionally **not** committed to this repo.
