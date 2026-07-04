package jsonld

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// firstString returns the first value of message m's repeated string field
// named prop (case/separator-insensitive), or "".
func firstString(t *testing.T, m protoreflect.Message, prop string) string {
	t.Helper()
	f := fieldByName(t, m, prop)
	if f == nil || f.Kind() != protoreflect.StringKind {
		return ""
	}
	if !f.IsList() { // singular (e.g. the id field)
		return m.Get(f).String()
	}
	l := m.Get(f).List()
	if l.Len() == 0 {
		return ""
	}
	return l.Get(0).String()
}

// firstItem returns the first nested Schema<Range> message of m's property
// wrapper field named prop (following the wrapper's message arm), or nil.
func firstItem(t *testing.T, m protoreflect.Message, prop, arm string) protoreflect.Message {
	t.Helper()
	f := fieldByName(t, m, prop)
	if f == nil || f.Kind() != protoreflect.MessageKind {
		return nil
	}
	l := m.Get(f).List()
	if l.Len() == 0 {
		return nil
	}
	wrapper := l.Get(0).Message()
	af := fieldByName(t, wrapper, arm)
	if af == nil {
		return nil
	}
	return wrapper.Get(af).Message()
}

// firstWrapperText returns the string (text arm) of the first element of m's
// property-wrapper field named prop.
func firstWrapperText(t *testing.T, m protoreflect.Message, prop string) string {
	t.Helper()
	f := fieldByName(t, m, prop)
	if f == nil || f.Kind() != protoreflect.MessageKind {
		return ""
	}
	l := m.Get(f).List()
	if l.Len() == 0 {
		return ""
	}
	w := l.Get(0).Message()
	fs := w.Descriptor().Fields()
	for i := 0; i < fs.Len(); i++ {
		if sf := fs.Get(i); sf.Kind() == protoreflect.StringKind {
			return w.Get(sf).String()
		}
	}
	return ""
}

func fieldByName(t *testing.T, m protoreflect.Message, name string) protoreflect.FieldDescriptor {
	t.Helper()
	fs := m.Descriptor().Fields()
	target := norm(name)
	for i := 0; i < fs.Len(); i++ {
		if f := fs.Get(i); norm(string(f.Name())) == target {
			return f
		}
	}
	return nil
}

// norm lowercases and drops non-alphanumerics (test-local helper).
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

func TestExtractJSONPersonWithNestedAddress(t *testing.T) {
	src := `{
	  "@context": "https://schema.org",
	  "@type": "Person",
	  "@id": "https://example.com/jane#me",
	  "name": "Jane Doe",
	  "jobTitle": "Engineer",
	  "url": "https://example.com/jane",
	  "address": {
	    "@type": "PostalAddress",
	    "streetAddress": "123 Main St",
	    "addressLocality": "Springfield",
	    "postalCode": "12345"
	  }
	}`
	items, err := Extract([]byte(src))
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].Type != "Person" {
		t.Fatalf("type = %q, want Person", items[0].Type)
	}
	m := items[0].Message.ProtoReflect()

	if got := firstString(t, m, "id"); got != "https://example.com/jane#me" {
		t.Errorf("id = %q", got)
	}
	if got := firstString(t, m, "name"); got != "Jane Doe" {
		t.Errorf("name = %q, want Jane Doe", got)
	}
	// jobTitle's range is DefinedTerm|Text, so it is a wrapper field; a bare
	// string lands in the wrapper's text arm.
	if got := firstWrapperText(t, m, "jobTitle"); got != "Engineer" {
		t.Errorf("jobTitle = %q", got)
	}
	if got := firstString(t, m, "url"); got != "https://example.com/jane" {
		t.Errorf("url = %q", got)
	}

	addr := firstItem(t, m, "address", "SchemaPostalAddress")
	if addr == nil {
		t.Fatalf("address nested item not extracted")
	}
	if got := firstString(t, addr, "streetAddress"); got != "123 Main St" {
		t.Errorf("address.streetAddress = %q", got)
	}
	if got := firstString(t, addr, "postalCode"); got != "12345" {
		t.Errorf("address.postalCode = %q", got)
	}
}

func TestExtractJSONGraphArray(t *testing.T) {
	src := `{"@context":"https://schema.org","@graph":[
	  {"@type":"Organization","name":"Acme"},
	  {"@type":"WebSite","name":"Acme Site","url":"https://acme.example"}
	]}`
	items, err := Extract([]byte(src))
	if err != nil {
		t.Fatalf("ExtractJSON: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Type != "Organization" || items[1].Type != "WebSite" {
		t.Fatalf("types = %q, %q", items[0].Type, items[1].Type)
	}
	if got := firstString(t, items[0].Message.ProtoReflect(), "name"); got != "Acme" {
		t.Errorf("org name = %q", got)
	}
}

func TestExtractProductWithOffer(t *testing.T) {
	src := `{"@context":"https://schema.org","@type":"Product","name":"Widget",
	   "offers":{"@type":"Offer","price":"9.99","priceCurrency":"USD"}}`
	items, err := Extract([]byte(src))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(items) != 1 || items[0].Type != "Product" {
		t.Fatalf("got %d items (want 1 Product): %+v", len(items), items)
	}
	m := items[0].Message.ProtoReflect()
	if got := firstString(t, m, "name"); got != "Widget" {
		t.Errorf("product name = %q", got)
	}
	offer := firstItem(t, m, "offers", "SchemaOffer")
	if offer == nil {
		t.Fatalf("offers nested item not extracted")
	}
	if got := firstString(t, offer, "price"); got != "9.99" {
		t.Errorf("offer.price = %q", got)
	}
	if got := firstString(t, offer, "priceCurrency"); got != "USD" {
		t.Errorf("offer.priceCurrency = %q", got)
	}
}
