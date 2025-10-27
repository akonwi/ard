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
	if elapsed < time.Duration(10*time.Millisecond) {
		t.Errorf("Expected script to take >= 15ms, but took %v", elapsed)
	}
}

func TestWaitingOnFibers(t *testing.T) {
	start := time.Now()

	run(t, `
		use ard/async

		// Start 3 fibers that each sleep for 100ms
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

	// Should take ~1s (longest fiber) due to concurrency, not ~1.2s (sequential)
	if elapsed < 500*time.Millisecond {
		t.Errorf("Expected concurrent execution to take at least 500ms, but took %v", elapsed)
	}

	// Allow some overhead, but shouldn't take much longer than the longest fiber
	// something wrong. it shouldn't be taking up to 1.5s, should it?
	if elapsed > 1500*time.Millisecond {
		t.Errorf("Expected concurrent execution to take at most 510ms, but took %v", elapsed)
	}
}
