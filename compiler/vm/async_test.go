package vm_test

import (
	"testing"
	"time"
)

func TestAsyncSleep(t *testing.T) {
	start := time.Now()
	run(t, `
		use ard/async

		async::sleep(10)
	`)
	elapsed := time.Since(start)
	if elapsed < time.Duration(10*time.Nanosecond) {
		t.Errorf("Expected script to take >= 10ns, but took %v", elapsed)
	}
}

func TestWaitingOnFibers(t *testing.T) {
	start := time.Now()

	run(t, `
		use ard/async
		use ard/duration

		// Start 3 fibers that each sleep for up to 500ns
		let fiber1 = async::start(fn() {
			async::sleep(500)
		})
		let fiber2 = async::start(fn() {
			async::sleep(100)
		})
		let fiber3 = async::start(fn() {
			async::sleep(100)
		})

		// Wait for all fibers to complete
		fiber1.join()
		fiber2.join()
		fiber3.join()
	`)

	elapsed := time.Since(start)
	min := 500 * time.Nanosecond

	// Should take at least 500ns (longest sleep) due to concurrency, not ~700ns (sequential)
	if elapsed < min {
		t.Errorf("Expected concurrent execution to take at least 500ns, but took %v", elapsed)
	}
}

func TestAsync(t *testing.T) {
	runTests(t, []test{
		{
			name: "async::eval() for obtaining values concurrently",
			input: `
				use ard/async

				fn add_2(x: Int) Int { x + 2 }

				let fiber = async::eval(fn() { add_2(5) })
				let got = fiber.get()
				if not got == 7 {
					panic("Expected 7 and got {got}")
				}
			`,
		},
	})
}

func TestAsyncJoin(t *testing.T) {
	start := time.Now()

	run(t, `
		use ard/async
		use ard/duration

		async::join([
			async::start(fn() { async::sleep(duration::from_millis(100)) }),
			async::start(fn() { async::sleep(duration::from_millis(100)) }),
			async::start(fn() { async::sleep(duration::from_millis(200)) }),
		])
	`)

	elapsed := time.Since(start)
	// Should take at least 200ms (longest sleep), accounting for interpreter overhead
	minDuration := 200 * time.Millisecond

	if elapsed < minDuration {
		t.Errorf("Expected concurrent execution to take at least %v, but took %v", minDuration, elapsed)
	}

	// Should not take much longer than 300ms (200ms sleep + overhead)
	maxDuration := 300 * time.Millisecond

	if elapsed > maxDuration {
		t.Errorf("Expected concurrent execution to take at most %v, but took %v", maxDuration, elapsed)
	}
}

func TestFiberTypeParams(t *testing.T) {
	runTests(t, []test{
		{
			name: "async::start() returns a Fiber<Void>",
			input: `
				use ard/async

				let fiber: async::Fiber<Void> = async::start(fn() {})
				fiber.get()
			`,
			want: nil,
		},
		{
			name: "Fibers can hold Results",
			input: `
				use ard/async

				let fiber: async::Fiber<Int!Bool> = async::eval(fn() Int!Bool { Result::ok(100) })
				fiber.get().or(0)
			`,
			want: 100,
		},
	})
}
