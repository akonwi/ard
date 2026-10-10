package checker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

// A Go alias is the same type as its target, including behind a pointer:
// `*Alias` imports as the same `&mut T` foreign pointer as `*Value` (#512).
func TestGoPointerToAliasKeepsTargetIdentity(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/app\n\ngo 1.27\n",
		"ffi/ffi.go": `package ffi

type Value struct{ N int }

type Alias = Value

type Update struct {
	Snapshot *Alias
	Plain    *Value
}

func Take(value *Alias) int { return value.N }

func All() []*Alias { return nil }
`,
		"ffi/contract/contract.go": `package contract

type SessionSnapshot struct{ ID string }

type SessionUpdate struct{ Snapshot *SessionSnapshot }
`,
		"ffi/api/api.go": `package api

import "example.com/app/ffi/contract"

type SessionSnapshot = contract.SessionSnapshot

type SessionUpdate struct{ Snapshot *SessionSnapshot }
`,
	}
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name   string
		source string
	}{
		{name: "pointer-to-alias field as alias pointer", source: "use go:example.com/app/ffi\n\nfn snapshot(update: ffi::Update) &mut ffi::Alias {\n  update.Snapshot\n}"},
		{name: "pointer-to-alias field as target pointer", source: "use go:example.com/app/ffi\n\nfn snapshot(update: ffi::Update) &mut ffi::Value {\n  update.Snapshot\n}"},
		{name: "target pointer field as alias pointer", source: "use go:example.com/app/ffi\n\nfn plain(update: ffi::Update) &mut ffi::Alias {\n  update.Plain\n}"},
		{name: "annotated binding", source: "use go:example.com/app/ffi\n\nfn read(update: ffi::Update) Int {\n  let snapshot: &mut ffi::Value = update.Snapshot\n  snapshot.N\n}"},
		{name: "pointer-to-alias parameter", source: "use go:example.com/app/ffi\n\nfn take() Int {\n  ffi::Take(&mut ffi::Value{N: 1})\n}"},
		{name: "slice of pointer-to-alias", source: "use go:example.com/app/ffi\n\nfn all() [&mut ffi::Value] {\n  ffi::All()\n}"},
		{name: "cross-package alias", source: "use go:example.com/app/ffi/api\n\nfn snapshot(update: api::SessionUpdate) &mut api::SessionSnapshot {\n  update.Snapshot\n}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parse.Parse([]byte(tt.source+"\n"), "test.ard")
			if len(result.Errors) > 0 {
				t.Fatalf("parse errors: %v", result.Errors)
			}
			// Each source primes its own Go import set (ADR 0044).
			resolver := checker.NewGoPackagesResolver(root, nil)
			c := checker.New("test.ard", result.Program, nil, checker.CheckOptions{GoResolver: resolver})
			c.Check()
			if c.HasErrors() {
				t.Fatalf("checker diagnostics: %v", c.Diagnostics())
			}
		})
	}
}
