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

func TestConcurrentReadsOfVariables(t *testing.T) {
	result := run(t, `
		use ard/async

		// Shared immutable variables that all fibers will access
		let shared_number = 42
		let shared_string = "hello world"
		let shared_list = [1, 2, 3, 4, 5]

		mut fibers: [async::Fiber] = []

		for i in 0..30 {
			match i % 3 {
				0 => {
					fibers.push(async::start(fn() {
						shared_number + shared_string.size() + shared_list.at(0)
					}))
				},
				1 => {
					fibers.push(async::start(fn() {
						shared_number * 2 + shared_list.at(1)
					}))
				},
				_ => {
					fibers.push(async::start(fn() {
						shared_string.size() + shared_list.at(2)
					}))
				}
			}
		}

		for f in fibers {
			f.join()
		}
		"success"
	`)

	// Run the test multiple times to stress test the scope system
	if result != "success" {
		t.Errorf("Expected 'success', got %v", result)
	}
}
