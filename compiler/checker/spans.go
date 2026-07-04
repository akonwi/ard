package checker

import (
	"sort"
	"strings"

	"github.com/akonwi/ard/parse"
)

// SpanIndex is a position-indexed table of resolved source spans, recorded
// during Check when CheckOptions.RecordSpans is set. It is the semantic
// source of truth for tooling (LSP hover, definition, references, rename).
// See docs/adrs/0043-rebuild-lsp-on-snapshot-analysis.md.
type SpanIndex struct {
	records []SpanRecord
	seen    map[spanDedupKey]bool
	sorted  bool
}

// spanDedupKey identifies a record by position and checked node so the same
// resolution recorded through both checkExpr and checkExprAs (or through
// re-checks) collapses to one entry. Reference counts (ByKey) depend on this.
type spanDedupKey struct {
	loc  parse.Location
	node Expression
	key  any
}

// SpanRecord ties a source span to its checked node and an optional identity
// key that groups definition and use sites of the same entity.
type SpanRecord struct {
	Loc parse.Location
	// Source is the parse-tree node the span came from.
	Source parse.Expression
	// Node is the checked expression, when the span is an expression. It may
	// be nil for pure definition records (e.g. a variable binding).
	Node Expression
	// Key groups records referring to the same entity. Locals use the
	// declaring *Symbol pointer; nominal entities use stable string keys such
	// as "fn:<module>:<name>". Nil when the record has no identity.
	Key any
	// IsDef marks the record as the entity's definition site.
	IsDef bool
	// Target names an entity defined in another module, for cross-file
	// navigation. Nil for local references.
	Target *SpanTarget
}

// TargetKind classifies a cross-module reference target.
type TargetKind uint8

const (
	TargetFunction TargetKind = iota
	TargetType
	TargetField
	TargetMethod
)

// SpanTarget names an entity defined in another Ard module.
type SpanTarget struct {
	Kind TargetKind
	// Module is the target module's canonical path.
	Module string
	// File is the target module's source file path when known at check time.
	// Preferred over re-resolving Module in tooling.
	File string
	// Symbol is the entity's name in the target module.
	Symbol string
	// Owner is the owning type's name for field and method targets.
	Owner string
}

// memberTarget builds a field/method target from a subject type when the
// subject is a struct (or mutable reference to one).
func memberTarget(kind TargetKind, subjType Type, member string) *SpanTarget {
	if ref, ok := subjType.(*MutableRef); ok {
		subjType = ref.Of()
	}
	switch owner := subjType.(type) {
	case *StructDef:
		return &SpanTarget{Kind: kind, Module: owner.ModulePath, Symbol: member, Owner: owner.Name}
	case *Enum:
		return &SpanTarget{Kind: kind, Module: owner.ModulePath, Symbol: member, Owner: owner.Name}
	case *Trait:
		return &SpanTarget{Kind: kind, Module: owner.ModulePath, Symbol: member, Owner: owner.Name}
	}
	return nil
}

// recordMember records a struct/enum member access for navigation. Member
// accesses on function-typed properties are method values and recorded as
// method targets.
func (c *Checker) recordMember(loc parse.Location, kind TargetKind, subjType Type, member string, node Expression) {
	if c.spans == nil {
		return
	}
	if kind == TargetField && node != nil {
		if _, isFn := node.Type().(*FunctionDef); isFn {
			kind = TargetMethod
		}
	}
	target := memberTarget(kind, subjType, member)
	if target == nil {
		return
	}
	target.File = c.moduleFiles[target.Module]
	c.spans.add(SpanRecord{
		Loc:    loc,
		Node:   node,
		Target: target,
	})
}

func (i *SpanIndex) add(rec SpanRecord) {
	if !locValid(rec.Loc) {
		return
	}
	dedup := spanDedupKey{loc: rec.Loc, node: rec.Node, key: rec.Key}
	if i.seen == nil {
		i.seen = map[spanDedupKey]bool{}
	}
	if i.seen[dedup] {
		return
	}
	i.seen[dedup] = true
	i.records = append(i.records, rec)
	i.sorted = false
}

