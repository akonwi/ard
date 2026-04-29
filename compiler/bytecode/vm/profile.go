package vm

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	profileEnvVar    = "ARD_VM_PROFILE"
	profileTopEnvVar = "ARD_VM_PROFILE_TOP"
)

func ProfilingEnabled() bool {
	raw, ok := os.LookupEnv(profileEnvVar)
	if !ok {
		return false
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw != "" && raw != "0" && raw != "false" && raw != "off"
}

func profileTopN() int {
	raw := strings.TrimSpace(os.Getenv(profileTopEnvVar))
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	return n
}

type bindingProfile struct {
	binding  string
	calls    int
	argcSum  int
	maxArity int
	total    time.Duration
}

type executionProfile struct {
	started time.Time

	mu sync.Mutex

	directCalls      int
	moduleCalls      int
	closureCalls     int
	closureArgSum    int
	closureMaxArity  int
	closureCreations int
	captureSlots     int
	maxCaptures      int
	externCalls      int
	externArgSum     int
	externMaxArity   int
	externTotal      time.Duration
	externByBinding  map[string]*bindingProfile
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

func (p *executionProfile) RecordDirectCall() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.directCalls++
	p.mu.Unlock()
}

func (p *executionProfile) RecordModuleCall() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.moduleCalls++
	p.mu.Unlock()
}

func (p *executionProfile) RecordClosureCreation(captures int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closureCreations++
	p.captureSlots += captures
	if captures > p.maxCaptures {
		p.maxCaptures = captures
	}
}

func (p *executionProfile) RecordClosureCall(argc int) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closureCalls++
	p.closureArgSum += argc
	if argc > p.closureMaxArity {
		p.closureMaxArity = argc
	}
}

func (p *executionProfile) RecordExternCall(binding string, argc int, dur time.Duration) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	p.externCalls++
	p.externArgSum += argc
	p.externTotal += dur
	if argc > p.externMaxArity {
		p.externMaxArity = argc
	}

	stat := p.externByBinding[binding]
	if stat == nil {
		stat = &bindingProfile{binding: binding}
		p.externByBinding[binding] = stat
	}
	stat.calls++
	stat.argcSum += argc
	stat.total += dur
	if argc > stat.maxArity {
		stat.maxArity = argc
	}
}

func formatProfileDuration(d time.Duration) string {
	if d >= time.Microsecond {
		return d.Round(time.Microsecond).String()
	}
	return d.String()
}

func (p *executionProfile) Report() string {
	if p == nil {
		return ""
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	var out strings.Builder
	wall := time.Since(p.started)
	fmt.Fprintf(&out, "[ard vm profile]\n")
	fmt.Fprintf(&out, "wall=%s\n", wall.Round(time.Microsecond))
	fmt.Fprintf(&out, "calls direct=%d module=%d closure=%d extern=%d\n", p.directCalls, p.moduleCalls, p.closureCalls, p.externCalls)
	if p.closureCalls > 0 || p.closureCreations > 0 {
		avgClosureArity := 0.0
		if p.closureCalls > 0 {
			avgClosureArity = float64(p.closureArgSum) / float64(p.closureCalls)
		}
		avgCaptures := 0.0
		if p.closureCreations > 0 {
			avgCaptures = float64(p.captureSlots) / float64(p.closureCreations)
		}
		fmt.Fprintf(&out, "closures created=%d avg_captures=%.2f max_captures=%d avg_call_arity=%.2f max_call_arity=%d\n", p.closureCreations, avgCaptures, p.maxCaptures, avgClosureArity, p.closureMaxArity)
	}
	if p.externCalls > 0 {
		avgExternArity := float64(p.externArgSum) / float64(p.externCalls)
		avgExternTime := p.externTotal / time.Duration(p.externCalls)
		fmt.Fprintf(&out, "extern total=%s avg=%s avg_arity=%.2f max_arity=%d distinct_bindings=%d\n", p.externTotal.Round(time.Microsecond), formatProfileDuration(avgExternTime), avgExternArity, p.externMaxArity, len(p.externByBinding))

		bindings := make([]*bindingProfile, 0, len(p.externByBinding))
		for _, stat := range p.externByBinding {
			bindings = append(bindings, stat)
		}
		sort.Slice(bindings, func(i, j int) bool {
			if bindings[i].total == bindings[j].total {
				if bindings[i].calls == bindings[j].calls {
					return bindings[i].binding < bindings[j].binding
				}
				return bindings[i].calls > bindings[j].calls
			}
			return bindings[i].total > bindings[j].total
		})

		limit := profileTopN()
		if limit > len(bindings) {
			limit = len(bindings)
		}
		fmt.Fprintf(&out, "top extern bindings (by total time):\n")
		for i := 0; i < limit; i++ {
			stat := bindings[i]
			avg := stat.total / time.Duration(stat.calls)
			avgArity := float64(stat.argcSum) / float64(stat.calls)
			fmt.Fprintf(&out, "  %2d. %s calls=%d total=%s avg=%s avg_arity=%.2f max_arity=%d\n", i+1, stat.binding, stat.calls, stat.total.Round(time.Microsecond), formatProfileDuration(avg), avgArity, stat.maxArity)
		}
	}
	return strings.TrimRight(out.String(), "\n")
}
