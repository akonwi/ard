package air

import (
	"strings"
	"testing"
)

func TestLowerGlobalInitializerWithLocalBinding(t *testing.T) {
	program := lowerSource(t, `
		let answer = {
			let value = 42
			value
		}

		fn main() {
			if answer != 42 { panic("bad") }
		}
	`)

	if err := Validate(program); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
	if len(program.Globals) != 1 {
		t.Fatalf("global count = %d, want 1", len(program.Globals))
	}
	global := program.Globals[0]
	if len(global.Initializer.Locals) != 1 {
		t.Fatalf("initializer locals = %#v, want value local", global.Initializer.Locals)
	}
	local := global.Initializer.Locals[0]
	if local.ID != 0 || local.Name != "value" || local.Type != global.Type {
		t.Fatalf("initializer local = %#v, want value local with global type", local)
	}
	if global.Initializer.Value.Kind != ExprBlock {
		t.Fatalf("initializer kind = %d, want ExprBlock", global.Initializer.Value.Kind)
	}
	body := global.Initializer.Value.BlockPayload().Body
	if len(body.Stmts) != 1 || body.Stmts[0].Kind != StmtLet || body.Stmts[0].Local != local.ID {
		t.Fatalf("initializer statements = %#v, want let for local %d", body.Stmts, local.ID)
	}
	if body.Result == nil || body.Result.Kind != ExprLoadLocal || body.Result.LocalPayload().Local != local.ID {
		t.Fatalf("initializer result = %#v, want load of local %d", body.Result, local.ID)
	}
}

func TestLowerGlobalInitializerPreservesContextualContainerType(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "Result ok",
			source: `
				let saved: Int!Str = { Result::ok(42) }
				fn main() Int { saved.expect("bad") }
			`,
		},
		{
			name: "Result err",
			source: `
				let saved: Int!Str = { Result::err("bad") }
				fn main() Int {
					match saved {
						ok(value) => value,
						err(_) => 42,
					}
				}
			`,
		},
		{
			name: "Maybe",
			source: `
				let saved: Int? = { Maybe::new(42) }
				fn main() Int { saved.expect("bad") }
			`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := lowerSource(t, test.source)
			global := program.Globals[0]
			if global.Initializer.Value.Type != global.Type {
				t.Fatalf("initializer type = %d, want global type %d", global.Initializer.Value.Type, global.Type)
			}
		})
	}
}

func TestLowerGlobalInitializersOwnIndependentLocalNamespaces(t *testing.T) {
	program := lowerSource(t, `
		let first = {
			let value = 20
			value
		}
		let second = {
			let value = 22
			value
		}
		fn main() Int { first + second }
	`)
	if len(program.Globals) != 2 {
		t.Fatalf("global count = %d, want 2", len(program.Globals))
	}
	for _, global := range program.Globals {
		if len(global.Initializer.Locals) != 1 || global.Initializer.Locals[0].ID != 0 {
			t.Fatalf("global %s locals = %#v, want independent local 0", global.Name, global.Initializer.Locals)
		}
	}
}

func TestValidateRejectsMalformedGlobalInitializerLocals(t *testing.T) {
	validValue := func(local LocalID) Expr {
		return Expr{
			Kind: ExprBlock,
			Type: 1,
			Payload: &BlockExprPayload{Body: Block{
				Stmts:  []Stmt{{Kind: StmtLet, Local: local, Name: "value", Type: 1, Value: &Expr{Kind: ExprConstInt, Type: 1, Payload: &TextExprPayload{Value: "42"}}}},
				Result: &Expr{Kind: ExprLoadLocal, Type: 1, Payload: &LocalExprPayload{Local: local}},
			}},
		}
	}
	tests := []struct {
		name    string
		locals  []Local
		value   Expr
		wantErr string
	}{
		{name: "missing local", value: validValue(0), wantErr: "invalid local 0"},
		{name: "noncanonical local id", locals: []Local{{ID: 1, Name: "value", Type: 1}}, value: validValue(0), wantErr: "local table entry 0 has id 1"},
		{name: "invalid local type", locals: []Local{{ID: 0, Name: "value", Type: 99}}, value: validValue(0), wantErr: "local value has invalid type 99"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program := &Program{
				Modules: []Module{{ID: 0, Path: "main.ard", Globals: []GlobalID{0}}},
				Types:   []TypeInfo{{ID: 1, Kind: TypeInt, Name: "Int"}},
				Globals: []Global{{
					ID:     0,
					Module: 0,
					Name:   "answer",
					Type:   1,
					Initializer: GlobalInitializer{
						Locals: test.locals,
						Value:  test.value,
					},
				}},
				Entry:  NoFunction,
				Script: NoFunction,
			}
			err := Validate(program)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
