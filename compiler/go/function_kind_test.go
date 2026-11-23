package gotarget

import "testing"

// The backend tells lifted closures apart from declared functions by
// air.Function.Kind, so a declared function whose name looks like a
// compiler-generated closure name keeps its natural Go name.
func TestDeclaredFunctionNamedLikeClosureKeepsNaturalName(t *testing.T) {
	source := `use go:fmt

fn anon_func_3_1(x: Int) Int { x + 1 }

fn main() {
  let f = anon_func_3_1
  let k = 10
  let g = fn(x: Int) Int { x + k }
  fmt::Println(f(1), anon_func_3_1(2), g(1))
}
`
	files := lowerProgramAST(t, lowerSource(t, source), Options{PackageName: "main"})
	if _, ok := astFilesFunc(files, "AnonFunc31"); !ok {
		t.Fatal("generated AST missing natural name AnonFunc31 for the declared function")
	}
	if got := runGoTargetSourceStdout(t, source); got != "2 3 11\n" {
		t.Fatalf("stdout = %q, want %q", got, "2 3 11\n")
	}
}
