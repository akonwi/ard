package vm_test

import (
	"testing"
	"time"
)

func TestAsyncSleep(t *testing.T) {
	start := time.Now()
	run(t, `
		use ard/async

		async::sleep(1000)
	`)
	elapsed := time.Since(start)
	if elapsed < time.Duration(1000*time.Millisecond) {
		t.Errorf("Expected script to take >= 105ms, but took %v", elapsed)
	}
}

func TestWaitingOnFibers(t *testing.T) {
	start := time.Now()

	run(t, `
		use ard/async

		// Start 3 fibers that each sleep for 100ms
		let fiber1 = async::start(fn() {
			async::sleep(1000)
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

	// Should take ~1s (longest fiber) due to concurrency, not ~1.2s (sequential)
	if elapsed < 1*time.Second {
		t.Errorf("Expected concurrent execution to take at least 1s, but took %v", elapsed)
	}

	// Allow some overhead, but shouldn't take much longer than the longest fiber
	if elapsed > 1500*time.Millisecond {
		t.Errorf("Expected concurrent execution to take at most 1.5s, but took %v", elapsed)
	}
}
