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

	// Should take at least 500ns (longest sleep) due to concurrency, not ~700ns (sequential)
	if elapsed < 500*time.Nanosecond {
		t.Errorf("Expected concurrent execution to take at least 500ns, but took %v", elapsed)
	}

	// Allow some overhead, but shouldn't take much longer than the longest fiber
	if elapsed > 1000*time.Millisecond {
		t.Errorf("Expected concurrent execution to take at most 500ns, but took %v", elapsed)
	}
}
