// Command gen is a schema-driven generator for JSON-LD extractor test cases.
//
// It walks the compiled schema.org type system (proto/schemaorg.fdset, via the
// schemaorg reflection helpers) rather than any hand-maintained fixture list, so
// every property it selects and every nested range it wires up is validated
// against the real messages the extractor fills. For each case it emits a paired:
//
//	testing/testdata/generated-jsonld/NNN-<Type>.json           (JSON-LD input)
//	testing/testdata/generated-jsonld/NNN-<Type>.expected.json  (expected items)
//
// The expected file is an *independent prediction* of jsonld.Extract's output —
// computed from schema.org semantics (first modeled @type wins; a scalar becomes
// [text]; a nested typed node recurses; @id fills id), NOT by running the
// extractor. service/jsonld/generated_test.go re-derives the same normal form
// from the extractor's messages and deep-compares, so a drift in the extractor
// fails the test.
//
// Run:  go run ./testing/gen
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	schemaorg "github.com/accretional/proto-schemaorg/proto"
)

const outDir = "testing/testdata/generated-jsonld"

// ── the case model ──────────────────────────────────────────────────────────

// item is one structured node to emit: its @type(s), optional @id, and ordered
// properties. It mirrors the shape a JSON-LD node and a schemaorg.Item share.
type item struct {
	types []string
	id    string
	props []prop
}

// prop is one property of an item: either scalar value(s) or a single nested
// item. camel is the schema.org (camelCase) property name written into the input.
type prop struct {
	camel   string
	scalars []scalarVal
	nested  *item
}

// scalarVal is a scalar property value. When raw != "", it is emitted into the
// JSON-LD input as a bare token (a number or a boolean) rather than a quoted
// string; text is always the string the extractor is expected to yield.
type scalarVal struct {
	text string
	raw  string
}

func I(types ...string) *item { return &item{types: types} }

func (it *item) ID(id string) *item { it.id = id; return it }

func (it *item) S(camel, val string) *item {
	it.props = append(it.props, prop{camel: camel, scalars: []scalarVal{{text: val}}})
	return it
}

func (it *item) Arr(camel string, vals ...string) *item {
	var sv []scalarVal
	for _, v := range vals {
		sv = append(sv, scalarVal{text: v})
	}
	it.props = append(it.props, prop{camel: camel, scalars: sv})
	return it
}

// Num adds a property whose input is the bare numeric token tok; the extractor
// reproduces the token verbatim, so it is also the expected string.
func (it *item) Num(camel, tok string) *item {
	it.props = append(it.props, prop{camel: camel, scalars: []scalarVal{{text: tok, raw: tok}}})
	return it
}

// Bool adds a boolean property (input token true/false, expected "true"/"false").
func (it *item) Bool(camel string, b bool) *item {
	tok := "false"
	if b {
		tok = "true"
	}
	it.props = append(it.props, prop{camel: camel, scalars: []scalarVal{{text: tok, raw: tok}}})
	return it
}

func (it *item) Nest(camel string, child *item) *item {
	it.props = append(it.props, prop{camel: camel, nested: child})
	return it
}

// ── build: one validated pass yielding input + predicted-expected together ───

var dropped []string // props skipped by validation, reported at the end

// build walks it once, validating each property against the winning type's
// message. Valid properties are added to both the JSON-LD input map and the
// predicted expected item; invalid ones are skipped from both so the two stay in
// lock-step. ok is false for an unmodeled type or a node with no valid property.
func build(it *item, path string) (inputMap map[string]any, expected map[string]any, ok bool) {
	win, md := winner(it.types)
	if md == nil {
		dropped = append(dropped, fmt.Sprintf("%s: no modeled type in %v", path, it.types))
		return nil, nil, false
	}
	inputMap = map[string]any{}
	if len(it.types) == 1 {
		inputMap["@type"] = it.types[0]
	} else {
		inputMap["@type"] = toAny(it.types)
	}
	expected = map[string]any{"type": win}
	if it.id != "" {
		inputMap["@id"] = it.id
		expected["id"] = it.id
	}
	props := map[string]any{}
	for _, p := range it.props {
		fd := schemaorg.FieldByName(md, p.camel)
		if fd == nil {
			dropped = append(dropped, fmt.Sprintf("%s.%s: no such field on %s", path, p.camel, win))
			continue
		}
		fn := string(fd.Name())
		if p.nested != nil {
			nwin, _ := winner(p.nested.types)
			if fd.Kind() != protoreflect.MessageKind || armFor(fd, nwin) == nil {
				dropped = append(dropped, fmt.Sprintf("%s.%s: no %s arm", path, p.camel, nwin))
				continue
			}
			cin, cexp, cok := build(p.nested, path+"."+p.camel)
			if !cok {
				continue
			}
			inputMap[p.camel] = cin
			props[fn] = []any{cexp}
			continue
		}
		if !scalarOK(fd) {
			dropped = append(dropped, fmt.Sprintf("%s.%s: field is a class-only wrapper (no scalar arm)", path, p.camel))
			continue
		}
		if len(p.scalars) == 1 {
			inputMap[p.camel] = scalarInput(p.scalars[0])
		} else {
			var arr []any
			for _, s := range p.scalars {
				arr = append(arr, scalarInput(s))
			}
			inputMap[p.camel] = arr
		}
		var pv []any
		for _, s := range p.scalars {
			pv = append(pv, s.text)
		}
		props[fn] = pv
	}
	if len(props) == 0 {
		return nil, nil, false
	}
	expected["props"] = props
	return inputMap, expected, true
}

