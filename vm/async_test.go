package vm_test

import (
	"testing"
	"time"
)

func TestAsyncStart(t *testing.T) {
	run(t, `
		use ard/async

		// Start a simple fiber with no operations
		let fiber = async::start(fn() {
			// empty function
		})

		// Wait for it to complete
		fiber.wait()
	`)
}

func TestAsyncSleep(t *testing.T) {
	run(t, `
		use ard/async

		// Test sleep function directly
		async::sleep(1)
	`)
}

func TestConcurrentSleep(t *testing.T) {
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
		fiber1.wait()
		fiber2.wait()
		fiber3.wait()
	`)

	elapsed := time.Since(start)

	// If truly concurrent, should take ~100ms, not ~300ms
	// Allow some overhead, but should be significantly less than sequential
	if elapsed > 200*time.Millisecond {
		t.Errorf("Expected concurrent execution to take ~100ms, but took %v", elapsed)
	}

	// Should take at least 100ms to prove sleep actually works
	if elapsed < 90*time.Millisecond {
		t.Errorf("Expected at least 90ms execution time, but took %v", elapsed)
	}
}
