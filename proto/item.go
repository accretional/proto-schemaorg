package schemaorg

// item.go — the syntax-independent structured-data item, and the one mapper from
// it to the generated Schema<Type> proto messages. Both front-ends (JSON-LD and
// microdata) produce Items; Build turns an Item into a typed message. This is the
// single place that knows the type system's shape (property wrappers, oneof arms,
// datatype vs item fields).

import (
	"encoding/json"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// Item is one structured-data item in the syntax-independent (WHATWG-shaped)
// form: its schema.org type(s), an optional global id, and named properties
// whose values are scalars or nested items. It marshals to the WHATWG JSON form
// ({type, id, properties}), which is the ground-truth the microdata corpus uses.
type Item struct {
	Types []string           `json:"type,omitempty"`
	ID    string             `json:"id,omitempty"`
	Props map[string][]Value `json:"properties"`
}

// Value is a property value: either a scalar string or a nested Item.
type Value struct {
	Text string
	Item *Item
}

// MarshalJSON renders a Value as a bare string, or as the nested item object.
func (v Value) MarshalJSON() ([]byte, error) {
	if v.Item != nil {
		return json.Marshal(v.Item)
	}
	return json.Marshal(v.Text)
}

// Typed is a Build result: the resolved schema.org type local name and the
// dynamicpb message.
type Typed struct {
	Type    string
	Message protoreflect.ProtoMessage
}

// Build maps a generic Item to its Schema<Type> proto message, resolving the
// type against the modeled type system (the first of Item.Types that is modeled
// wins). Returns ok=false for an unmodeled/untyped item.
func Build(item *Item) (Typed, bool) {
	for _, t := range item.Types {
		md, err := MessageForType(LocalTypeName(t))
		if err != nil || md == nil {
			continue
		}
		msg := dynamicpb.NewMessage(md)
		fill(msg, item)
		return Typed{Type: LocalTypeName(t), Message: msg}, true
	}
	return Typed{}, false
}

// fill populates a Schema<Type> message from an Item.
func fill(m protoreflect.Message, item *Item) {
	md := m.Descriptor()
	if item.ID != "" {
		if f := FieldByName(md, "id"); f != nil {
			m.Set(f, protoreflect.ValueOfString(item.ID))
		}
	}
	for name, values := range item.Props {
		f := FieldByName(md, name)
		if f == nil {
			continue
		}
		list := m.Mutable(f).List()
		for _, v := range values {
			if f.Kind() == protoreflect.MessageKind {
				fillWrapper(list.AppendMutable().Message(), v)
			} else {
				list.Append(protoreflect.ValueOfString(v.Text))
			}
		}
	}
}

// fillWrapper fills a property-wrapper message (a oneof over the property's
// ranges, plus sometimes a text arm) from a single value.
func fillWrapper(w protoreflect.Message, v Value) {
	if v.Item != nil {
		if arm := messageArm(w.Descriptor(), v.Item.Types); arm != nil {
			fill(w.Mutable(arm).Message(), v.Item)
			return
		}
	}
	if arm := stringArm(w.Descriptor()); arm != nil {
		w.Set(arm, protoreflect.ValueOfString(v.Text))
	}
}

// messageArm returns the wrapper field whose message is Schema<one-of-types>, or
// the wrapper's only message field when no type matches (the common single-range
// case where the value's type is omitted or is a subtype).
func messageArm(wmd protoreflect.MessageDescriptor, types []string) protoreflect.FieldDescriptor {
	fs := wmd.Fields()
	want := make(map[string]bool, len(types))
	for _, t := range types {
		want["schema"+norm(LocalTypeName(t))] = true
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

// LocalTypeName reduces a schema.org type reference to its local name:
// "https://schema.org/Person" / "schema:Person" / "Person" → "Person".
func LocalTypeName(t string) string {
	if i := strings.LastIndexAny(t, "/:"); i >= 0 {
		return t[i+1:]
	}
	return t
}
