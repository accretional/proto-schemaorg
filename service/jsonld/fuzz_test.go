package jsonld

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// FuzzExtractJSON feeds arbitrary bytes to Extract. Extract may return an error
// (malformed JSON, unmodeled types) but must never panic, and any items it does
// return must be well formed enough to reflect over without crashing. The parse
// path runs through accretional/proto-json, so this also exercises that parser's
// robustness on adversarial input.
func FuzzExtractJSON(f *testing.F) {
	for _, s := range seedCorpus() {
		f.Add([]byte(s))
	}
	// Fold in the committed generated JSON-LD inputs as additional seeds (the
	// paired *.expected.json files are excluded — they are not JSON-LD).
	if inputs, err := filepath.Glob("../../testing/testdata/generated-jsonld/*.json"); err == nil {
		for _, p := range inputs {
			if strings.HasSuffix(p, ".expected.json") {
				continue
			}
			if b, err := os.ReadFile(p); err == nil {
				f.Add(b)
			}
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		items, err := Extract(data)
		if err != nil {
			return // errors are acceptable; only panics are failures
		}
		// Touch every returned item so a malformed message surfaces here rather
		// than deep in a consumer.
		for _, it := range items {
			if it.Message == nil {
				continue
			}
			m := it.Message.ProtoReflect()
			m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
				_ = fd.Name()
				if fd.IsList() {
					_ = v.List().Len()
				}
				return true
			})
		}
	})
}

// seedCorpus is a spread of valid, edge, and malformed JSON-LD used to seed the
// fuzzer.
func seedCorpus() []string {
	return []string{
		// valid schema.org nodes
		`{"@context":"https://schema.org","@type":"Person","name":"Jane"}`,
		`{"@context":"https://schema.org","@type":"Product","name":"W","offers":{"@type":"Offer","price":9.99}}`,
		`{"@context":"https://schema.org","@graph":[{"@type":"Organization","name":"Acme"}]}`,
		`[{"@type":"Person","name":"A"},{"@type":"Person","name":"B"}]`,
		`{"@type":["Person","Organization"],"name":"Hybrid","@id":"https://x/1"}`,
		`{"@type":"Person","knowsLanguage":["en","fr"],"address":{"@type":"PostalAddress","postalCode":"1"}}`,
		`{"@type":"Person","name":{"@value":"Jane","@language":"en"}}`,
		`{"@type":"Person","url":{"@id":"https://x/2"}}`,
		// unmodeled / untyped
		`{"name":"no type"}`,
		`{"@type":"NotARealSchemaType","name":"x"}`,
		`{"@type":"","name":"empty type"}`,
		// structural edges
		`{}`,
		`[]`,
		`null`,
		`0`,
		`""`,
		`true`,
		`{"@graph":[]}`,
		`{"@graph":null}`,
		`{"@type":"Person","name":null}`,
		`{"@type":"Person","name":[]}`,
		`{"@type":"Person","address":{"@type":"Person","address":{"@type":"Person"}}}`,
		// deeply nested arrays/objects
		`{"@type":"Person","x":[[[[["deep"]]]]]}`,
		// malformed JSON
		`{`,
		`}`,
		`{"@type":}`,
		`{"@type":"Person",}`,
		`{"@type":"Person" "name":"x"}`,
		`{"@type":"Person","name":"unterminated`,
		`   `,
		`{"@type":"Person","n":1e999}`,
		`{"@type":"Person","n":-}`,
		"\x00\x01\x02",
		`{"@type":"Perß°n","name":"unicode"}`,
	}
}
