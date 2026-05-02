package vm_next

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/akonwi/ard/air"
)

const (
	profileEnvVar       = "ARD_VM_NEXT_PROFILE"
	profileCompatEnvVar = "ARD_VM_PROFILE"
	profileTopEnvVar    = "ARD_VM_NEXT_PROFILE_TOP"
	profileCompatTopEnv = "ARD_VM_PROFILE_TOP"
)

func ProfilingEnabled() bool {
	return truthyEnv(profileEnvVar) || truthyEnv(profileCompatEnvVar)
}

func truthyEnv(name string) bool {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return false
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw != "" && raw != "0" && raw != "false" && raw != "off"
}

func profileTopN() int {
	raw := strings.TrimSpace(os.Getenv(profileTopEnvVar))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv(profileCompatTopEnv))
	}
	if raw == "" {
		return 12
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 12
	}
	return n
}

type bindingProfile struct {
	binding    string
	calls      int
	argcSum    int
	maxArity   int
	convertIn  time.Duration
	host       time.Duration
	convertOut time.Duration
}

type executionProfile struct {
	started time.Time

	directCalls      atomic.Uint64
	directArgSum     atomic.Uint64
	closureCalls     atomic.Uint64
	closureArgSum    atomic.Uint64
	closureCreations atomic.Uint64
	captureSlots     atomic.Uint64
	traitCalls       atomic.Uint64
	fiberSpawns      atomic.Uint64
	fiberWaits       atomic.Uint64
	fiberWaitNS      atomic.Int64
	frames           atomic.Uint64
	maxLocals        atomic.Uint64

	stmtCounts [256]atomic.Uint64
	exprCounts [256]atomic.Uint64

	externCalls    atomic.Uint64
	externArgSum   atomic.Uint64
	externMaxArity atomic.Uint64
	externInNS     atomic.Int64
	externHostNS   atomic.Int64
	externOutNS    atomic.Int64

	mu              sync.Mutex
	externByBinding map[string]*bindingProfile
}

func newExecutionProfile() *executionProfile {
	if !ProfilingEnabled() {
		return nil
	}
	return &executionProfile{
		started:         time.Now(),
		externByBinding: make(map[string]*bindingProfile),
	}
}

func (p *executionProfile) RecordDirectCall(argc int, locals int) {
	if p == nil {
		return
	}
	p.directCalls.Add(1)
	p.directArgSum.Add(uint64(argc))
	p.recordFrame(locals)
}

func (p *executionProfile) RecordClosureCall(argc int, locals int) {
	if p == nil {
		return
	}
	p.closureCalls.Add(1)
	p.closureArgSum.Add(uint64(argc))
	p.recordFrame(locals)
}

func (p *executionProfile) RecordClosureCreation(captures int) {
	if p == nil {
		return
	}
	p.closureCreations.Add(1)
	p.captureSlots.Add(uint64(captures))
}

func (p *executionProfile) RecordTraitCall() {
	if p == nil {
		return
	}
	p.traitCalls.Add(1)
}

func (p *executionProfile) RecordFiberSpawn() {
	if p == nil {
		return
	}
	p.fiberSpawns.Add(1)
}

func (p *executionProfile) RecordFiberWait(dur time.Duration) {
	if p == nil {
		return
	}
	p.fiberWaits.Add(1)
	p.fiberWaitNS.Add(dur.Nanoseconds())
}

func (p *executionProfile) RecordStmt(kind air.StmtKind) {
	if p == nil {
		return
	}
	p.stmtCounts[uint8(kind)].Add(1)
}

func (p *executionProfile) RecordExpr(kind air.ExprKind) {
	if p == nil {
		return
	}
	p.exprCounts[uint8(kind)].Add(1)
}

func (p *executionProfile) RecordExternCall(binding string, argc int, convertIn, host, convertOut time.Duration) {
	if p == nil {
		return
	}
	p.externCalls.Add(1)
	p.externArgSum.Add(uint64(argc))
	for {
		current := p.externMaxArity.Load()
		if uint64(argc) <= current || p.externMaxArity.CompareAndSwap(current, uint64(argc)) {
			break
		}
	}
	p.externInNS.Add(convertIn.Nanoseconds())
	p.externHostNS.Add(host.Nanoseconds())
	p.externOutNS.Add(convertOut.Nanoseconds())

	p.mu.Lock()
	defer p.mu.Unlock()
	stat := p.externByBinding[binding]
	if stat == nil {
		stat = &bindingProfile{binding: binding}
		p.externByBinding[binding] = stat
	}
	stat.calls++
	stat.argcSum += argc
	if argc > stat.maxArity {
		stat.maxArity = argc
	}
	stat.convertIn += convertIn
	stat.host += host
	stat.convertOut += convertOut
}

