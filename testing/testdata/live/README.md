# Live real-world microdata corpus

This directory holds **raw HTML documents fetched from the public web** that contain
[schema.org Microdata](https://schema.org/docs/gs.html) (the `itemscope` / `itemtype` /
`itemprop` attribute family). The goal is to exercise the extractor against **messy,
real-world markup** rather than hand-written fixtures.

These files are **verbatim snapshots**. They are unmodified from what the server returned
(only decompressed when the server used gzip). They are intentionally imperfect: some are
huge, some mix `http://schema.org/` and `https://schema.org/` in the same document, some
contain broken/unclosed tags, and several contain **HTML-escaped example markup** inside
`<pre>`/`<code>` that looks like microdata but is *not* live in the DOM (a good negative
test — the extractor must not pick it up).

## Provenance format

Every page `NNN-sourceslug.html` has a sibling `NNN-sourceslug.html.meta.json`:

```json
{
  "url": "https://example.com/page",
  "fetched": "2026-07-03",
  "source_desc": "human description of what this page is",
  "schema_types_seen": ["Type1", "Type2"],
  "notes": "quirks, http-vs-https, live-vs-escaped, size, etc."
}
```

`schema_types_seen` lists the schema.org types that appear as **live, in-DOM** `itemtype`
values (types that only appear escaped inside code samples are called out in `notes`
instead).

All pages were fetched on **2026-07-03** with:

```
curl -sL --compressed -A "<desktop Chrome UA>" <url> -o <file>
```

## Sources in this corpus

| File | Source | Live schema.org types |
|------|--------|-----------------------|
| 001-schemaorg-person.html | schema.org/Person doc page | BreadcrumbList, ListItem, SoftwareSourceCode |
| 002-schemaorg-product.html | schema.org/Product doc page | BreadcrumbList, ListItem, SoftwareSourceCode |
| 003-schemaorg-recipe.html | schema.org/Recipe doc page | BreadcrumbList, ListItem, SoftwareSourceCode |
| 004-schemaorg-event.html | schema.org/Event doc page | BreadcrumbList, ListItem, SoftwareSourceCode |
| 005-stackoverflow-question.html | Stack Overflow Q&A page | QAPage, Question, Answer, Comment, Person |
| 006-gutenberg-book.html | Project Gutenberg ebook page | Book, Offer, BreadcrumbList, ListItem |
| 007-openlibrary-work.html | Open Library work page | Book |
| 008-w3resource-recipe-tutorial.html | w3resource Recipe tutorial | TechArticle |
| 009-humanlevel-product-guide.html | Human Level SEO blog article | Blog, CreativeWork |
| 010-nps-yellowstone.html | US NPS Yellowstone home page | VideoObject |
| 011-npr-error-page.html | NPR 404 / error page | SpeakableSpecification |

Union of live types across the corpus: **QAPage, Question, Answer, Comment, Person, Book,
Offer, BreadcrumbList, ListItem, TechArticle, Blog, CreativeWork, VideoObject,
SpeakableSpecification, SoftwareSourceCode**.

### Notable real-world characteristics to test against

- **Escaped vs. live microdata.** The four schema.org doc pages, the w3resource tutorial,
  and the Human Level blog article all embed microdata *examples* as HTML-escaped text.
  Those must NOT be extracted. Only their page-chrome microdata is live.
- **Mixed URL scheme.** Stack Overflow and Project Gutenberg use both `http://schema.org/`
  and `https://schema.org/` within a single document.
- **Deep nesting.** The Stack Overflow page nests Person/Comment items inside each Answer,
  inside a QAPage — the richest structure here.
- **Error page.** 011 is deliberately an NPR 404 page whose template still emits microdata.

## Caveat: these are snapshots and will rot

The live web changes. Any of these pages may be redesigned, migrate fully to JSON-LD,
change URL, or disappear. **Do not re-fetch and expect identical bytes.** Treat the saved
`.html` files as the ground truth; the `url` in each `.meta.json` records where it came
from at fetch time, not a guarantee of current content.

### Types we looked for but could not find in live microdata

During collection, a large majority of candidate sites had **migrated to JSON-LD** (or
blocked automated fetches). In particular, live **Recipe** microdata is now effectively
extinct on major recipe sites (AllRecipes, BBC Good Food, Simply Recipes, RecipeTin,
Natasha's Kitchen, Sally's Baking, etc. — all JSON-LD or 403/404), and mainstream
**Product/Offer**, **NewsArticle**, **Event**, and **LocalBusiness** pages likewise no
longer emit microdata. Discarded candidates included: Wikipedia, BBC News, Rotten Tomatoes,
Metacritic, Goodreads, the WHATWG and W3C microdata specs (examples are escaped-only),
MDN's `itemscope` reference (escaped-only), Discogs/eBay/Yelp/Zillow/cars.com (403),
IMDb (empty JS shell), Eventbrite (untyped `itemscope`), plus ~20 recipe/news/blog sites
that returned zero live microdata.
