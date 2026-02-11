package vm

import (
	"testing"
	"time"
)

func TestBytecodeAsyncSleep(t *testing.T) {
	start := time.Now()
	runBytecode(t, `
		use ard/async
		async::sleep(1000000)
	`)
	if elapsed := time.Since(start); elapsed < time.Millisecond {
		t.Fatalf("Expected script to take >= 1ms, took %v", elapsed)
	}
}

func TestBytecodeWaitingOnFibers(t *testing.T) {
	start := time.Now()
	runBytecode(t, `
		use ard/async
		let fiber1 = async::start(fn() { async::sleep(2000000) })
		let fiber2 = async::start(fn() { async::sleep(1000000) })
		let fiber3 = async::start(fn() { async::sleep(1000000) })
		fiber1.join()
		fiber2.join()
		fiber3.join()
	`)
	if elapsed := time.Since(start); elapsed < 2*time.Millisecond {
		t.Fatalf("Expected concurrent execution >= 2ms, got %v", elapsed)
	}
}

func TestBytecodeAsync(t *testing.T) {
	runBytecodeTests(t, []vmTestCase{
		{
			name: "async::eval obtains values concurrently",
			input: `
				use ard/async
				fn add_2(x: Int) Int { x + 2 }
				let fiber = async::eval(fn() { add_2(5) })
				let got = fiber.get()
				if not got == 7 { panic("Expected 7 and got {got}") }
			`,
			want: nil,
		},
	})
}

func TestBytecodeAsyncJoin(t *testing.T) {
	start := time.Now()
	runBytecode(t, `
		use ard/async
		use ard/duration
		async::join([
			async::start(fn() { async::sleep(duration::from_millis(20)) }),
			async::start(fn() { async::sleep(duration::from_millis(20)) }),
			async::start(fn() { async::sleep(duration::from_millis(40)) }),
		])
	`)
	elapsed := time.Since(start)
	if elapsed < 40*time.Millisecond {
		t.Fatalf("Expected concurrent execution >= 40ms, got %v", elapsed)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("Expected concurrent execution <= 200ms, got %v", elapsed)
	}
}

func TestBytecodeFiberTypeParams(t *testing.T) {
	t.Run("async::start returns Fiber<Void>", func(t *testing.T) {
		if got := runBytecode(t, `
			use ard/async
			let fiber: async::Fiber<Void> = async::start(fn() {})
			fiber.get()
		`); got != nil {
			t.Fatalf("Expected nil, got %v", got)
		}
	})

	t.Run("Fibers can hold Results", func(t *testing.T) {
		if got := runBytecode(t, `
			use ard/async
			let fiber: async::Fiber<Int!Bool> = async::eval(fn() Int!Bool { Result::ok(100) })
			fiber.get().or(0)
		`); got != 100 {
			t.Fatalf("Expected 100, got %v", got)
		}
	})
}
