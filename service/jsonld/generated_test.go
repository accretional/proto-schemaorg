package jsonld

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestGeneratedCorpus runs every schema-driven fixture in
// testing/testdata/generated-jsonld (produced by `go run ./testing/gen`) through
// jsonld.Extract and checks the extracted items against the committed expected
// JSON. The expected file is an independent prediction of the extractor's output
// (see testing/gen); this test re-derives the same normal form from the filled
// proto messages and deep-compares, so any drift in the extractor fails here.
func TestGeneratedCorpus(t *testing.T) {
	dir := "../../testing/testdata/generated-jsonld"
	expects, err := filepath.Glob(filepath.Join(dir, "*.expected.json"))
	if err != nil || len(expects) == 0 {
		t.Fatalf("no generated fixtures in %s (%v) — run: go run ./testing/gen", dir, err)
	}
	sort.Strings(expects)

	for _, expPath := range expects {
		name := strings.TrimSuffix(filepath.Base(expPath), ".expected.json")
		t.Run(name, func(t *testing.T) {
			inPath := filepath.Join(dir, name+".json")
			inBytes, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			expBytes, err := os.ReadFile(expPath)
			if err != nil {
				t.Fatalf("read expected: %v", err)
			}
			var exp struct {
				Items []any `json:"items"`
			}
			if err := json.Unmarshal(expBytes, &exp); err != nil {
				t.Fatalf("expected json: %v", err)
			}

			items, err := Extract(inBytes)
			if err != nil {
				t.Fatalf("extract: %v", err)
			}

			got := normItems(t, items)
			want := exp.Items
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

// normItems converts the extractor's typed items into the generic normal form
// (a JSON tree of maps/slices/strings) the expected files use, round-tripping
// through JSON so leaf types match the unmarshaled expectation exactly.
func normItems(t *testing.T, items []Item) []any {
	t.Helper()
	out := make([]any, 0, len(items))
	for _, it := range items {
		out = append(out, normTopItem(it))
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	var v []any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func normTopItem(it Item) map[string]any {
	out := map[string]any{"type": it.Type}
	fillNorm(it.Message.ProtoReflect(), out)
	return out
}

// normMessage normalizes a nested Schema<Range> message: its type is the message
// name with the "Schema" prefix stripped (SchemaPostalAddress → PostalAddress).
func normMessage(m protoreflect.Message) map[string]any {
	out := map[string]any{"type": strings.TrimPrefix(string(m.Descriptor().Name()), "Schema")}
	fillNorm(m, out)
	return out
}

// fillNorm populates out with the message's id (if set) and a "props" map of
// field name → list of normalized values.
func fillNorm(m protoreflect.Message, out map[string]any) {
	props := map[string]any{}
	m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if string(f.Name()) == "id" && !f.IsList() {
			out["id"] = v.String()
			return true
		}
		var vals []any
		if f.IsList() {
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				vals = append(vals, normValue(f, l.Get(i)))
			}
		} else {
			vals = append(vals, normValue(f, v))
		}
		props[string(f.Name())] = vals
		return true
	})
	if len(props) > 0 {
		out["props"] = props
	}
}

// normValue reduces one field value to its normal form: a scalar field yields
// its string; a wrapper field follows its set arm (text arm → string, message
// arm → a nested item).
func normValue(f protoreflect.FieldDescriptor, v protoreflect.Value) any {
	if f.Kind() != protoreflect.MessageKind {
		return v.String()
	}
	w := v.Message()
	arm := setArm(w)
	if arm == nil {
		return ""
	}
	if arm.Kind() == protoreflect.MessageKind {
		return normMessage(w.Get(arm).Message())
	}
	return w.Get(arm).String()
}

// setArm returns the single populated field of a property-wrapper message.
func setArm(m protoreflect.Message) protoreflect.FieldDescriptor {
	var found protoreflect.FieldDescriptor
	m.Range(func(f protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		found = f
		return false
	})
	return found
}
