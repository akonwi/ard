package air

import "testing"

func TestLowerFinalRecursiveLocalDeclarationLoadsExistingBinding(t *testing.T) {
	program := lowerSource(t, `
		fn make_add(base: Int) fn(Int) Int {
			fn add_inner(value: Int) Int {
				if value == 0 { base } else { 1 + add_inner(value - 1) }
			}
		}
	`)

	makeAdd := findFunction(t, program, "make_add")
	assertFinalRecursiveLocalBinding(t, program, makeAdd.Body, "add_inner")
}

func TestLowerGlobalInitializerFinalRecursiveLocalDeclarationLoadsExistingBinding(t *testing.T) {
	program := lowerSource(t, `
		let add: fn(Int) Int = {
			let base = 40
			fn add_inner(value: Int) Int {
				if value == 0 { base } else { 1 + add_inner(value - 1) }
			}
		}
	`)

	if len(program.Globals) != 1 || program.Globals[0].Initializer.Value.Kind != ExprBlock {
		t.Fatalf("globals = %#v, want block-valued add", program.Globals)
	}
	body := program.Globals[0].Initializer.Value.BlockPayload().Body
	assertFinalRecursiveLocalBinding(t, program, body, "add_inner")
}

func assertFinalRecursiveLocalBinding(t *testing.T, program *Program, body Block, name string) {
	t.Helper()
	var binding *Stmt
	for i := range body.Stmts {
		if body.Stmts[i].LocalFunction {
			if binding != nil {
				t.Fatalf("body statements = %#v, want one local function binding", body.Stmts)
			}
			binding = &body.Stmts[i]
		}
	}
	if binding == nil || binding.Kind != StmtLet || !binding.Predeclare || binding.Value == nil || binding.Value.Kind != ExprMakeClosure {
		t.Fatalf("body statements = %#v, want predeclared recursive closure", body.Stmts)
	}
	if body.Result == nil || body.Result.Kind != ExprLoadLocal {
		t.Fatalf("body result = %#v, want existing local binding load", body.Result)
	}
	result := body.Result.LocalPayload()
	if result == nil || result.Local != binding.Local {
		t.Fatalf("body result local = %#v, want binding local %d", result, binding.Local)
	}
	payload := binding.Value.CallPayload()
	if payload == nil || payload.Function < 0 || int(payload.Function) >= len(program.Functions) {
		t.Fatalf("closure payload = %#v, want valid helper", payload)
	}
	closure := program.Functions[payload.Function]
	foundSelf := false
	foundBase := false
	for _, capture := range closure.Captures {
		switch capture.Name {
		case name:
			foundSelf = true
			if capture.Mode != CaptureSlot {
				t.Fatalf("self capture = %#v, want CaptureSlot", capture)
			}
		case "base":
			foundBase = true
			if capture.Mode != CaptureValue {
				t.Fatalf("base capture = %#v, want CaptureValue", capture)
			}
		}
	}
	if !foundSelf || !foundBase {
		t.Fatalf("closure captures = %#v, want self slot %q and base value", closure.Captures, name)
	}
}
