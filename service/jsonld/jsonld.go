// Package jsonld extracts schema.org structured data from JSON-LD into the
// generated schema.org proto type system (see package schemaorg). JSON-LD is the
// dominant schema.org syntax on today's web, and it maps cleanly onto the type
// system: a node's @type selects the Schema<Type> message, @id fills the id
// field, and each remaining key is a property whose value is a scalar (a
// datatype field) or a nested node (a property-wrapper message holding a oneof
// over the property's ranges).
//
// Parsing is delegated to accretional/proto-json (JSON → the json.proto AST);
// this package never does its own byte-level parsing. It walks that AST and
// fills messages by reflection (dynamicpb against schemaorg.fdset), so adding
// types to the vocabulary needs only a genast regeneration, not a code change.
//
// Input here is raw JSON-LD. Extracting the <script type="application/ld+json">
// blocks out of an HTML page is a separate concern that belongs to the HTML
// grammar (accretional/proto-html) and is not done here.
package jsonld

import (
	"strings"

	"github.com/accretional/proto-json/jsonparse"
	jsonpb "github.com/accretional/proto-json/proto/gen"

	schemaorg "github.com/accretional/proto-schemaorg/proto"
)

// Item is one extracted top-level schema.org entity: its type local name and the
// typed message. It is schemaorg.Typed (the shared result of the item→message
// mapper both front-ends use).
type Item = schemaorg.Typed

// Extract parses a raw JSON-LD document (a single node, an array of nodes, or an
// object with an @graph) and returns its top-level schema.org items. Untyped or
// unmodeled nodes are skipped.
func Extract(src []byte) ([]Item, error) {
	jt, err := jsonparse.Parse(string(src))
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, obj := range topNodes(jt.GetValue()) {
		if t, ok := schemaorg.Build(toItem(obj)); ok {
			items = append(items, t)
		}
	}
	return items, nil
}

// topNodes flattens a JSON-LD document value into its top-level object nodes,
// following an @graph array when present.
func topNodes(v *jsonpb.Value) []*jsonpb.Object {
	switch {
	case v.GetObject() != nil:
		obj := v.GetObject()
		if g := member(obj, "@graph"); g != nil {
			return objectsOf(g)
		}
		return []*jsonpb.Object{obj}
	case v.GetArray() != nil:
		var out []*jsonpb.Object
		for _, e := range v.GetArray().GetSeq1().GetValue() {
			out = append(out, objectsOf(e)...)
		}
		return out
	}
	return nil
}

func objectsOf(v *jsonpb.Value) []*jsonpb.Object {
	if v.GetObject() != nil {
		return []*jsonpb.Object{v.GetObject()}
	}
	if v.GetArray() != nil {
		var out []*jsonpb.Object
		for _, e := range v.GetArray().GetSeq1().GetValue() {
			out = append(out, objectsOf(e)...)
		}
		return out
	}
	return nil
}

// toItem converts a JSON-LD object node into the generic schemaorg.Item: @type
// selects the type(s), @id the id, and each other key a property whose values
// are scalars or nested typed items. schemaorg.Build then maps it to a message.
func toItem(obj *jsonpb.Object) *schemaorg.Item {
	it := &schemaorg.Item{Props: map[string][]schemaorg.Value{}}
	it.Types = typeNames(member(obj, "@type"))
	if id := member(obj, "@id"); id != nil {
		it.ID = scalar(id)
	}
	for _, mem := range obj.GetSeq1().GetMember() {
		key := jsonparse.StringValue(mem.GetString_())
		if strings.HasPrefix(key, "@") { // @type, @id, @context, @reverse, …
			continue
		}
		for _, e := range values(mem.GetValue()) {
			it.Props[key] = append(it.Props[key], toValue(e))
		}
	}
	return it
}

// toValue maps a JSON-LD value to a generic Value: a nested typed node becomes a
// nested item; everything else (scalars, @value / @id objects) becomes text.
func toValue(v *jsonpb.Value) schemaorg.Value {
	if obj := v.GetObject(); obj != nil && len(typeNames(member(obj, "@type"))) > 0 {
		return schemaorg.Value{Item: toItem(obj)}
	}
	return schemaorg.Value{Text: scalar(v)}
}

// ── AST helpers ─────────────────────────────────────────────────────────────

func typeNames(v *jsonpb.Value) []string {
	if v == nil {
		return nil
	}
	if v.GetString_() != nil {
		return []string{localName(jsonparse.StringValue(v.GetString_()))}
	}
	if v.GetArray() != nil {
		var out []string
		for _, e := range v.GetArray().GetSeq1().GetValue() {
			out = append(out, typeNames(e)...)
		}
		return out
	}
	if obj := v.GetObject(); obj != nil { // {"@id": "…Type"}
		if id := member(obj, "@id"); id != nil && id.GetString_() != nil {
			return []string{localName(jsonparse.StringValue(id.GetString_()))}
		}
	}
	return nil
}

// member returns the value of obj's first member with the given key, or nil.
func member(obj *jsonpb.Object, key string) *jsonpb.Value {
	for _, m := range obj.GetSeq1().GetMember() {
		if jsonparse.StringValue(m.GetString_()) == key {
			return m.GetValue()
		}
	}
	return nil
}

// values normalizes a value to a slice, unwrapping a JSON array.
func values(v *jsonpb.Value) []*jsonpb.Value {
	if v.GetArray() != nil {
		return v.GetArray().GetSeq1().GetValue()
	}
	return []*jsonpb.Value{v}
}

// scalar reduces a value to its string form: a string/number literally, a
// boolean as "true"/"false", and a value/reference object via @value or @id.
func scalar(v *jsonpb.Value) string {
	switch {
	case v.GetString_() != nil:
		return jsonparse.StringValue(v.GetString_())
	case v.GetNumber() != nil:
		return jsonparse.NumberLiteral(v.GetNumber())
	case v.GetTrueKeyword() != nil:
		return "true"
	case v.GetFalseKeyword() != nil:
		return "false"
	case v.GetObject() != nil:
		obj := v.GetObject()
		if val := member(obj, "@value"); val != nil {
			return scalar(val)
		}
		if id := member(obj, "@id"); id != nil {
			return scalar(id)
		}
	}
	return ""
}

func localName(id string) string {
	if i := strings.LastIndexAny(id, "/:"); i >= 0 {
		return id[i+1:]
	}
	return id
}
