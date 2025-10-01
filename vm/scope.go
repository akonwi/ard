package vm

import (
	"maps"
	"sync"
	"sync/atomic"

	"github.com/akonwi/ard/runtime"
)

type scopeData struct {
	bindings  map[string]*runtime.Object
	breakable bool
	broken    bool
	refCount  int32 // atomic reference counter
}

type scope struct {
	parent *scope
	data   *scopeData // shared, immutable until copied
	mu     sync.RWMutex
}

func newScope(parent *scope) *scope {
	return &scope{
		parent: parent,
		data: &scopeData{
			bindings: make(map[string]*runtime.Object),
			refCount: 1,
		},
	}
}

func (s *scope) clone() *scope {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Increment reference count atomically
	atomic.AddInt32(&s.data.refCount, 1)

	return &scope{
		parent: s.parent, // Share parent reference (no recursive copy!)
		data:   s.data,   // Share data (COW semantics)
	}
}

// fork creates a fiber-optimized clone for read-only access
func (s *scope) fork() *scope {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Since fibers are read-only, we can safely share without incrementing refCount
	// This avoids any COW overhead for pure read access
	return &scope{
		parent: s.parent,
		data:   s.data,
	}
}

func (s *scope) add(name string, value *runtime.Object) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to copy-on-write
	if atomic.LoadInt32(&s.data.refCount) > 1 {
		s.copyData()
	}

	s.data.bindings[name] = value
}

// copyData creates a private copy of the shared data
func (s *scope) copyData() {
	// Decrement old reference count
	atomic.AddInt32(&s.data.refCount, -1)

	// Create new private copy
	newData := &scopeData{
		bindings:  make(map[string]*runtime.Object),
		breakable: s.data.breakable,
		broken:    s.data.broken,
		refCount:  1,
	}

	// Copy bindings
	maps.Copy(newData.bindings, s.data.bindings)

	s.data = newData
}

func (s *scope) get(name string) (*runtime.Object, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data.bindings[name]
	if !ok && s.parent != nil {
		return s.parent.get(name)
	}
	return v, ok
}

func (s *scope) set(name string, value *runtime.Object) {
	if binding, ok := s.get(name); ok {
		*binding = *value
	}
}

func (s *scope) _break() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to copy-on-write
	if atomic.LoadInt32(&s.data.refCount) > 1 {
		s.copyData()
	}

	if s.data.breakable {
		s.data.broken = true
	} else if s.parent != nil {
		s.parent._break()
	}
}

// setBroken unconditionally sets broken to true (for try-catch early returns)
func (s *scope) setBroken(broken bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to copy-on-write
	if atomic.LoadInt32(&s.data.refCount) > 1 {
		s.copyData()
	}

	s.data.broken = broken
}

// Helper methods for accessing data fields
func (s *scope) setBreakable(breakable bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to copy-on-write
	if atomic.LoadInt32(&s.data.refCount) > 1 {
		s.copyData()
	}

	s.data.breakable = breakable
}

func (s *scope) isBreakable() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.breakable
}

func (s *scope) isBroken() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.broken
}
