// Package microdata extracts HTML Microdata (itemscope/itemtype/itemprop/
// itemid/itemref) per the WHATWG algorithm, over accretional/proto-html's DOM
// (no byte-level HTML parsing here). It produces the WHATWG structured form —
// items of {type, id, properties} where a value is a string or a nested item —
// the same syntax-independent shape JSON-LD yields, so both feed the one
// schema.org type system.
package microdata

import (
	"net/url"
	"sort"
	"strings"

	"github.com/accretional/proto-html/dom"
	"github.com/accretional/proto-html/htmlparse"
	schemaorg "github.com/accretional/proto-schemaorg/proto"
)

// Document is the top-level extraction result: the syntax-independent items
// (schemaorg.Item), which marshal to the WHATWG JSON form and map to typed
// proto messages via schemaorg.Build (see Messages).
type Document struct {
	Items []*schemaorg.Item `json:"items"`
}

// Messages runs Extract and maps every item to its typed Schema<Type> proto
// message (skipping unmodeled types).
func Messages(htmlSrc []byte, baseURL string) ([]schemaorg.Typed, error) {
	doc, err := Extract(htmlSrc, baseURL)
	if err != nil {
		return nil, err
	}
	var out []schemaorg.Typed
	for _, it := range doc.Items {
		if t, ok := schemaorg.Build(it); ok {
			out = append(out, t)
		}
	}
	return out, nil
}

// Extract parses HTML via proto-html and runs the microdata extraction. baseURL
// resolves URL-valued properties and itemid; if "", a <base href> in the
// document is used, else URLs are left as written.
func Extract(htmlSrc []byte, baseURL string) (*Document, error) {
	root, err := htmlparse.ParseDOM(string(htmlSrc))
	if err != nil {
		return nil, err
	}
	x := &extractor{
		order: map[*dom.Node]int{},
		ids:   map[string]*dom.Node{},
		base:  baseURL,
	}
	i := 0
	root.Walk(func(n *dom.Node) {
		x.order[n] = i
		i++
		if n.Type == dom.ElementNode {
			if id, ok := n.AttrVal("id"); ok {
				if _, seen := x.ids[id]; !seen {
					x.ids[id] = n // first element with the id wins
				}
			}
			if x.base == "" && n.Data == "base" {
				if href, ok := n.AttrVal("href"); ok {
					x.base = href
				}
			}
		}
	})

	var items []*schemaorg.Item
	root.Walk(func(n *dom.Node) {
		if isItem(n) && !hasAttr(n, "itemprop") { // top-level items: itemscope, no itemprop
			items = append(items, x.object(n, nil))
		}
	})
	return &Document{Items: items}, nil
}

type extractor struct {
	order map[*dom.Node]int
	ids   map[string]*dom.Node
	base  string
}

// object builds the item rooted at item; memory is the chain of itemscope
// ancestors (for the cycle guard).
func (x *extractor) object(item *dom.Node, memory []*dom.Node) *schemaorg.Item {
	it := &schemaorg.Item{Props: map[string][]schemaorg.Value{}}
	if v, ok := item.AttrVal("itemtype"); ok {
		it.Types = tokens(v)
	}
	if v, ok := item.AttrVal("itemid"); ok && len(it.Types) > 0 {
		it.ID = x.resolve(v)
	}
	mem := append(append([]*dom.Node{}, memory...), item)

	for _, p := range x.properties(item) {
		v, _ := p.AttrVal("itemprop")
		for _, name := range tokens(v) {
			var val schemaorg.Value
			switch {
			case hasAttr(p, "itemscope") && contains(mem, p):
				val = schemaorg.Value{Text: "ERROR"} // cycle guard (WHATWG)
			case hasAttr(p, "itemscope"):
				val = schemaorg.Value{Item: x.object(p, mem)}
			default:
				val = schemaorg.Value{Text: x.propertyValue(p)}
			}
			it.Props[name] = append(it.Props[name], val)
		}
	}
	return it
}

// properties returns the property elements of an item, in tree order: crawl the
// item's descendants (not descending into nested items) plus itemref'd elements,
// keeping those that carry an itemprop.
func (x *extractor) properties(root *dom.Node) []*dom.Node {
	memory := map[*dom.Node]bool{root: true}
	var pending []*dom.Node
	pending = append(pending, childElements(root)...)
	if v, ok := root.AttrVal("itemref"); ok {
		for _, id := range tokens(v) {
			if e := x.ids[id]; e != nil {
				pending = append(pending, e)
			}
		}
	}

	var results []*dom.Node
	for len(pending) > 0 {
		cur := pending[0]
		pending = pending[1:]
		if memory[cur] {
			continue // already visited (itemref/cycle guard)
		}
		memory[cur] = true
		if !hasAttr(cur, "itemscope") {
			pending = append(pending, childElements(cur)...)
		}
		if v, ok := cur.AttrVal("itemprop"); ok && strings.TrimSpace(v) != "" {
			results = append(results, cur)
		}
	}
	sort.SliceStable(results, func(i, j int) bool { return x.order[results[i]] < x.order[results[j]] })
	return results
}

// propertyValue returns the string value of a (non-item) property element per
// the WHATWG value cascade. URL-valued elements are resolved against the base;
// a missing source attribute yields "".
func (x *extractor) propertyValue(p *dom.Node) string {
	switch p.Data {
	case "meta":
		return attr(p, "content")
	case "audio", "embed", "iframe", "img", "source", "track", "video":
		return x.urlAttr(p, "src")
	case "a", "area", "link":
		return x.urlAttr(p, "href")
	case "object":
		return x.urlAttr(p, "data")
	case "data", "meter":
		return attr(p, "value")
	case "time":
		if v, ok := p.AttrVal("datetime"); ok {
			return v
		}
		return p.Text()
	default:
		return p.Text()
	}
}

func (x *extractor) urlAttr(p *dom.Node, name string) string {
	v, ok := p.AttrVal(name)
	if !ok {
		return "" // no attribute → empty string, not the base
	}
	return x.resolve(v)
}

// resolve resolves ref against the base URL; if either is unparseable or base is
// empty, ref is returned as written.
func (x *extractor) resolve(ref string) string {
	if x.base == "" {
		return ref
	}
	b, err := url.Parse(x.base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// ── helpers ─────────────────────────────────────────────────────────────────

func isItem(n *dom.Node) bool {
	return n.Type == dom.ElementNode && hasAttr(n, "itemscope")
}

func hasAttr(n *dom.Node, key string) bool {
	_, ok := n.AttrVal(key)
	return ok
}

func attr(n *dom.Node, key string) string {
	v, _ := n.AttrVal(key)
	return v
}

func childElements(n *dom.Node) []*dom.Node {
	var out []*dom.Node
	for _, c := range n.Children {
		if c.Type == dom.ElementNode {
			out = append(out, c)
		}
	}
	return out
}

func contains(ns []*dom.Node, n *dom.Node) bool {
	for _, m := range ns {
		if m == n {
			return true
		}
	}
	return false
}

// tokens splits an attribute on ASCII whitespace, dropping empties and
// duplicates (first occurrence kept), as itemtype/itemprop/itemref require.
func tokens(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range strings.Fields(s) {
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
