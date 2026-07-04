// Command genast generates the schema.org protobuf type system directly from
// the vendored schema.org JSON-LD vocabulary by building a gluon AST in memory
// and lowering it with gluon's compiler — no EBNF text, no string-manipulation
// grammar pipeline.
//
// Pipeline:
//
//	schema/schemaorg-all-https.jsonld   (vendored vocabulary = source of truth)
//	  → parse the @graph into a type/property model
//	  → build a gluon *pb.ASTDescriptor  (Rule per type + per object-property)
//	  → compiler.Compile                 → *descriptorpb.FileDescriptorProto
//	  → descriptor.ToString / marshal
//
// Outputs:
//
//	proto/schemaorg.proto    bundled .proto (human-readable, committed)
//	proto/schemaorg.fdset    serialized FileDescriptorSet (committed)
//
// Model. One message per schema.org Type, named "Schema<Type>" (the Schema
// prefix keeps type messages out of the property/keyword namespace and away
// from digit-leading names like 3DModel). Each message carries an "id" field
// (the item's @id / itemid) plus one field per applicable property — the
// property's own domain plus every ancestor's (flattened inheritance), exactly
// the properties microdata/JSON-LD permit on an item of that type. A property
// whose value range is only DataTypes (Text/Number/Date/…) lowers to a repeated
// string field; a property that can hold a nested item gets a wrapper message
// (named after the property) holding a oneof over its class ranges (+ a text
// arm when the range also allows a DataType). Every property is repeated:
// schema.org lets a property occur multiple times.
//
// Because gluon derives BOTH a field's name and its message type from the
// referenced nonterminal's name (snakeCase / pascalCase of the same string),
// the AST references types by their exact rule name; casing is applied
// identically on both sides, so references always resolve.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/accretional/gluon/v2/compiler"
	pb "github.com/accretional/gluon/v2/pb"
	"github.com/accretional/merge/descriptor"
)

// defaultRoots is the curated core the closure starts from when -all is not
// given. The transitive closure over these types' object-property ranges pulls
// in everything reachable; anything past the (optional) cap collapses to the
// generic SchemaThing. Thing is always included as the fallback item type.
var defaultRoots = []string{
	"Thing", "Person", "Organization", "PostalAddress", "Place", "LocalBusiness",
	"Product", "Offer", "AggregateOffer", "AggregateRating", "Review", "Rating", "Brand",
	"Recipe", "NutritionInformation", "HowToStep", "Event", "Article", "NewsArticle",
	"BlogPosting", "WebPage", "WebSite", "BreadcrumbList", "ListItem", "ImageObject",
	"VideoObject", "Question", "Answer", "Book", "Movie", "CreativeWork",
}