func locValid(loc parse.Location) bool {
	return !(loc.Start.Row == 0 && loc.Start.Col == 0)
}

func (i *SpanIndex) ensureSorted() {
	if i.sorted {
		return
	}
	sort.SliceStable(i.records, func(a, b int) bool {
		ra, rb := i.records[a], i.records[b]
		if ra.Loc.Start.Row != rb.Loc.Start.Row {
			return ra.Loc.Start.Row < rb.Loc.Start.Row
		}
		return ra.Loc.Start.Col < rb.Loc.Start.Col
	})
	i.sorted = true
}

// At returns all records whose span contains the point, innermost first.
func (i *SpanIndex) At(p parse.Point) []SpanRecord {
	if i == nil {
		return nil
	}
	i.ensureSorted()
	var out []SpanRecord
	for _, rec := range i.records {
		if spanContains(rec.Loc, p) {
			out = append(out, rec)
		}
	}
	// innermost = smallest span first
	sort.SliceStable(out, func(a, b int) bool {
		return spanSize(out[a].Loc) < spanSize(out[b].Loc)
	})
	return out
}

// ByKey returns all records sharing the identity key, in source order.
func (i *SpanIndex) ByKey(key any) []SpanRecord {
	if i == nil || key == nil {
		return nil
	}
	i.ensureSorted()
	var out []SpanRecord
	for _, rec := range i.records {
		if rec.Key == key {
			out = append(out, rec)
		}
	}
	return out
}

// Def returns the definition record for the identity key, if recorded.
func (i *SpanIndex) Def(key any) (SpanRecord, bool) {
	for _, rec := range i.ByKey(key) {
		if rec.IsDef {
			return rec, true
		}
	}
	return SpanRecord{}, false
}

// Len reports the number of recorded spans.
func (i *SpanIndex) Len() int {
	if i == nil {
		return 0
	}
	return len(i.records)
}

// Records returns a copy of the records in source order.
func (i *SpanIndex) Records() []SpanRecord {
	if i == nil {
		return nil
	}
	i.ensureSorted()
	out := make([]SpanRecord, len(i.records))
	copy(out, i.records)
	return out
}

func spanContains(loc parse.Location, p parse.Point) bool {
	if !locValid(loc) {
		return false
	}
	if p.Row < loc.Start.Row || (p.Row == loc.Start.Row && p.Col < loc.Start.Col) {
		return false
	}
	if loc.End.Row <= 0 {
		// An unset end would otherwise match every later point; treat the
		// span as covering only its start row.
		return p.Row == loc.Start.Row
	}
	if p.Row > loc.End.Row || (p.Row == loc.End.Row && p.Col > loc.End.Col) {
		return false
	}
	return true
}

func spanSize(loc parse.Location) int {
	rows := loc.End.Row - loc.Start.Row
	if rows == 0 {
		return loc.End.Col - loc.Start.Col
	}
	return rows * 10000
}

// --- checker recording hooks ---

// recordExprSpan records a checked expression's span.
func (c *Checker) recordExprSpan(source parse.Expression, node Expression) {
	if c.spans == nil || source == nil || node == nil {
		return
	}
	rec := SpanRecord{
		Loc:    source.GetLocation(),
		Source: source,
		Node:   node,
		Key:    c.spanKeyFor(node),
	}
	// Derive member targets centrally so method calls navigate to their
	// definitions. The precise method-name sub-span comes from the parse node
	// when available.
	if _, _, ok := BuiltinMethodInfo(node); ok {
		// Builtin method: narrow to the method-name sub-span for precise
		// hover/definition anchoring.
		if parsed, ok := source.(*parse.InstanceMethod); ok {
			rec.Loc = parsed.Method.GetLocation()
		}
	}
	if im, ok := node.(*InstanceMethod); ok && im.Method != nil {
		var subjType Type
		if im.StructType != nil {
			subjType = im.StructType
		} else if im.EnumType != nil {
			subjType = im.EnumType
		} else if im.TraitType != nil {
			subjType = im.TraitType
		}
		if subjType != nil {
			if target := memberTarget(TargetMethod, subjType, im.Method.Name); target != nil {
				target.File = c.moduleFiles[target.Module]
				rec.Target = target
				if parsed, ok := source.(*parse.InstanceMethod); ok {
					rec.Loc = parsed.Method.GetLocation()
				}
			}
		}
	}
	c.spans.add(rec)
}