func (p *executionProfile) recordFrame(locals int) {
	p.frames.Add(1)
	for {
		current := p.maxLocals.Load()
		if uint64(locals) <= current || p.maxLocals.CompareAndSwap(current, uint64(locals)) {
			return
		}
	}
}

func (p *executionProfile) Report() string {
	if p == nil {
		return ""
	}

	wall := time.Since(p.started)
	directCalls := p.directCalls.Load()
	closureCalls := p.closureCalls.Load()
	closureCreations := p.closureCreations.Load()
	externCalls := p.externCalls.Load()
	fiberWaits := p.fiberWaits.Load()

	var out strings.Builder
	fmt.Fprintf(&out, "[ard vm_next profile]\n")
	fmt.Fprintf(&out, "wall=%s\n", wall.Round(time.Microsecond))
	fmt.Fprintf(&out, "calls direct=%d closure=%d trait=%d extern=%d\n", directCalls, closureCalls, p.traitCalls.Load(), externCalls)
	fmt.Fprintf(&out, "frames=%d max_locals=%d\n", p.frames.Load(), p.maxLocals.Load())

	if closureCalls > 0 || closureCreations > 0 {
		avgClosureArity := avgUint64(p.closureArgSum.Load(), closureCalls)
		avgCaptures := avgUint64(p.captureSlots.Load(), closureCreations)
		fmt.Fprintf(&out, "closures created=%d avg_captures=%.2f avg_call_arity=%.2f\n", closureCreations, avgCaptures, avgClosureArity)
	}
	if p.fiberSpawns.Load() > 0 || fiberWaits > 0 {
		fmt.Fprintf(&out, "fibers spawned=%d waits=%d wait_total=%s\n", p.fiberSpawns.Load(), fiberWaits, time.Duration(p.fiberWaitNS.Load()).Round(time.Microsecond))
	}

	p.writeTopStmtCounts(&out)
	p.writeTopExprCounts(&out)
	p.writeExternReport(&out, externCalls)
	return strings.TrimRight(out.String(), "\n")
}

func (p *executionProfile) writeExternReport(out *strings.Builder, externCalls uint64) {
	if externCalls == 0 {
		return
	}
	convertIn := time.Duration(p.externInNS.Load())
	host := time.Duration(p.externHostNS.Load())
	convertOut := time.Duration(p.externOutNS.Load())
	total := convertIn + host + convertOut
	avgExternArity := avgUint64(p.externArgSum.Load(), externCalls)
	fmt.Fprintf(out, "extern total=%s convert_in=%s host=%s convert_out=%s avg_arity=%.2f max_arity=%d distinct_bindings=%d\n",
		total.Round(time.Microsecond), convertIn.Round(time.Microsecond), host.Round(time.Microsecond), convertOut.Round(time.Microsecond), avgExternArity, p.externMaxArity.Load(), len(p.externByBinding))

	p.mu.Lock()
	bindings := make([]*bindingProfile, 0, len(p.externByBinding))
	for _, stat := range p.externByBinding {
		copy := *stat
		bindings = append(bindings, &copy)
	}
	p.mu.Unlock()

	sort.Slice(bindings, func(i, j int) bool {
		left := bindings[i].convertIn + bindings[i].host + bindings[i].convertOut
		right := bindings[j].convertIn + bindings[j].host + bindings[j].convertOut
		if left == right {
			if bindings[i].calls == bindings[j].calls {
				return bindings[i].binding < bindings[j].binding
			}
			return bindings[i].calls > bindings[j].calls
		}
		return left > right
	})

	limit := profileTopN()
	if limit > len(bindings) {
		limit = len(bindings)
	}
	fmt.Fprintf(out, "top extern bindings (by boundary+host time):\n")
	for i := 0; i < limit; i++ {
		stat := bindings[i]
		total := stat.convertIn + stat.host + stat.convertOut
		avg := total / time.Duration(stat.calls)
		avgArity := float64(stat.argcSum) / float64(stat.calls)
		fmt.Fprintf(out, "  %2d. %s calls=%d total=%s avg=%s in=%s host=%s out=%s avg_arity=%.2f max_arity=%d\n",
			i+1, stat.binding, stat.calls, total.Round(time.Microsecond), formatProfileDuration(avg), stat.convertIn.Round(time.Microsecond), stat.host.Round(time.Microsecond), stat.convertOut.Round(time.Microsecond), avgArity, stat.maxArity)
	}
}