func scalarInput(s scalarVal) any {
	if s.raw != "" {
		return json.RawMessage(s.raw)
	}
	return s.text
}

// ── type-system reflection helpers ───────────────────────────────────────────

// winner returns the local name and message of the first modeled type in types.
func winner(types []string) (string, protoreflect.MessageDescriptor) {
	for _, t := range types {
		ln := schemaorg.LocalTypeName(t)
		md, err := schemaorg.MessageForType(ln)
		if err == nil && md != nil {
			return ln, md
		}
	}
	return "", nil
}

// armFor returns the property-wrapper's message field for range type r (matched
// on the Schema<r> message name), or nil.
func armFor(fd protoreflect.FieldDescriptor, r string) protoreflect.FieldDescriptor {
	if fd.Kind() != protoreflect.MessageKind {
		return nil
	}
	want := norm("Schema" + r)
	fs := fd.Message().Fields()
	for i := 0; i < fs.Len(); i++ {
		f := fs.Get(i)
		if f.Kind() == protoreflect.MessageKind && norm(string(f.Message().Name())) == want {
			return f
		}
	}
	return nil
}

// scalarOK reports whether a bare scalar routes to a value the extractor keeps:
// a plain string field, or a wrapper that carries a string (text) arm.
func scalarOK(fd protoreflect.FieldDescriptor) bool {
	switch fd.Kind() {
	case protoreflect.StringKind:
		return true
	case protoreflect.MessageKind:
		fs := fd.Message().Fields()
		for i := 0; i < fs.Len(); i++ {
			if fs.Get(i).Kind() == protoreflect.StringKind {
				return true
			}
		}
	}
	return false
}

func norm(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
		}
	}
	return b.String()
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// ── emission ─────────────────────────────────────────────────────────────────

type outCase struct {
	note  string
	typ   string
	input map[string]any // full JSON-LD document (single node or an @graph)
	items []any          // predicted expected items
}

// single builds an outCase from a single top-level node.
func single(note string, it *item) (outCase, bool) {
	in, exp, ok := build(it, it.types[0])
	if !ok {
		return outCase{}, false
	}
	in["@context"] = "https://schema.org"
	win, _ := winner(it.types)
	return outCase{note: note, typ: win, input: in, items: []any{exp}}, true
}

// graph builds an outCase whose input wraps several nodes in an @graph.
func graph(note, typ string, its ...*item) (outCase, bool) {
	var nodes []any
	var items []any
	for _, it := range its {
		in, exp, ok := build(it, it.types[0])
		if !ok {
			return outCase{}, false
		}
		nodes = append(nodes, in)
		items = append(items, exp)
	}
	doc := map[string]any{"@context": "https://schema.org", "@graph": nodes}
	return outCase{note: note, typ: typ, input: doc, items: items}, true
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	if _, err := schemaorg.File(); err != nil {
		return fmt.Errorf("load type system: %w", err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	// Clean previously generated files (this directory is exclusively ours).
	olds, _ := filepath.Glob(filepath.Join(outDir, "*.json"))
	for _, f := range olds {
		if err := os.Remove(f); err != nil {
			return err
		}
	}

	cases := buildCases()

	written := 0
	for i, c := range cases {
		base := fmt.Sprintf("%03d-%s", i+1, c.typ)
		inBytes, err := json.MarshalIndent(c.input, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, base+".json"), append(inBytes, '\n'), 0o644); err != nil {
			return err
		}
		expDoc := map[string]any{"note": c.note, "items": c.items}
		expBytes, err := json.MarshalIndent(expDoc, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outDir, base+".expected.json"), append(expBytes, '\n'), 0o644); err != nil {
			return err
		}
		written++
	}

	fmt.Printf("generated %d cases into %s\n", written, outDir)
	if len(dropped) > 0 {
		sort.Strings(dropped)
		fmt.Printf("%d property(ies) skipped by validation:\n", len(dropped))
		for _, d := range dropped {
			fmt.Println("  -", d)
		}
	}
	return nil
}

