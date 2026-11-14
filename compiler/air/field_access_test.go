package air

import (
	"strings"
	"testing"
)

// A field read targets a struct or a reference to one. A read through a Maybe
// reaches AIR as an explicit option match (ADR 0066), so the Go backend has no
// lowering for any other target (#589).
func TestValidateRejectsMalformedFieldReads(t *testing.T) {
	base := func() *Program {
		return lowerSource(t, `
			struct User {
				name: Str,
			}

			fn name(user: User, maybe: User?) Str {
				user.name
			}

			fn main() {}
		`)
	}
	fieldRead := func(program *Program) (Function, *Expr) {
		fn := findFunction(t, program, "name")
		read := fn.Body.Result
		if read == nil || read.Kind != ExprGetField {
			t.Fatalf("name result = %+v, want a field read", read)
		}
		return fn, read
	}

	tests := []struct {
		name    string
		mutate  func(*Program)
		wantErr string
	}{
		{
			name: "missing target",
			mutate: func(program *Program) {
				_, read := fieldRead(program)
				read.Target = nil
			},
			wantErr: "field read is missing its target",
		},
		{
			name: "maybe target",
			mutate: func(program *Program) {
				fn, read := fieldRead(program)
				for id, local := range fn.Locals {
					if local.Name == "maybe" {
						read.Target = loadLocal(local.Type, LocalID(id))
						return
					}
				}
				t.Fatal("name has no local for its maybe parameter")
			},
			wantErr: "field read has non-struct target type",
		},
		{
			name: "field out of range",
			mutate: func(program *Program) {
				_, read := fieldRead(program)
				read.Payload = &FieldExprPayload{Field: 1}
			},
			wantErr: "field read has invalid index 1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			program := base()
			tt.mutate(program)
			err := Validate(program)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
