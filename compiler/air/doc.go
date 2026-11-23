// Package air is the Ard Intermediate Representation: the typed,
// target-neutral program that backends lower from after checking (ADR 0002).
//
// AIR is shaped by what compiler targets need and changes when they need
// something different. It is not a stable API for tools outside the
// compiler: types, fields, IDs, and node kinds may change in any release, and
// serialized AIR only round-trips within one compiler binary. A tool that
// reads AIR as a Go library should pin the compiler version it builds
// against.
package air
