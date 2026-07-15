package microdata

import (
	"testing"

	"google.golang.org/protobuf/reflect/protoreflect"

	schemaorg "github.com/accretional/proto-schemaorg/proto"
)

// FuzzBuild exercises the shared item→message mapper (schemaorg.Build) — the
// single piece of product code both front-ends run — with adversarial Items
// decoded from fuzz bytes: arbitrary type names, property names, scalar and
// nested values, and nesting depth. Build must never panic, whether the type is
// modeled or not; when it does map an item, the resulting message must be safe
// to reflect over. The flaky HTML parser is deliberately avoided.
func FuzzBuild(f *testing.F) {
	seeds := [][]byte{
		[]byte("Person\x1ename\x1fJane"),
		[]byte("Product\x1ename\x1fWidget\x1eoffers\x1f\x02Offer\x1eprice\x1f9.99"),
		[]byte("PostalAddress\x1epostalCode\x1f12345\x1estreetAddress\x1f1 Main"),
		[]byte("NotAType\x1efoo\x1fbar"),
		[]byte(""),
		[]byte("\x1e\x1f"),
		[]byte("Person\x1e\x1f"),
		[]byte("Person\x1ejobTitle\x1fEngineer"),
		[]byte("Person\x1eaddress\x1f\x02Person\x1eaddress\x1f\x02Person"),
		[]byte("Person Organization\x1cThing\x1ename\x1fMulti"),
		[]byte("\x02\x02\x02\x1e\x1f\x02"),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		item := decodeItem(data, 0)
		if item == nil {
			return
		}
		typed, ok := schemaorg.Build(item)
		if !ok || typed.Message == nil {
			return
		}
		// Reflect over the produced message (and one level of wrappers) to shake
		// out any invalid field state Build might have left behind.
		m := typed.Message.ProtoReflect()
		m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			touch(fd, v)
			return true
		})
	})
}

func touch(fd protoreflect.FieldDescriptor, v protoreflect.Value) {
	if !fd.IsList() {
		return
	}
	l := v.List()
	for i := 0; i < l.Len(); i++ {
		e := l.Get(i)
		if fd.Kind() == protoreflect.MessageKind {
			e.Message().Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool { return true })
		}
	}
}

// decodeItem builds a schemaorg.Item from fuzz bytes with a small, panic-free
// grammar so the fuzzer explores many item shapes:
//
//	item      = typespec (RS prop)*
//	typespec  = name (FS name)*            // 0x1c-separated @type tokens
//	prop      = name US value              // RS=0x1e separates props, US=0x1f
//	value     = STX item | text            // 0x02 introduces a nested item
//
// Depth is capped so a hostile input cannot build an unbounded item tree.
func decodeItem(data []byte, depth int) *schemaorg.Item {
	if depth > 6 || len(data) == 0 {
		return nil
	}
	it := &schemaorg.Item{Props: map[string][]schemaorg.Value{}}
	segments := splitByte(data, 0x1e) // record separator between type-spec and props
	if len(segments) == 0 {
		return nil
	}
	for _, tok := range splitByte(segments[0], 0x1c) { // file separator between @type tokens
		if len(tok) > 0 {
			it.Types = append(it.Types, string(tok))
		}
	}
	for _, seg := range segments[1:] {
		key, val, found := cut(seg, 0x1f) // unit separator between name and value
		if !found || len(key) == 0 {
			continue
		}
		var value schemaorg.Value
		if len(val) > 0 && val[0] == 0x02 { // nested item marker
			if child := decodeItem(val[1:], depth+1); child != nil {
				value = schemaorg.Value{Item: child}
			} else {
				continue
			}
		} else {
			value = schemaorg.Value{Text: string(val)}
		}
		k := string(key)
		it.Props[k] = append(it.Props[k], value)
	}
	if len(it.Types) == 0 && len(it.Props) == 0 {
		return nil
	}
	return it
}

func splitByte(b []byte, sep byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == sep {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	out = append(out, b[start:])
	return out
}

func cut(b []byte, sep byte) (before, after []byte, found bool) {
	for i := 0; i < len(b); i++ {
		if b[i] == sep {
			return b[:i], b[i+1:], true
		}
	}
	return b, nil, false
}