// recordBinding records a local symbol's definition site.
func (c *Checker) recordBinding(loc parse.Location, sym *Symbol) {
	if c.spans == nil || sym == nil {
		return
	}
	c.spans.add(SpanRecord{
		Loc:   loc,
		Key:   sym,
		IsDef: true,
	})
}

// recordSymbolUse records an identifier use resolved to a scope symbol.
func (c *Checker) recordSymbolUse(source parse.Expression, sym *Symbol, node Expression) {
	if c.spans == nil || sym == nil {
		return
	}
	c.spans.add(SpanRecord{
		Loc:    source.GetLocation(),
		Source: source,
		Node:   node,
		Key:    sym,
	})
}

// recordTarget records a reference to an entity defined in another module.
func (c *Checker) recordTarget(source parse.Expression, node Expression, target SpanTarget) {
	if c.spans == nil || source == nil {
		return
	}
	if target.File == "" {
		target.File = c.moduleFiles[target.Module]
	}
	c.spans.add(SpanRecord{
		Loc:    source.GetLocation(),
		Source: source,
		Node:   node,
		Target: &target,
	})
}

// recordTypeRef records a reference to a locally defined nominal type.
func (c *Checker) recordTypeRef(loc parse.Location, name string) {
	if c.spans == nil {
		return
	}
	c.spans.add(SpanRecord{
		Loc: loc,
		Key: TypeKey(c.typeOwnerPath(), name),
	})
}

// recordDef records a nominal entity's definition site under a string key.
func (c *Checker) recordDef(loc parse.Location, key string) {
	if c.spans == nil {
		return
	}
	c.spans.add(SpanRecord{
		Loc:   loc,
		Key:   key,
		IsDef: true,
	})
}

// spanKeyFor derives a stable identity key from a checked node, when one
// exists. Locals are keyed elsewhere (by *Symbol); this covers nominal
// references like function calls.
func (c *Checker) spanKeyFor(node Expression) any {
	switch n := node.(type) {
	case *FunctionCall:
		// Only key module-local, non-namespaced calls. Namespaced calls
		// (mod::fn, Type::fn, Go packages) would mis-key under the current
		// module path; cross-module identity is a later slice.
		if n.fn != nil && n.fn.Name != "" && n.fn.Receiver == "" &&
			!strings.Contains(n.Name, "::") {
			return FunctionKey(c.typeOwnerPath(), n.fn.Name)
		}
	}
	return nil
}

// isNominalType reports whether a checker type is a nominal declaration that
// TypeKey identity applies to.
func isNominalType(t Type) bool {
	switch t.(type) {
	case *StructDef, *Enum, *Trait, *Union:
		return true
	}
	return false
}

// FunctionKey builds the identity key for a module-level function.
func FunctionKey(modulePath, name string) string {
	return "fn:" + modulePath + ":" + name
}

// TypeKey builds the identity key for a nominal type.
func TypeKey(modulePath, name string) string {
	return "type:" + modulePath + ":" + name
}

// Spans returns the recorded span index. Nil-safe: returns an empty index
// when recording was not enabled.
func (c *Checker) Spans() *SpanIndex {
	if c.spans == nil {
		return &SpanIndex{}
	}
	return c.spans
}

// spansMark returns the current record count so speculative checking can roll
// back records it produced. Zero-cost when recording is disabled.
func (c *Checker) spansMark() int {
	if c.spans == nil {
		return 0
	}
	return len(c.spans.records)
}

// spansTruncate discards records added after mark. Used together with
// diagnostic rollback in speculative checking paths.
func (c *Checker) spansTruncate(mark int) {
	if c.spans == nil {
		return
	}
	if mark < len(c.spans.records) {
		for _, rec := range c.spans.records[mark:] {
			delete(c.spans.seen, spanDedupKey{loc: rec.Loc, node: rec.Node, key: rec.Key})
		}
		c.spans.records = c.spans.records[:mark]
	}
}
