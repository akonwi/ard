package vm_test

import (
	"testing"
	"time"
)

func TestAsyncSleep(t *testing.T) {
	start := time.Now()
	run(t, `
		use ard/async

		async::sleep(100)
	`)
	elapsed := time.Since(start)
	if elapsed < time.Duration(100*time.Millisecond) {
		t.Errorf("Expected script to take ~100ms, but took %v", elapsed)
	}
}

func TestWaitingOnFibers(t *testing.T) {
	start := time.Now()

	run(t, `
		use ard/async

		// Start 3 fibers that each sleep for 100ms
		let fiber1 = async::start(fn() {
			async::sleep(100)
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

	// If truly concurrent, should take ~100ms, not ~300ms
	// Allow some overhead, but should be significantly less than sequential
	if elapsed > 90*time.Millisecond {
		t.Errorf("Expected concurrent execution to take ~100ms, but took %v", elapsed)
	}

	// Shouldn't take 300ms since they were in parallel
	if elapsed > 300*time.Millisecond {
		t.Errorf("Expected at least 90ms execution time, but took %v", elapsed)
	}
}