// buildCases assembles the curated + schema-walked spread. Each spec is validated
// in build(); an invalid one is dropped with a note rather than emitted.
func buildCases() []outCase {
	var specs []func() (outCase, bool)

	add := func(note string, it *item) {
		specs = append(specs, func() (outCase, bool) { return single(note, it) })
	}
	addGraph := func(note, typ string, its ...*item) {
		specs = append(specs, func() (outCase, bool) { return graph(note, typ, its...) })
	}

	// ── Person ──
	add("Person: scalars + @id",
		I("Person").ID("https://example.com/jane#me").
			S("name", "Jane Doe").S("givenName", "Jane").S("familyName", "Doe").
			S("email", "jane@example.com").S("telephone", "+1-555-0100").
			S("url", "https://example.com/jane"))
	add("Person: bare string into a wrapper's text arm (jobTitle range DefinedTerm|Text)",
		I("Person").S("name", "Jane Doe").S("jobTitle", "Software Engineer"))
	add("Person: nested PostalAddress",
		I("Person").S("name", "Jane Doe").
			Nest("address", I("PostalAddress").
				S("streetAddress", "123 Main St").S("addressLocality", "Springfield").
				S("postalCode", "12345")))
	add("Person: nested Organization via worksFor",
		I("Person").S("name", "Jane Doe").
			Nest("worksFor", I("Organization").S("name", "Acme Corp").S("url", "https://acme.example")))
	add("Person: repeated (array) scalar property",
		I("Person").S("name", "Jane Doe").
			Arr("sameAs", "https://a.example/jane", "https://b.example/jane"))
	add("Person: multi-type @type array (first modeled wins)",
		I("Person", "Organization").S("name", "Hybrid Entity").S("url", "https://hybrid.example"))
	add("Person: three-level nesting (Person > Organization > PostalAddress)",
		I("Person").S("name", "Jane Doe").
			Nest("worksFor", I("Organization").S("name", "Acme Corp").
				Nest("address", I("PostalAddress").S("streetAddress", "1 Industrial Way").S("postalCode", "99999"))))

	// ── Product ──
	add("Product: scalars + @id",
		I("Product").ID("https://example.com/widget").
			S("name", "Super Widget").S("sku", "SW-001").S("gtin13", "0614141999996").
			S("color", "blue").S("mpn", "925872").S("url", "https://example.com/widget"))
	add("Product: boolean scalar",
		I("Product").S("name", "Kids Widget").Bool("isFamilyFriendly", true))
	add("Product: nested Offer with a numeric price",
		I("Product").S("name", "Super Widget").
			Nest("offers", I("Offer").Num("price", "9.99").S("priceCurrency", "USD")))
	add("Product: nested AggregateRating (numeric fields)",
		I("Product").S("name", "Super Widget").
			Nest("aggregateRating", I("AggregateRating").
				Num("ratingValue", "4.5").Num("reviewCount", "100").Num("bestRating", "5")))
	add("Product: nested Brand (arm chosen among Brand|Organization)",
		I("Product").S("name", "Super Widget").
			Nest("brand", I("Brand").S("name", "Acme").S("slogan", "We make widgets")))
	add("Product: deep nesting (Product > Review > Rating)",
		I("Product").S("name", "Super Widget").
			Nest("review", I("Review").S("reviewBody", "Great widget.").
				Nest("reviewRating", I("Rating").Num("ratingValue", "5").Num("bestRating", "5"))))

	// ── Offer ──
	add("Offer: scalars + @id",
		I("Offer").ID("https://example.com/offer/1").
			Num("price", "19.99").S("priceCurrency", "USD").
			S("availabilityStarts", "2026-01-01").S("sku", "SW-001").S("url", "https://example.com/offer/1"))
	add("Offer: nested seller Organization",
		I("Offer").Num("price", "19.99").S("priceCurrency", "USD").
			Nest("seller", I("Organization").S("name", "Acme Store")))
	add("Offer: nested itemOffered Product (arm chosen among many ranges)",
		I("Offer").Num("price", "19.99").S("priceCurrency", "USD").
			Nest("itemOffered", I("Product").S("name", "Super Widget").S("sku", "SW-001")))

	// ── Recipe ──
	add("Recipe: ISO-8601 duration + text scalars",
		I("Recipe").S("name", "Chocolate Cake").S("cookTime", "PT45M").S("prepTime", "PT20M").
			S("totalTime", "PT65M").S("recipeCategory", "Dessert").S("recipeCuisine", "French").
			S("url", "https://example.com/cake"))
	add("Recipe: nested NutritionInformation",
		I("Recipe").S("name", "Chocolate Cake").
			Nest("nutrition", I("NutritionInformation").S("calories", "350 calories").S("fatContent", "12 g")))
	add("Recipe: nested author Person",
		I("Recipe").S("name", "Chocolate Cake").
			Nest("author", I("Person").S("name", "Chef Pierre")))
	add("Recipe: nested AggregateRating",
		I("Recipe").S("name", "Chocolate Cake").
			Nest("aggregateRating", I("AggregateRating").Num("ratingValue", "4.8").Num("reviewCount", "52")))

	// ── Organization ──
	add("Organization: scalars + @id",
		I("Organization").ID("https://acme.example/#org").
			S("name", "Acme Corp").S("legalName", "Acme Corporation Inc.").
			S("email", "info@acme.example").S("telephone", "+1-555-0199").
			S("foundingDate", "1985-04-01").S("url", "https://acme.example"))
	add("Organization: nested PostalAddress",
		I("Organization").S("name", "Acme Corp").
			Nest("address", I("PostalAddress").S("streetAddress", "500 Corporate Blvd").
				S("addressLocality", "Metropolis").S("postalCode", "54321")))
	add("Organization: nested ImageObject via logo",
		I("Organization").S("name", "Acme Corp").
			Nest("logo", I("ImageObject").S("contentUrl", "https://acme.example/logo.png").S("name", "Acme logo")))
	add("Organization: nested ContactPoint",
		I("Organization").S("name", "Acme Corp").
			Nest("contactPoint", I("ContactPoint").S("telephone", "+1-555-0111").
				S("contactType", "customer service").S("email", "support@acme.example")))

	// ── Event ──
	add("Event: date scalars + @id",
		I("Event").ID("https://example.com/event/1").
			S("name", "Widget Expo").S("startDate", "2026-09-01T09:00").S("endDate", "2026-09-03T17:00").
			S("url", "https://example.com/event/1"))
	add("Event: nested Place via location",
		I("Event").S("name", "Widget Expo").
			Nest("location", I("Place").S("name", "Convention Center").S("telephone", "+1-555-0122")))
	add("Event: nested PostalAddress via location (multi-arm choice by @type)",
		I("Event").S("name", "Widget Expo").
			Nest("location", I("PostalAddress").S("streetAddress", "1 Expo Way").S("postalCode", "10001")))
	add("Event: two nested items (offers + performer)",
		I("Event").S("name", "Widget Expo").
			Nest("offers", I("Offer").Num("price", "25.00").S("priceCurrency", "USD")).
			Nest("performer", I("Person").S("name", "The Widget Band")))

	// ── PostalAddress ──
	add("PostalAddress: scalars",
		I("PostalAddress").S("streetAddress", "123 Main St").S("addressLocality", "Springfield").
			S("postalCode", "12345").S("telephone", "+1-555-0100").S("name", "HQ"))
	add("PostalAddress: nested Country via addressCountry",
		I("PostalAddress").S("streetAddress", "123 Main St").
			Nest("addressCountry", I("Country").S("name", "US")))

	// ── AggregateRating ──
	add("AggregateRating: numeric scalars",
		I("AggregateRating").Num("ratingValue", "4.5").Num("reviewCount", "100").
			Num("ratingCount", "120").Num("bestRating", "5").Num("worstRating", "1"))
	add("AggregateRating: nested Thing via itemReviewed",
		I("AggregateRating").Num("ratingValue", "4.5").
			Nest("itemReviewed", I("Thing").S("name", "Super Widget")))

	// ── spread across more types ──
	add("Review: text scalars + nested author and reviewRating",
		I("Review").S("name", "My review").S("reviewBody", "Solid product.").S("datePublished", "2026-05-01").
			Nest("author", I("Person").S("name", "A. Reviewer")).
			Nest("reviewRating", I("Rating").Num("ratingValue", "4").Num("bestRating", "5")))
	add("Rating: numeric scalars",
		I("Rating").Num("ratingValue", "3").Num("bestRating", "5").Num("worstRating", "1"))
	add("Place: geo scalars + nested address and GeoCoordinates",
		I("Place").S("name", "Central Park").
			Nest("address", I("PostalAddress").S("streetAddress", "59th to 110th St").S("postalCode", "10022")).
			Nest("geo", I("GeoCoordinates").Num("latitude", "40.7829").Num("longitude", "-73.9654")))
	add("Book: identifiers + nested author",
		I("Book").S("name", "Widgets: A History").S("isbn", "978-3-16-148410-0").
			S("bookEdition", "2nd").S("numberOfPages", "320").S("datePublished", "2020-01-01").
			Nest("author", I("Person").S("name", "Jane Author")))
	add("Movie: nested director + AggregateRating",
		I("Movie").S("name", "The Widget").S("datePublished", "2024-06-01").
			Nest("director", I("Person").S("name", "D. Rector")).
			Nest("aggregateRating", I("AggregateRating").Num("ratingValue", "7.8").Num("reviewCount", "1500")))
	add("LocalBusiness: business scalars + nested address",
		I("LocalBusiness").S("name", "Joe's Hardware").S("priceRange", "$$").S("telephone", "+1-555-0133").
			S("openingHours", "Mo-Fr 09:00-17:00").
			Nest("address", I("PostalAddress").S("streetAddress", "7 Market St").S("postalCode", "60601")))
	add("Restaurant: cuisine + boolean + nested address",
		I("Restaurant").S("name", "Bistro Widget").S("servesCuisine", "French").S("priceRange", "$$$").
			Bool("acceptsReservations", true).S("telephone", "+1-555-0144").
			Nest("address", I("PostalAddress").S("streetAddress", "42 Rue de Widget").S("postalCode", "75001")))
	add("Article: text scalars + nested author and publisher",
		I("Article").S("name", "Widgets Explained").S("headline", "Everything about widgets").
			S("articleBody", "Widgets are ...").S("datePublished", "2026-03-15").
			Nest("author", I("Person").S("name", "Staff Writer")).
			Nest("publisher", I("Organization").S("name", "Widget Times")))
	add("WebSite: simple scalars",
		I("WebSite").S("name", "Acme Site").S("url", "https://acme.example").S("issn", "1234-5678"))
	add("ImageObject: media scalars",
		I("ImageObject").S("name", "Product photo").S("contentUrl", "https://cdn.example/p.jpg").
			S("uploadDate", "2026-02-02").S("encodingFormat", "image/jpeg"))
	add("JobPosting: employment scalars",
		I("JobPosting").S("title", "Widget Engineer").S("datePosted", "2026-06-01").
			S("employmentType", "FULL_TIME").S("salaryCurrency", "USD").S("validThrough", "2026-08-01"))
	add("MonetaryAmount: numeric range scalars",
		I("MonetaryAmount").S("currency", "USD").Num("minValue", "1000").Num("maxValue", "5000"))
	add("NutritionInformation: measurement scalars",
		I("NutritionInformation").S("calories", "250 calories").S("fatContent", "8 g").
			S("proteinContent", "12 g").S("servingSize", "1 cup"))
	add("Brand: simple scalars",
		I("Brand").S("name", "Acme").S("slogan", "Widgets that work").S("url", "https://acme.example"))
	add("ContactPoint: contact scalars",
		I("ContactPoint").S("telephone", "+1-555-0100").S("contactType", "sales").S("email", "sales@acme.example"))
	add("GeoCoordinates: numeric coordinates",
		I("GeoCoordinates").Num("latitude", "40.7128").Num("longitude", "-74.006").Num("elevation", "10"))

	// ── @graph documents (multiple top-level items, document order) ──
	addGraph("@graph: Organization + WebSite", "graph",
		I("Organization").S("name", "Acme Corp").S("url", "https://acme.example"),
		I("WebSite").S("name", "Acme Site").S("url", "https://acme.example"))
	addGraph("@graph: Person + Product-with-Offer", "graph",
		I("Person").S("name", "Jane Doe"),
		I("Product").S("name", "Super Widget").
			Nest("offers", I("Offer").Num("price", "9.99").S("priceCurrency", "USD")))

	var cases []outCase
	for _, mk := range specs {
		if c, ok := mk(); ok {
			cases = append(cases, c)
		}
	}
	return cases
}
