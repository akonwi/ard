package ffi

// Fiber is a handle to concurrent work started with Start. It has value
// semantics (it wraps a channel), so Ard holds it as a plain `Fiber` value
// rather than a mutable pointer.
type Fiber struct {
	done chan struct{}
}

// Start runs do on a new goroutine and returns a Fiber to await its completion.
func Start(do func()) Fiber {
	f := Fiber{done: make(chan struct{})}
	go func() {
		defer close(f.done)
		do()
	}()
	return f
}

// Wait blocks until the fiber's work has finished.
func (f Fiber) Wait() {
	<-f.done
}
