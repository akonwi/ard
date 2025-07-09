# Asynchronous programs

Work in progress. Ideas for inspiration:
- risor
- inko
- rust

Ard takes inspiration for async execution from Go and Rust. From Go, Ard uses goroutines as the async implementation.
From Rust, Ard uses the safety guardrails of policing shared memory between execution contexts.

There are currently no OS threads in Ard. Separate execution contexts are called Fibers.

> This keeps the possibility of OS-level threads open without naming issues

Currently, all fibers are goroutines under the hood, meaning they are "green" threads managed by the Go runtime.

Async functionality is available in the `ard/async` module of the standard library.

From the current (main) fiber, a program can sleep for a duration of milliseconds.

```ard
use ard/async
use ard/io

fn main() {
  io::print("hello...")
  async::sleep(1000)
  io::print("world!")
}
```

Note how there's no `async`, `await`, or even `go` keyword. Ard avoids Javascript's problem of infectious `async` functions.

To run code concurrently, use the `async::start()` function and provide a callback.

```ard
use ard/async
use ard/io

fn main() {
  io::print("1")
  async::start(fn() {
    io::print("2")
  })
  io::print("3")
}
```

The printed output of that program is not guaranteed to have the numbers in order because there's no control over when the async callback runs.

In order to "await" a fiber, the created fiber provides a `.wait()` method.

```ard
use ard/async
use ard/io

fn main() {
  io::print("1")
  let fiber = async::start(fn() {
    async::sleep(5000)
    io::print("2")
  })
  fiber.wait()
  io::print("3")
}
```

Now, the output of this program will be:
```
1
2
3
```
With a 5 second delay between 2 and 3.
