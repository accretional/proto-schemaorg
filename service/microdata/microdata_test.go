package microdata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
)

// TestSyntheticCorpus runs every hand-authored fixture in
// testing/testdata/synthetic and checks the extracted items against the
// expected WHATWG JSON (the ground truth).
func TestSyntheticCorpus(t *testing.T) {
	dir := "../../testing/testdata/synthetic"
	htmls, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil || len(htmls) == 0 {
		t.Fatalf("no fixtures in %s (%v)", dir, err)
	}
	sort.Strings(htmls)

	for _, htmlPath := range htmls {
		name := strings.TrimSuffix(filepath.Base(htmlPath), ".html")
		t.Run(name, func(t *testing.T) {
			jsonPath := strings.TrimSuffix(htmlPath, ".html") + ".json"
			htmlSrc, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatal(err)
			}
			expRaw, err := os.ReadFile(jsonPath)
			if err != nil {
				t.Fatal(err)
			}
			var exp map[string]any
			if err := json.Unmarshal(expRaw, &exp); err != nil {
				t.Fatalf("expected json: %v", err)
			}
			base, _ := exp["_base"].(string)

			doc, err := Extract(htmlSrc, base)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}

			got := normalize(t, doc)
			want := exp["items"]
			if want == nil {
				want = []any{}
			}
			if !reflect.DeepEqual(got, want) {
				gb, _ := json.MarshalIndent(got, "", "  ")
				wb, _ := json.MarshalIndent(want, "", "  ")
				t.Errorf("items mismatch\n--- got ---\n%s\n--- want ---\n%s", gb, wb)
			}
		})
	}
}

// normalize marshals the extracted items and re-parses them to a generic value,
// so the comparison ignores Go types and JSON key order.
func normalize(t *testing.T, doc *Document) any {
	t.Helper()
	b, err := json.Marshal(doc.Items)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Items == nil {
		return []any{}
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

// TestMessages proves microdata items now map to typed Schema<Type> proto
// messages via the shared schemaorg.Build.
func TestMessages(t *testing.T) {
	page := `<!DOCTYPE html><html><head></head><body>` +
		`<div itemscope itemtype="https://schema.org/Person">` +
		`<span itemprop="name">Jane Doe</span>` +
		`<div itemprop="address" itemscope itemtype="https://schema.org/PostalAddress">` +
		`<span itemprop="postalCode">12345</span></div></div></body></html>`
	msgs, err := Messages([]byte(page), "https://example.com/")
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Type != "Person" {
		t.Fatalf("got %d msgs (want 1 Person): %+v", len(msgs), msgs)
	}
	b, err := protojson.Marshal(msgs[0].Message)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{"Jane Doe", "12345"} {
		if !strings.Contains(s, want) {
			t.Errorf("typed message missing %q:\n%s", want, s)
		}
	}
}
