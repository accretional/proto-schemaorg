# Schema.org Vocabulary Vendoring Plan

Goal: vendor the machine-readable schema.org vocabulary into the repo so a Go
generator can emit protobuf messages (one message per schema.org Type, fields
derived from properties).

## 1. Latest release

- **Latest version: `30.0`**, released **2026-03-19**
  ("Add equivalence annotations, add EU DPP examples, misc updates").
- Source repo: <https://github.com/schemaorg/schemaorg>
- Release dumps live under `data/releases/<version>/`. Each release ships the
  same vocabulary in many serializations, in an `http` and an `https` variant
  (identifier scheme for the term IRIs) and in a `-current-` (core only) and
  `-all-` (core + pending + attic/retired) flavor.
- **Use the `https` + `all` variant** so IRIs are `https://schema.org/...` and
  no terms are silently missing.

### Exact raw URLs (version 30.0)

Base: `https://raw.githubusercontent.com/schemaorg/schemaorg/main/data/releases/30.0/`
(`main` is pinned to 30.0 today; for reproducibility you may pin the release
tag/commit instead of `main`.)

| File | Raw URL | Size |
|------|---------|------|
| `schemaorg-all-https.jsonld` (PRIMARY) | `https://raw.githubusercontent.com/schemaorg/schemaorg/main/data/releases/30.0/schemaorg-all-https.jsonld` | 1,560,763 B (~1.5 MB) |
| `schemaorg-all-https-types.csv` (convenience) | `https://raw.githubusercontent.com/schemaorg/schemaorg/main/data/releases/30.0/schemaorg-all-https-types.csv` | 2,484,737 B (~2.4 MB) |
| `schemaorg-all-https-properties.csv` (convenience) | `https://raw.githubusercontent.com/schemaorg/schemaorg/main/data/releases/30.0/schemaorg-all-https-properties.csv` | 515,322 B (~503 KB) |

Other serializations available in the same directory if ever needed:
`.nt` (N-Triples), `.nq` (N-Quads), `.ttl` (Turtle), `.rdf` (RDF/XML),
`.owl`, plus `schemaorgcontext.jsonld` (the JSON-LD `@context`).

## 2. Vocabulary structure

The JSON-LD dump is a single object with a `@context` and a flat
`"@graph": [ ... ]` array of ~3,235 nodes. Every term is one node keyed by
`@id` (e.g. `"schema:Person"`; expands to `https://schema.org/Person`).

### Type (class)
```json
{ "@id": "schema:Person", "@type": "rdfs:Class",
  "rdfs:label": "Person",
  "rdfs:comment": "A person (alive, dead, undead, or fictional).",
  "rdfs:subClassOf": { "@id": "schema:Thing" } }
```
- `@type` = `rdfs:Class`.
- `rdfs:subClassOf` gives the parent(s) (single object or array). Root is
  `schema:Thing`.
- In the CSV `schemaorg-all-https-types.csv` each type is a row with columns:
  `id,label,comment,subTypeOf,enumerationtype,equivalentClass,properties,
  subTypes,supersedes,supersededBy,isPartOf`. The `properties` column is a
  comma-separated list of the property IRIs whose domain includes this type
  (this is the pre-joined convenience the CSV gives you).