func main() {
	jsonldPath := flag.String("jsonld", "schema/schemaorg-all-https.jsonld", "vendored schema.org JSON-LD vocabulary")
	protoOut := flag.String("proto", "proto/schemaorg.proto", "bundled .proto output")
	fdsetOut := flag.String("fdset", "proto/schemaorg.fdset", "FileDescriptorSet output")
	ebnfOut := flag.String("ebnf", "lang/schemaorg.ebnf", "human-readable EBNF view of the grammar (\"\" to skip)")
	pkg := flag.String("package", "schemaorg", "proto package name")
	goPkg := flag.String("go-package", "github.com/accretional/proto-schemaorg/proto/pb/schemaorg;schemaorgpb", "go_package option")
	rootsFlag := flag.String("roots", "", "comma-separated root types (default: curated core)")
	all := flag.Bool("all", false, "include every schema.org type (full vocabulary)")
	maxTypes := flag.Int("max", 0, "cap on included types (0 = unlimited); classes past the cap collapse to SchemaThing")
	flag.Parse()

	voc, err := loadVocab(*jsonldPath)
	if err != nil {
		log.Fatalf("load vocab: %v", err)
	}
	fmt.Printf("vocab: %d classes, %d properties, %d datatypes\n", len(voc.classes), len(voc.props), len(voc.datatypes))

	roots := defaultRoots
	if *rootsFlag != "" {
		roots = splitComma(*rootsFlag)
	}
	included := voc.closure(roots, *all, *maxTypes)
	fmt.Printf("included %d types (closure of %d roots%s)\n", len(included), len(roots), allNote(*all))

	fileNode, nMsgs := voc.buildAST(included)

	// Emit the grammar as a human-readable EBNF view. The gluon AST is the
	// canonical grammar; this is a generated projection of it (with the
	// subClassOf hierarchy annotated), so the vocabulary's structure — including
	// the property→range cross-entity links — is legible and composable.
	if *ebnfOut != "" {
		if err := os.MkdirAll(filepath.Dir(*ebnfOut), 0o755); err != nil {
			log.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(*ebnfOut, []byte(voc.emitEBNF(fileNode)), 0o644); err != nil {
			log.Fatalf("write %s: %v", *ebnfOut, err)
		}
		fmt.Printf("wrote %s\n", *ebnfOut)
	}

	fdp, err := compiler.Compile(&pb.ASTDescriptor{Language: *pkg, Root: fileNode}, compiler.Options{
		Package:   *pkg,
		GoPackage: *goPkg,
		FileName:  filepath.Base(*protoOut),
	})
	if err != nil {
		log.Fatalf("compiler.Compile: %v", err)
	}
	fmt.Printf("compiled %d messages (%d AST rules)\n", len(fdp.GetMessageType()), nMsgs)

	if err := os.MkdirAll(filepath.Dir(*protoOut), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}

	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fdp}}
	blob, err := proto.Marshal(set)
	if err != nil {
		log.Fatalf("marshal fdset: %v", err)
	}
	if err := os.WriteFile(*fdsetOut, blob, 0o644); err != nil {
		log.Fatalf("write %s: %v", *fdsetOut, err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", *fdsetOut, len(blob))

	src, err := descriptor.ToString(fdp)
	if err != nil {
		log.Fatalf("descriptor.ToString: %v", err)
	}
	if err := os.WriteFile(*protoOut, []byte(src), 0o644); err != nil {
		log.Fatalf("write %s: %v", *protoOut, err)
	}
	fmt.Printf("wrote %s\n", *protoOut)
}

// ── Vocabulary model ────────────────────────────────────────────────────────

type property struct {
	name    string
	domains []string // class names this property is declared on
	ranges  []string // value type names (classes and/or datatypes)
}

type vocab struct {
	classes    map[string]bool     // every rdfs:Class name
	datatypes  map[string]bool     // classes that are (or descend from) DataType
	subClassOf map[string][]string // name → direct superclasses
	props      map[string]property // property name → declaration
	// ancMemo memoizes the ancestor set (incl. self) per class.
	ancMemo map[string]map[string]bool
}

// loadVocab parses the schema.org JSON-LD @graph into the type/property model.
func loadVocab(path string) (*vocab, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Graph []map[string]any `json:"@graph"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}

	v := &vocab{
		classes:    map[string]bool{},
		datatypes:  map[string]bool{},
		subClassOf: map[string][]string{},
		props:      map[string]property{},
		ancMemo:    map[string]map[string]bool{},
	}
	dataTypeSeed := map[string]bool{"DataType": true}

	for _, node := range doc.Graph {
		id := local(str(node["@id"]))
		if id == "" {
			continue
		}
		types := idList(node["@type"]) // raw type tokens, e.g. rdfs:Class, rdf:Property, schema:DataType
		typeSet := map[string]bool{}
		for _, t := range types {
			typeSet[t] = true
		}

		// schema.org's -all dump references external vocabularies (bibo:, dcterms:,
		// …) whose local names carry a colon and are not valid proto identifiers.
		// Keep only schema.org-native terms (a bare identifier); references to the
		// skipped terms simply never resolve to a class and are dropped downstream.
		if !isIdent(id) {
			continue
		}
		switch {
		case typeSet["rdf:Property"]:
			v.props[id] = property{
				name:    id,
				domains: local_ids(node["schema:domainIncludes"]),
				ranges:  local_ids(node["schema:rangeIncludes"]),
			}
		case typeSet["rdfs:Class"]:
			v.classes[id] = true
			v.subClassOf[id] = local_ids(node["rdfs:subClassOf"])
			if typeSet["schema:DataType"] {
				dataTypeSeed[id] = true
			}
		}
	}

	// Datatypes = DataType and everything transitively subClassOf a datatype.
	for name := range dataTypeSeed {
		v.datatypes[name] = true
	}
	for changed := true; changed; {
		changed = false
		for name := range v.classes {
			if v.datatypes[name] {
				continue
			}
			for _, p := range v.subClassOf[name] {
				if v.datatypes[p] {
					v.datatypes[name] = true
					changed = true
					break
				}
			}
		}
	}
	return v, nil
}

// ancestors returns the class plus all its transitive superclasses.
func (v *vocab) ancestors(name string) map[string]bool {
	if m, ok := v.ancMemo[name]; ok {
		return m
	}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(n string) {
		if seen[n] {
			return
		}
		seen[n] = true
		for _, p := range v.subClassOf[n] {
			walk(p)
		}
	}
	walk(name)
	v.ancMemo[name] = seen
	return seen
}

// typeProps returns the sorted property names applicable to a type: every
// property whose domain includes the type or any of its ancestors.
func (v *vocab) typeProps(name string) []string {
	anc := v.ancestors(name)
	var out []string
	for pn, p := range v.props {
		for _, d := range p.domains {
			if anc[d] {
				out = append(out, pn)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// classRanges returns the non-datatype (nested-item) ranges of a property, and
// whether the property also permits a DataType (text) value.
func (v *vocab) classRanges(p property) (classes []string, hasText bool) {
	for _, r := range p.ranges {
		switch {
		case v.datatypes[r]:
			hasText = true
		case v.classes[r]:
			classes = append(classes, r)
		}
	}
	sort.Strings(classes)
	return classes, hasText
}

// closure computes the set of types to emit: the roots plus, transitively,
// every class reachable through an included type's object-property ranges.
// With all=true every class is included. A non-zero max caps the set (classes
// discovered past the cap are represented by SchemaThing at use sites).
func (v *vocab) closure(roots []string, all bool, max int) map[string]bool {
	included := map[string]bool{"Thing": true}
	if all {
		for c := range v.classes {
			if !v.datatypes[c] {
				included[c] = true
			}
		}
		return included
	}
	var queue []string
	push := func(c string) {
		if c == "" || v.datatypes[c] || !v.classes[c] || included[c] {
			return
		}
		if max > 0 && len(included) >= max {
			return
		}
		included[c] = true
		queue = append(queue, c)
	}
	for _, r := range roots {
		push(r)
	}
	for len(queue) > 0 {
		t := queue[0]
		queue = queue[1:]
		for _, pn := range v.typeProps(t) {
			cr, _ := v.classRanges(v.props[pn])
			for _, c := range cr {
				push(c)
			}
		}
	}
	return included
}

// ── AST construction ────────────────────────────────────────────────────────

func (v *vocab) buildAST(included map[string]bool) (root *pb.ASTNode, nRules int) {
	typeList := sortedKeys(included)

	var rules []*pb.ASTNode
	objProps := map[string]bool{} // object-valued properties actually referenced

	// Document root: repeated top-level item, a oneof over every included type.
	var topVariants []*pb.ASTNode
	for _, t := range typeList {
		topVariants = append(topVariants, nt("Schema"+t))
	}
	rules = append(rules, ruleN("SchemaOrgDocument", rep(nt("TopLevelItem"))))
	rules = append(rules, ruleN("TopLevelItem", alt(topVariants...)))

	// One message per included type.
	for _, t := range typeList {
		items := []*pb.ASTNode{scalar("id")} // the item's @id / itemid
		for _, pn := range v.typeProps(t) {
			p := v.props[pn]
			cr, _ := v.classRanges(p)
			if len(cr) == 0 {
				// datatype-only → repeated string field named after the property
				items = append(items, rep(scalar(pn)))
				continue
			}
			// object-valued → repeated message field; wrapper rule defined below
			objProps[pn] = true
			items = append(items, rep(nt(pn)))
		}
		rules = append(rules, ruleN("Schema"+t, seq(items...)))
	}

	// One wrapper message per referenced object property.
	for _, pn := range sortedKeys(objProps) {
		p := v.props[pn]
		cr, hasText := v.classRanges(p)
		var variants []*pb.ASTNode
		seen := map[string]bool{}
		for _, c := range cr {
			ref := "Schema" + c
			if !included[c] {
				ref = "SchemaThing" // collapse out-of-closure ranges to the generic item
			}
			if seen[ref] {
				continue
			}
			seen[ref] = true
			variants = append(variants, nt(ref))
		}
		if hasText {
			variants = append(variants, scalar("text"))
		}
		if len(variants) == 1 {
			rules = append(rules, ruleN(pn, variants[0]))
		} else {
			rules = append(rules, ruleN(pn, alt(variants...)))
		}
	}

	return &pb.ASTNode{Kind: compiler.KindFile, Children: rules}, len(rules)
}

// ── EBNF projection ─────────────────────────────────────────────────────────

// emitEBNF renders the grammar AST as human-readable EBNF. This is a generated
// *view* of the canonical grammar (the gluon AST); it is not re-parsed. Type
// productions carry a `(* subClassOf: … *)` annotation so the schema.org entity
// hierarchy — one of the cross-entity links a flat proto can't show — stays
// legible alongside the property→range references.
func (v *vocab) emitEBNF(root *pb.ASTNode) string {
	var b strings.Builder
	b.WriteString("(* schemaorg.ebnf — GENERATED by cmd/genast; do not edit.\n")
	b.WriteString("   A human-readable EBNF view of the schema.org vocabulary grammar.\n")
	b.WriteString("   Conventions: a production `Schema<Type>` is an item of that type; a\n")
	b.WriteString("   reference names another production (the property's range = an entity\n")
	b.WriteString("   link); `<name>` is a datatype (string) leaf; `{ x }` is a repetition\n")
	b.WriteString("   (a property may recur). The canonical grammar is the gluon AST built\n")
	b.WriteString("   in cmd/genast; this file is a projection of it. *)\n\n")
	for _, rule := range root.GetChildren() {
		name := rule.GetValue()
		if sub := v.subClassAnnot(name); sub != "" {
			fmt.Fprintf(&b, "(* %s *)\n", sub)
		}
		body := ""
		if kids := rule.GetChildren(); len(kids) == 1 {
			body = ebnfExpr(kids[0], 4)
		}
		fmt.Fprintf(&b, "%s = %s ;\n\n", name, body)
	}
	return b.String()
}

// subClassAnnot returns a "subClassOf: A, B" note for a Schema<Type> rule whose
// type has superclasses, else "".
func (v *vocab) subClassAnnot(rule string) string {
	if !strings.HasPrefix(rule, "Schema") {
		return ""
	}
	t := rule[len("Schema"):]
	if !v.classes[t] {
		return ""
	}
	sup := v.subClassOf[t]
	if len(sup) == 0 {
		return ""
	}
	return "subClassOf: " + strings.Join(sup, ", ")
}

// ebnfExpr renders one AST expression node. indent is the column for wrapped
// sequence/alternation items.
func ebnfExpr(n *pb.ASTNode, indent int) string {
	pad := "\n" + strings.Repeat(" ", indent)
	sep := func(op string, kids []*pb.ASTNode) string {
		parts := make([]string, len(kids))
		for i, k := range kids {
			parts[i] = ebnfExpr(k, indent+2)
		}
		if len(parts) <= 1 {
			return strings.Join(parts, "")
		}
		return pad + strings.Join(parts, pad+op+" ")
	}
	switch n.GetKind() {
	case compiler.KindNonterminal:
		return n.GetValue()
	case compiler.KindScalar:
		return "<" + n.GetValue() + ">"
	case compiler.KindRepetition:
		if kids := n.GetChildren(); len(kids) == 1 {
			return "{ " + ebnfExpr(kids[0], indent) + " }"
		}
		return "{ }"
	case compiler.KindSequence:
		if len(n.GetChildren()) == 0 {
			return "(* empty *)"
		}
		return sep(",", n.GetChildren())
	case compiler.KindAlternation:
		return sep("|", n.GetChildren())
	default:
		return ""
	}
}

// ── gluon AST node builders ─────────────────────────────────────────────────

func ruleN(name string, body *pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: compiler.KindRule, Value: name, Children: []*pb.ASTNode{body}}
}
func seq(items ...*pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: compiler.KindSequence, Children: items}
}
func alt(items ...*pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: compiler.KindAlternation, Children: items}
}
func rep(child *pb.ASTNode) *pb.ASTNode {
	return &pb.ASTNode{Kind: compiler.KindRepetition, Children: []*pb.ASTNode{child}}
}
func nt(name string) *pb.ASTNode  { return &pb.ASTNode{Kind: compiler.KindNonterminal, Value: name} }
func scalar(n string) *pb.ASTNode { return &pb.ASTNode{Kind: compiler.KindScalar, Value: n} }

// ── JSON-LD helpers ─────────────────────────────────────────────────────────

// local strips a schema.org identifier down to its local name: "schema:Person"
// → "Person", "https://schema.org/Person" → "Person". Non-schema prefixes
// (rdfs:, rdf:) are preserved so @type tokens stay comparable.
func local(id string) string {
	if id == "" {
		return ""
	}
	if strings.HasPrefix(id, "schema:") {
		return id[len("schema:"):]
	}
	if i := strings.LastIndex(id, "/"); i >= 0 && strings.Contains(id, "schema.org") {
		return id[i+1:]
	}
	return id
}

// isIdent reports whether name is a bare identifier: a letter followed by
// letters/digits/underscore. schema.org type and property local names satisfy
// this; external-vocab references (with a ':' prefix) do not.
func isIdent(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case (r >= '0' && r <= '9' || r == '_') && i > 0:
		default:
			return false
		}
	}
	return true
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// idList returns the raw string tokens from a JSON-LD value that is a string,
// an {"@id":…} object, or an array of either. Used for @type (kept raw).
func idList(v any) []string {
	var out []string
	switch t := v.(type) {
	case string:
		out = append(out, t)
	case map[string]any:
		if id, ok := t["@id"].(string); ok {
			out = append(out, id)
		}
	case []any:
		for _, e := range t {
			out = append(out, idList(e)...)
		}
	}
	return out
}

// local_ids is idList with each token reduced to its schema.org local name.
func local_ids(v any) []string {
	ids := idList(v)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, local(id))
	}
	return out
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func allNote(all bool) string {
	if all {
		return ", -all"
	}
	return ""
}
