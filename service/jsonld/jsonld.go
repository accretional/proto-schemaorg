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
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	schemaorg "github.com/accretional/proto-schemaorg/proto"
)

// Item is one extracted top-level schema.org entity: its type local name and the
// typed message (a *dynamicpb.Message over the Schema<Type> descriptor).
type Item struct {
	Type    string
	Message protoreflect.ProtoMessage
}

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
		if it, ok := buildItem(obj); ok {
			items = append(items, it)
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

// buildItem builds a top-level Item from an object node, resolving its @type
// against the modeled type system. Returns ok=false for untyped/unmodeled nodes.
func buildItem(obj *jsonpb.Object) (Item, bool) {
	for _, t := range typesOf(obj) {
		md, err := schemaorg.MessageForType(t)
		if err != nil || md == nil {
			continue
		}
		msg := dynamicpb.NewMessage(md)
		fillObject(msg, obj)
		return Item{Type: t, Message: msg}, true
	}
	return Item{}, false
}

// fillObject populates a Schema<Type> message from a JSON-LD object node.
func fillObject(m protoreflect.Message, obj *jsonpb.Object) {
	md := m.Descriptor()
	for _, mem := range obj.GetSeq1().GetMember() {
		key := jsonparse.StringValue(mem.GetString_())
		val := mem.GetValue()
		switch {
		case key == "@id":
			if f := schemaorg.FieldByName(md, "id"); f != nil {
				m.Set(f, protoreflect.ValueOfString(scalar(val)))
			}
		case strings.HasPrefix(key, "@"): // @type, @context, @reverse, …
			continue
		default:
			if f := schemaorg.FieldByName(md, key); f != nil {
				setProperty(m, f, val)
			}
		}
	}
}

// setProperty appends val (a scalar, a node, or an array of them) to the
// repeated field f.
func setProperty(m protoreflect.Message, f protoreflect.FieldDescriptor, val *jsonpb.Value) {
	list := m.Mutable(f).List()
	for _, e := range values(val) {
		if f.Kind() == protoreflect.MessageKind {
			fillWrapper(list.AppendMutable().Message(), e)
		} else {
			list.Append(protoreflect.ValueOfString(scalar(e)))
		}
	}
}

// fillWrapper fills a property-wrapper message (a oneof over the property's
// ranges, plus sometimes a string text arm) from a single value e.
func fillWrapper(w protoreflect.Message, e *jsonpb.Value) {
	if obj := e.GetObject(); obj != nil {
		if arm := messageArm(w.Descriptor(), typesOf(obj)); arm != nil {
			fillObject(w.Mutable(arm).Message(), obj)
			return
		}
	}
	if arm := stringArm(w.Descriptor()); arm != nil {
		w.Set(arm, protoreflect.ValueOfString(scalar(e)))
	}
}

// messageArm returns the wrapper field whose message is Schema<one-of-types>,
// or the wrapper's only message field when no type matches (the common
// single-range case where @type is omitted or is a subtype).
func messageArm(wmd protoreflect.MessageDescriptor, types []string) protoreflect.FieldDescriptor {
	fs := wmd.Fields()
	want := make(map[string]bool, len(types))
	for _, t := range types {
		want["schema"+norm(t)] = true
	}
	var onlyMsg protoreflect.FieldDescriptor
	msgCount := 0
	for i := 0; i < fs.Len(); i++ {
		f := fs.Get(i)
		if f.Kind() != protoreflect.MessageKind {
			continue
		}
		msgCount++
		onlyMsg = f
		if want[norm(string(f.Message().Name()))] {
			return f
		}
	}
	if len(want) == 0 && msgCount == 1 {
		return onlyMsg
	}
	return nil
}

func stringArm(wmd protoreflect.MessageDescriptor) protoreflect.FieldDescriptor {
	fs := wmd.Fields()
	for i := 0; i < fs.Len(); i++ {
		if f := fs.Get(i); f.Kind() == protoreflect.StringKind {
			return f
		}
	}
	return nil
}

// ── AST helpers ─────────────────────────────────────────────────────────────

// typesOf returns the schema.org local type names of an object node's @type.
func typesOf(obj *jsonpb.Object) []string {
	return typeNames(member(obj, "@type"))
}

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