### Property
```json
{ "@id": "schema:author", "@type": "rdf:Property",
  "rdfs:label": "author",
  "schema:domainIncludes": [ {"@id":"schema:CreativeWork"}, {"@id":"schema:Rating"} ],
  "schema:rangeIncludes": [ {"@id":"schema:Organization"}, {"@id":"schema:Person"} ] }
```
- `@type` = `rdf:Property`.
- `schema:domainIncludes` = the types that CAN carry this property (the "which
  messages get this field" set). Single object or array.
- `schema:rangeIncludes` = the expected value type(s) (the "field type" set).
  Single object or array.
- `rdfs:subPropertyOf`, `schema:inverseOf` may also appear.
- CSV `schemaorg-all-https-properties.csv` columns:
  `id,label,comment,subPropertyOf,equivalentProperty,subproperties,
  domainIncludes,rangeIncludes,inverseOf,supersedes,supersededBy,isPartOf`
  where `domainIncludes`/`rangeIncludes` are comma-separated IRI lists.

### Enumeration
- An enumeration type is a normal `rdfs:Class` that is (transitively)
  `rdfs:subClassOf schema:Enumeration`, e.g. `schema:DayOfWeek`.
- Its members are separate nodes whose `@type` is the enumeration class:
```json
{ "@id": "schema:Monday", "@type": "schema:DayOfWeek",
  "rdfs:label": "Monday", "rdfs:comment": "...",
  "schema:sameAs": {"@id":"http://www.wikidata.org/entity/Q105"} }
```
- In `types.csv`, members appear as their own rows with the `enumerationtype`
  column set to the owning enumeration IRI.

### DataType
- The 7 primitive datatypes are nodes typed `["schema:DataType","rdfs:Class"]`:
  **`Text`, `Number`, `Boolean`, `Date`, `DateTime`, `Time`, `Quantity`**.
```json
{ "@id": "schema:Text", "@type": ["schema:DataType","rdfs:Class"],
  "rdfs:label": "Text", "rdfs:comment": "Data type: Text." }
```
- `URL` is modeled as a subclass of `Text` (not a top-level DataType);
  `Integer`/`Float` are subclasses of `Number`; `True`/`False` are members of
  `Boolean`. Handle these as string/scalar mappings.

### Deprecation / superseding
- Retired or replaced terms carry `schema:supersededBy` pointing at the
  replacement, e.g. property `schema:episodes` -> `schema:supersededBy
  schema:episode`. In CSV this is the `supersededBy` column (and `supersedes`
  for the reverse). Treat non-empty `supersededBy` as deprecated.

### Rough counts (from the 30.0 https/all dump)
- JSON-LD `@graph`: **3,235 nodes** total.
- **1,014 `rdfs:Class` nodes** (types) — of which 930 are "plain" classes in
  the types CSV, plus enumeration classes; includes the 7 DataTypes.
- **1,684 `rdf:Property` nodes** (properties).
- **537 enumeration member** nodes.
- **7 primitive DataTypes.**
- CSV row totals (partition differently: members are rows in types.csv):
  types.csv = **1,474 rows** (930 classes + 537 enum members + 7 datatypes),
  properties.csv = **1,529 rows**.

## 3. Recommended distribution format

- **Primary (commit this): `schemaorg-all-https.jsonld`.** It is the complete,
  self-describing graph with unambiguous, multi-valued `domainIncludes` /
  `rangeIncludes` / `subClassOf` as proper IRI references, plus datatypes,
  enumerations, and `supersededBy`. Go parses it with just `encoding/json` into
  `map[string]any` / a typed struct over the `@graph` array — no RDF library
  needed. It is also the smallest of the useful dumps (~1.5 MB).
- **Convenience (optional, also commit): the CSV pair**
  `schemaorg-all-https-types.csv` + `schemaorg-all-https-properties.csv`.
  Parseable with `encoding/csv`. The CSVs pre-join the type<->property
  relationship (`properties` column on types, `domainIncludes` on properties),
  which can simplify a first-pass generator. Caveat: multi-valued cells are
  comma+space-separated strings you must split, and enum members are mixed into
  the types file — the JSON-LD keeps these distinctions cleaner.

Recommendation: vendor the JSON-LD as the source of truth for the generator;
optionally also vendor the two CSVs for quick inspection/diffing.

## 4. What to commit and how Go consumes it

### Commit into `schema/`
```
schema/
  schemaorg-all-https.jsonld            # PRIMARY - generator input
  schemaorg-all-https-types.csv         # optional convenience
  schemaorg-all-https-properties.csv    # optional convenience
  VERSION                               # "30.0" + upstream commit for provenance
```
Pin provenance: record version `30.0`, the upstream commit SHA, and the raw
URLs so re-vendoring is reproducible.

### Go generator sketch (JSON-LD -> proto)
1. `json.Unmarshal` the file; iterate `@graph`.
2. Bucket nodes by `@type`:
   - `rdfs:Class` (and `["schema:DataType","rdfs:Class"]`) -> types/datatypes.
   - `rdf:Property` -> properties.
   - anything else typed `schema:X` where `X` is an enumeration class -> enum
     member.
3. Build a `map[typeIRI] -> []propertyIRI` by scanning every property's
   `domainIncludes` (normalize single-object vs array). This yields, per type,
   the fields for its proto message.
4. Emit **one proto `message` per non-datatype Type** (message name = the label,
   e.g. `Person`). For each property in that type's field set:
   - field name = property label (snake_case it for proto style);
   - field type from `rangeIncludes`, mapped:
     - `Text`/`URL` -> `string`; `Number`/`Integer`/`Float` -> `double`/`int64`;
       `Boolean` -> `bool`; `Date`/`DateTime`/`Time` -> `string` (or
       `google.protobuf.Timestamp`);
     - a schema.org Type range -> a message reference to that type;
     - an Enumeration range -> a proto `enum` generated from that enumeration's
       members;
     - multiple `rangeIncludes` -> either `oneof`, or fall back to a wrapper /
       `string`. schema.org properties are inherently multi-valued, so make
       every field `repeated`.
5. Generate a proto `enum` per Enumeration class from its member nodes.
6. Skip or mark `@deprecated` any term with `supersededBy`.
7. Handle `subClassOf` either by flattening inherited properties into each
   message, or by composition — pick one and document it.

Note: proto has no inheritance, so decide up front whether subclass properties
are flattened into descendants or referenced via a shared/base message.
