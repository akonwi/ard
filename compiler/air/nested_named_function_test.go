package air

import (
	"strings"
	"testing"
)

func TestLowerNestedNamedFunctionBindsClosureLocal(t *testing.T) {
	program := lowerSource(t, `
		fn outer(value: Int) Int {
			fn inner() Int { value }
			inner()
		}
	`)
	outer := findFunction(t, program, "outer")
	if len(outer.Body.Stmts) != 1 {
		t.Fatalf("outer statements = %d, want one local function binding", len(outer.Body.Stmts))
	}
	binding := outer.Body.Stmts[0]
	if binding.Kind != StmtLet || !binding.LocalFunction || binding.Value == nil || binding.Value.Kind != ExprMakeClosure {
		t.Fatalf("local declaration = %#v, want named closure-valued let", binding)
	}
	if outer.Body.Result == nil || outer.Body.Result.Kind != ExprCallClosure {
		t.Fatalf("outer result = %#v, want closure call", outer.Body.Result)
	}
	if outer.Body.Result.Target == nil || outer.Body.Result.Target.Kind != ExprLoadLocal {
		t.Fatalf("closure call target = %#v, want local load", outer.Body.Result.Target)
	}
	load := outer.Body.Result.Target.LocalPayload()
	if load == nil || load.Local != binding.Local {
		t.Fatalf("closure call local = %#v, want declaration local %d", load, binding.Local)
	}
	payload := binding.Value.CallPayload()
	if payload == nil || payload.Function < 0 || int(payload.Function) >= len(program.Functions) {
		t.Fatalf("closure payload = %#v, want valid function", payload)
	}
	closure := program.Functions[payload.Function]
	if len(closure.Captures) != 1 || closure.Captures[0].Name != "value" || closure.Captures[0].Mode != CaptureValue {
		t.Fatalf("closure captures = %#v, want value capture", closure.Captures)
	}
}

func TestValidateRejectsMalformedLocalFunctionBinding(t *testing.T) {
	program := &Program{
		Modules: []Module{{ID: 0, Path: "test", Functions: []FunctionID{0}}},
		Types:   []TypeInfo{{ID: 1, Kind: TypeInt, Name: "Int"}},
		Functions: []Function{{
			ID:        0,
			Module:    0,
			Name:      "main",
			Signature: Signature{Return: 1},
			Locals:    []Local{{ID: 0, Name: "inner", Type: 1}},
			Body: Block{
				Stmts: []Stmt{{
					Kind:          StmtLet,
					Local:         0,
					Type:          1,
					LocalFunction: true,
					Value:         &Expr{Kind: ExprConstInt, Type: 1, Payload: &TextExprPayload{Value: "1"}},
				}},
				Result: &Expr{Kind: ExprConstInt, Type: 1, Payload: &TextExprPayload{Value: "1"}},
			},
		}},
		Entry:  0,
		Script: NoFunction,
	}
	if err := Validate(program); err == nil || !strings.Contains(err.Error(), "local function binding must be a closure-valued let") {
		t.Fatalf("Validate error = %v, want malformed local function binding", err)
	}
}

func TestLowerRecursiveNestedNamedFunctionCapturesSelfSlot(t *testing.T) {
	program := lowerSource(t, `
		fn outer(base: Int) Int {
			fn add(value: Int) Int {
				match value == 0 {
					true => base,
					false => 1 + add(value - 1),
				}
			}
			add(2)
		}
	`)
	outer := findFunction(t, program, "outer")
	if len(outer.Body.Stmts) != 1 || outer.Body.Stmts[0].Value == nil {
		t.Fatalf("outer body = %#v, want local recursive binding", outer.Body)
	}
	binding := outer.Body.Stmts[0]
	if !binding.Predeclare || !binding.LocalFunction {
		t.Fatalf("recursive binding = %#v, want predeclared local function", binding)
	}
	payload := binding.Value.CallPayload()
	if payload == nil || payload.Function < 0 || int(payload.Function) >= len(program.Functions) {
		t.Fatalf("closure payload = %#v, want valid function", payload)
	}
	closure := program.Functions[payload.Function]
	var self *Capture
	for i := range closure.Captures {
		if closure.Captures[i].Name == "add" {
			self = &closure.Captures[i]
			break
		}
	}
	if self == nil || self.Mode != CaptureSlot {
		t.Fatalf("recursive closure captures = %#v, want self CaptureSlot", closure.Captures)
	}
}
