// Command livecheck is a non-gating reporting harness over the real fetched
// pages in testing/testdata/live/*.html. For each page it runs both schema.org
// extractors — service/microdata.Extract and service/jsonld.ExtractHTML — and
// prints a per-page table: whether proto-html's ParseDOM accepted the document
// (OK/FAIL + the first line of the parse error), how many microdata items and
// JSON-LD items were extracted, and the most common schema.org types found.
//
// Real-world HTML is messy, so this is intentionally NOT a test: it never fails
// the build. It is a diagnostic lens on how the extractors fare against the live
// corpus. Run it with:
//
//	go run ./testing/livecheck
//
// It resolves the corpus relative to this source file, so the working directory
// does not matter.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/accretional/proto-html/htmlparse"

	schemaorg "github.com/accretional/proto-schemaorg/proto"
	"github.com/accretional/proto-schemaorg/service/jsonld"
	"github.com/accretional/proto-schemaorg/service/microdata"
)

// meta mirrors the sibling NNN-slug.html.meta.json provenance files; only the
// URL is used here (as the microdata base for URL-valued properties).
type meta struct {
	URL string `json:"url"`
}

// row is one page's diagnostic result.
type row struct {
	name      string
	parseOK   bool
	parseErr  string // first line of the parse error, when parseOK is false
	mdItems   int
	mdErr     string
	jsonldN   int
	jsonldErr string
	types     []string // top schema.org type local-names across both extractors
}

func main() {
	dir := liveDir()
	entries, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "glob:", err)
		os.Exit(1)
	}
	sort.Strings(entries)
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "no *.html under %s\n", dir)
		os.Exit(1)
	}

	var rows []row
	for _, path := range entries {
		rows = append(rows, check(path))
	}

	printTable(rows)
	printSummary(rows)
}

// liveDir locates testing/testdata/live relative to this source file, so the
// harness runs from any working directory.
func liveDir() string {
	_, self, _, _ := runtime.Caller(0)
	// self == .../testing/livecheck/main.go
	return filepath.Join(filepath.Dir(self), "..", "testdata", "live")
}

func check(path string) row {
	r := row{name: filepath.Base(path)}
	src, err := os.ReadFile(path)
	if err != nil {
		r.parseErr = "read: " + err.Error()
		return r
	}

	// Shared parse: both extractors go through htmlparse.ParseDOM, so this one
	// call decides whether either can produce anything at all.
	if _, perr := htmlparse.ParseDOM(string(src)); perr != nil {
		r.parseErr = firstLine(perr.Error())
	} else {
		r.parseOK = true
	}

	base := baseURL(path)

	// Microdata.
	typeCount := map[string]int{}
	if doc, err := microdata.Extract(src, base); err != nil {
		r.mdErr = firstLine(err.Error())
	} else {
		r.mdItems = countItems(doc.Items, typeCount)
	}

	// JSON-LD.
	if items, err := jsonld.ExtractHTML(src); err != nil {
		r.jsonldErr = firstLine(err.Error())
	} else {
		r.jsonldN = len(items)
		for _, it := range items {
			if it.Type != "" {
				typeCount[it.Type]++
			}
		}
	}

	r.types = topTypes(typeCount, 4)
	return r
}

// countItems tallies items (recursively counting nested items) and records the
// local-name of each item's first modeled-ish type.
func countItems(items []*schemaorg.Item, typeCount map[string]int) int {
	n := 0
	var walk func(*schemaorg.Item)
	walk = func(it *schemaorg.Item) {
		n++
		for _, t := range it.Types {
			typeCount[schemaorg.LocalTypeName(t)]++
		}
		for _, vs := range it.Props {
			for _, v := range vs {
				if v.Item != nil {
					walk(v.Item)
				}
			}
		}
	}
	for _, it := range items {
		walk(it)
	}
	return n
}

func topTypes(counts map[string]int, k int) []string {
	type kv struct {
		t string
		n int
	}
	var kvs []kv
	for t, n := range counts {
		kvs = append(kvs, kv{t, n})
	}
	sort.Slice(kvs, func(i, j int) bool {
		if kvs[i].n != kvs[j].n {
			return kvs[i].n > kvs[j].n
		}
		return kvs[i].t < kvs[j].t
	})
	var out []string
	for i := 0; i < len(kvs) && i < k; i++ {
		out = append(out, fmt.Sprintf("%s(%d)", kvs[i].t, kvs[i].n))
	}
	return out
}

// baseURL returns the fetched URL from the sibling .meta.json, or "" (in which
// case a <base href> in the document, if any, is used by the extractor).
func baseURL(htmlPath string) string {
	b, err := os.ReadFile(htmlPath + ".meta.json")
	if err != nil {
		return ""
	}
	var m meta
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	return m.URL
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func printTable(rows []row) {
	fmt.Println("LIVE CORPUS EXTRACTION REPORT (non-gating)")
	fmt.Println(strings.Repeat("=", 100))
	fmt.Printf("%-34s  %-6s  %5s  %5s  %s\n", "PAGE", "PARSE", "MD", "JSONLD", "TOP TYPES")
	fmt.Println(strings.Repeat("-", 100))
	for _, r := range rows {
		status := "OK"
		if !r.parseOK {
			status = "FAIL"
		}
		fmt.Printf("%-34s  %-6s  %5d  %5d  %s\n",
			trunc(r.name, 34), status, r.mdItems, r.jsonldN, strings.Join(r.types, " "))
		if !r.parseOK && r.parseErr != "" {
			fmt.Printf("%-34s  └─ %s\n", "", r.parseErr)
		}
	}
	fmt.Println(strings.Repeat("=", 100))
}

func printSummary(rows []row) {
	okParse, mdAny, jsonldAny := 0, 0, 0
	for _, r := range rows {
		if r.parseOK {
			okParse++
		}
		if r.mdItems > 0 {
			mdAny++
		}
		if r.jsonldN > 0 {
			jsonldAny++
		}
	}
	n := len(rows)
	fmt.Printf("\nSUMMARY: %d pages | parse OK: %d/%d | pages w/ microdata: %d/%d | pages w/ JSON-LD: %d/%d\n",
		n, okParse, n, mdAny, n, jsonldAny, n)
	if okParse == 0 {
		fmt.Println("NOTE: every real page failed htmlparse.ParseDOM — both extractors are blocked at the parse stage.")
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
