// Command compilebench times the Ard-owned compile stages (frontend load,
// AIR lowering, Go source generation) for one entry point without invoking
// the Go toolchain. It exists for the autoresearch compile-time session and
// is not part of the ard CLI.
//
// Usage: go run ./internal/compilebench -input <main.ard> -reps 3 [-out <dir>]
//
// It prints `METRIC name=value` lines (milliseconds, median over reps) and a
// SHA-256 of the generated sources, with pointer-derived anonymous function
// names normalized, so a driver can verify output identity across runs.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"sort"
	"time"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/frontend"
	gotarget "github.com/akonwi/ard/go"
)

type stageTimes struct {
	load, lower, validate, generate time.Duration
}

func (s stageTimes) total() time.Duration { return s.load + s.lower + s.validate + s.generate }

func main() {
	input := flag.String("input", "main.ard", "entry point to compile")
	reps := flag.Int("reps", 3, "number of repetitions; medians are reported")
	outDir := flag.String("out", "", "optional directory to write the last run's generated sources")
	cpuProfile := flag.String("cpuprofile", "", "write a CPU profile to this file")
	memProfile := flag.String("memprofile", "", "write an allocation profile to this file")
	flag.Parse()
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			fail(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			fail(err)
		}
		defer pprof.StopCPUProfile()
	}
	if *memProfile != "" {
		runtime.MemProfileRate = 64 * 1024
		defer func() {
			f, err := os.Create(*memProfile)
			if err != nil {
				fail(err)
			}
			defer f.Close()
			if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
				fail(err)
			}
		}()
	}

	absInput, err := filepath.Abs(*input)
	if err != nil {
		fail(err)
	}
	if err := os.Chdir(filepath.Dir(absInput)); err != nil {
		fail(err)
	}

	var runs []stageTimes
	var lastSources map[string][]byte
	var hashes []string
	for i := 0; i < *reps; i++ {
		runtime.GC()
		var t stageTimes
		started := time.Now()
		loaded, err := frontend.LoadModuleWithOptions(absInput, frontend.LoadOptions{})
		if err != nil {
			fail(err)
		}
		t.load = time.Since(started)
		reportHeap("load")

		started = time.Now()
		program, err := air.Lower(loaded.Module)
		if err != nil {
			fail(err)
		}
		t.lower = time.Since(started)
		reportHeap("lower")

		started = time.Now()
		if err := air.Validate(program); err != nil {
			fail(err)
		}
		if program.Entry == air.NoFunction {
			fail(fmt.Errorf("no main function"))
		}
		if err := air.ValidateEntrypointSignature(program); err != nil {
			fail(err)
		}
		t.validate = time.Since(started)

		started = time.Now()
		sources, err := gotarget.GenerateSources(program, gotarget.Options{PackageName: "main", ProjectInfo: loaded.ProjectInfo})
		if err != nil {
			fail(err)
		}
		t.generate = time.Since(started)
		reportHeap("generate")

		runs = append(runs, t)
		lastSources = sources
		hashes = append(hashes, hashSources(sources))
	}

	for i := 1; i < len(hashes); i++ {
		if hashes[i] != hashes[0] {
			fmt.Fprintf(os.Stderr, "warning: generated sources differ between rep 0 and rep %d\n", i)
		}
	}

	if *outDir != "" {
		if err := os.RemoveAll(*outDir); err != nil {
			fail(err)
		}
		for name, src := range lastSources {
			path := filepath.Join(*outDir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				fail(err)
			}
			if err := os.WriteFile(path, src, 0o644); err != nil {
				fail(err)
			}
		}
	}

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	fmt.Printf("METRIC ard_ms=%.1f\n", ms(median(runs, func(t stageTimes) time.Duration { return t.total() })))
	fmt.Printf("METRIC load_ms=%.1f\n", ms(median(runs, func(t stageTimes) time.Duration { return t.load })))
	fmt.Printf("METRIC lower_ms=%.1f\n", ms(median(runs, func(t stageTimes) time.Duration { return t.lower })))
	fmt.Printf("METRIC validate_ms=%.1f\n", ms(median(runs, func(t stageTimes) time.Duration { return t.validate })))
	fmt.Printf("METRIC generate_ms=%.1f\n", ms(median(runs, func(t stageTimes) time.Duration { return t.generate })))
	fmt.Printf("METRIC total_alloc_mb=%.1f\n", float64(mem.TotalAlloc)/(1024*1024)/float64(*reps))
	fmt.Printf("METRIC num_gc=%d\n", int(mem.NumGC)/(*reps))
	fmt.Printf("METRIC source_files=%d\n", len(lastSources))
	fmt.Printf("SOURCE_HASH %s\n", hashes[len(hashes)-1])
	for i, r := range runs {
		fmt.Printf("REP %d load=%.0f lower=%.0f validate=%.0f generate=%.0f\n", i, ms(r.load), ms(r.lower), ms(r.validate), ms(r.generate))
	}
}

// reportHeap prints live-heap figures after a stage when ARD_BENCH_HEAP is set.
func reportHeap(stage string) {
	if os.Getenv("ARD_BENCH_HEAP") == "" {
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Fprintf(os.Stderr, "heap after %-8s inuse=%4dMB sys=%4dMB total_alloc=%5dMB gc=%d\n", stage, mem.HeapInuse>>20, mem.HeapSys>>20, mem.TotalAlloc>>20, mem.NumGC)
}

func hashSources(sources map[string][]byte) string {
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write(sources[name])
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func median(runs []stageTimes, pick func(stageTimes) time.Duration) time.Duration {
	values := make([]time.Duration, len(runs))
	for i, r := range runs {
		values[i] = pick(r)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values[len(values)/2]
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

func fail(err error) {
	fmt.Fprintln(os.Stderr, "compilebench:", err)
	os.Exit(1)
}