func (p *executionProfile) writeTopStmtCounts(out *strings.Builder) {
	counts := make([]kindCount, 0)
	for i := range p.stmtCounts {
		count := p.stmtCounts[i].Load()
		if count > 0 {
			counts = append(counts, kindCount{name: stmtKindName(air.StmtKind(i)), count: count})
		}
	}
	writeTopCounts(out, "top statements", counts)
}

func (p *executionProfile) writeTopExprCounts(out *strings.Builder) {
	counts := make([]kindCount, 0)
	for i := range p.exprCounts {
		count := p.exprCounts[i].Load()
		if count > 0 {
			counts = append(counts, kindCount{name: exprKindName(air.ExprKind(i)), count: count})
		}
	}
	writeTopCounts(out, "top expressions", counts)
}

type kindCount struct {
	name  string
	count uint64
}

func writeTopCounts(out *strings.Builder, title string, counts []kindCount) {
	if len(counts) == 0 {
		return
	}
	sort.Slice(counts, func(i, j int) bool {
		if counts[i].count == counts[j].count {
			return counts[i].name < counts[j].name
		}
		return counts[i].count > counts[j].count
	})
	limit := profileTopN()
	if limit > len(counts) {
		limit = len(counts)
	}
	fmt.Fprintf(out, "%s:\n", title)
	for i := 0; i < limit; i++ {
		fmt.Fprintf(out, "  %2d. %s count=%d\n", i+1, counts[i].name, counts[i].count)
	}
}

func avgUint64(total uint64, count uint64) float64 {
	if count == 0 {
		return 0
	}
	return float64(total) / float64(count)
}

func formatProfileDuration(d time.Duration) string {
	if d >= time.Microsecond {
		return d.Round(time.Microsecond).String()
	}
	return d.String()
}

func stmtKindName(kind air.StmtKind) string {
	switch kind {
	case air.StmtLet:
		return "let"
	case air.StmtAssign:
		return "assign"
	case air.StmtSetField:
		return "set_field"
	case air.StmtExpr:
		return "expr"
	case air.StmtWhile:
		return "while"
	case air.StmtBreak:
		return "break"
	default:
		return fmt.Sprintf("stmt_%d", kind)
	}
}

var exprKindNames = []string{
	"const_void", "const_int", "const_float", "const_bool", "const_str", "panic", "load_local", "call", "call_extern", "make_closure", "call_closure", "spawn_fiber", "fiber_get", "fiber_join", "union_wrap", "match_union", "trait_upcast", "call_trait", "copy", "make_list", "list_at", "list_prepend", "list_push", "list_set", "list_size", "list_sort", "list_swap", "make_map", "map_keys", "map_size", "map_get", "map_set", "map_drop", "map_has", "map_key_at", "map_value_at", "make_struct", "get_field", "int_add", "int_sub", "int_mul", "int_div", "int_mod", "float_add", "float_sub", "float_mul", "float_div", "str_concat", "to_str", "str_at", "str_size", "str_is_empty", "str_contains", "str_replace", "str_replace_all", "str_split", "str_starts_with", "to_dynamic", "str_trim", "eq", "not_eq", "lt", "lte", "gt", "gte", "and", "or", "not", "neg", "block", "if", "make_result_ok", "make_result_err", "enum_variant", "match_enum", "match_int", "make_maybe_some", "make_maybe_none", "match_maybe", "maybe_expect", "maybe_is_none", "maybe_is_some", "maybe_or", "maybe_map", "maybe_and_then", "match_result", "result_expect", "result_or", "result_is_ok", "result_is_err", "result_map", "result_map_err", "result_and_then", "try_result", "try_maybe",
}

func exprKindName(kind air.ExprKind) string {
	index := int(kind)
	if index >= 0 && index < len(exprKindNames) {
		return exprKindNames[index]
	}
	return fmt.Sprintf("expr_%d", kind)
}
