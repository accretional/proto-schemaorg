package jsonld

// html.go — extract JSON-LD from a full HTML page. Parsing goes through
// accretional/proto-html (gluon-based HTML grammar → DOM); this package never
// touches raw HTML bytes. We locate every <script type="application/ld+json">
// block in the DOM and feed its body to Extract.

import (
	"strings"

	"github.com/accretional/proto-html/dom"
	"github.com/accretional/proto-html/htmlparse"
)

// ExtractHTML parses an HTML document via proto-html and extracts the schema.org
// items from every embedded JSON-LD script block. Malformed blocks are skipped.
func ExtractHTML(htmlSrc []byte) ([]Item, error) {
	doc, err := htmlparse.ParseDOM(string(htmlSrc))
	if err != nil {
		return nil, err
	}
	var items []Item
	for _, s := range doc.FindAll(isLDJSON) {
		body := s.Text()
		if strings.TrimSpace(body) == "" {
			continue
		}
		got, err := Extract([]byte(body))
		if err != nil {
			continue // best-effort over real pages
		}
		items = append(items, got...)
	}
	return items, nil
}

func isLDJSON(n *dom.Node) bool {
	if n.Type != dom.ElementNode || n.Data != "script" {
		return false
	}
	v, _ := n.AttrVal("type")
	return strings.EqualFold(strings.TrimSpace(v), "application/ld+json")
}
