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
	sorted  bool
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
}

func (i *SpanIndex) add(rec SpanRecord) {
	if !locValid(rec.Loc) {
		return
	}
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
	c.spans.add(SpanRecord{
		Loc:    source.GetLocation(),
		Source: source,
		Node:   node,
		Key:    c.spanKeyFor(node),
	})
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
		c.spans.records = c.spans.records[:mark]
	}
}
