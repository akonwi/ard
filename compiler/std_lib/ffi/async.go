package ffi

// Spawn runs do on a new goroutine. This is the one irreducible concurrency
// primitive, since Ard has no `go` statement; coordination (waiting, results)
// is done with channels in ordinary Ard code.
func Spawn(do func()) {
	go do()
}
