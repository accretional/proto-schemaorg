package jsonld

import "testing"

// TestExtractHTML exercises the full path: HTML → proto-html DOM → JSON-LD
// script bodies → schema.org messages. No byte-level parsing in this repo.
func TestExtractHTML(t *testing.T) {
	page := `<!DOCTYPE html><html lang="en"><head><title>Jane</title>
	  <script type="application/ld+json">
	  {"@context":"https://schema.org","@type":"Person","name":"Jane Doe",
	   "address":{"@type":"PostalAddress","postalCode":"12345"}}
	  </script>
	</head><body><p>ignored</p>
	  <script type="application/ld+json">{"@type":"Organization","name":"Acme"}</script>
	</body></html>`

	items, err := ExtractHTML([]byte(page))
	if err != nil {
		t.Fatalf("ExtractHTML: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (%+v)", len(items), items)
	}
	if items[0].Type != "Person" || items[1].Type != "Organization" {
		t.Fatalf("types = %q, %q", items[0].Type, items[1].Type)
	}
	if got := firstString(t, items[0].Message.ProtoReflect(), "name"); got != "Jane Doe" {
		t.Errorf("person name = %q", got)
	}
	addr := firstItem(t, items[0].Message.ProtoReflect(), "address", "SchemaPostalAddress")
	if addr == nil {
		t.Fatalf("address not extracted")
	}
	if got := firstString(t, addr, "postalCode"); got != "12345" {
		t.Errorf("postalCode = %q", got)
	}
	if got := firstString(t, items[1].Message.ProtoReflect(), "name"); got != "Acme" {
		t.Errorf("org name = %q", got)
	}
}
