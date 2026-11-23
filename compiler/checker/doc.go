// Package checker type-checks parsed Ard modules: it resolves imports,
// types, and symbols, reports diagnostics, and produces the checked program
// that AIR lowering consumes.
//
// The checker is internal to the compiler, not a stable API for outside
// tools: its types and functions may change in any release. A tool that uses
// it as a Go library should pin the compiler version it builds against.
package checker
