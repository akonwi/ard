package parse

import (
	"strings"
	"testing"
)

type sexprCase struct {
	input string
	want  string
	// errs lists substrings of the expected parse errors, in order.
	errs []string
}

func runSexprCases(t *testing.T, cases []sexprCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.input, func(t *testing.T) {
			result := Parse([]byte(tt.input+"\n"), "test.ard")
			got := make([]string, 0, len(result.Program.Statements))
			for _, statement := range result.Program.Statements {
				got = append(got, sexpr(statement))
			}
			if joined := strings.Join(got, " ; "); joined != tt.want {
				t.Errorf("parsed as\n  %s\nwant\n  %s", joined, tt.want)
			}
			if len(result.Errors) != len(tt.errs) {
				t.Fatalf("got errors %v, want %d matching %q", result.Errors, len(tt.errs), tt.errs)
			}
			for i, want := range tt.errs {
				if !strings.Contains(result.Errors[i].Message, want) {
					t.Errorf("error %d = %q, want it to contain %q", i, result.Errors[i].Message, want)
				}
			}
		})
	}
}

// Issue #639: a call type argument that is itself generic, such as
// `same<Box<Int>>(b)`, parses as a call with that type argument attached.
// The lexer emits `>>` as two adjacent `>` tokens, so each one closes a list.
func TestNestedCallTypeArguments(t *testing.T) {
	runSexprCases(t, []sexprCase{
		{input: "f<Box<Int>>(x)", want: "(call f<Box<Int>> x)"},
		{input: "f<Map<Str, Int>>(x)", want: "(call f<Map<Str, Int>> x)"},
		{input: "f<Box<Box<Int>>>(x)", want: "(call f<Box<Box<Int>>> x)"},
		{input: "f<Box<Box<Box<Int>>>>(x)", want: "(call f<Box<Box<Box<Int>>>> x)"},
		{input: "f<Box<Int>, Str>(x)", want: "(call f<Box<Int>, Str> x)"},
		{input: "f<Str, Box<Int>>(x)", want: "(call f<Str, Box<Int>> x)"},
		{input: "f<Box<Int>?>(x)", want: "(call f<Box<Int>?> x)"},
		{input: "f<Box<Int>!Str>(x)", want: "(call f<Box<Int>!Str> x)"},
		{input: "f<Box<Int>>()", want: "(call f<Box<Int>> )"},
		{input: "f<Box<Int>> (x)", want: "(call f<Box<Int>> x)"},
		{input: "f<m::Box<Int>>(x)", want: "(call f<m::Box<Int>> x)"},
		{input: "a<m::b<c>>(d)", want: "(call a<m::b<c>> d)"},
		{input: "decode<List<Int>>(x)", want: "(call decode<List<Int>> x)"},
		{input: "let c = same<Box<Int>>(b)", want: "(let c (call same<Box<Int>> b))"},
		{input: "Maybe::new<Box<Int>>()", want: "(:: Maybe (call new<Box<Int>> ))"},
		{input: "mod::f<Box<Int>>(x)", want: "(:: mod (call f<Box<Int>> x))"},
		{input: "mod::f<Box<Box<Int>>>(x)", want: "(:: mod (call f<Box<Box<Int>>> x))"},
		{input: "x.f<Box<Int>>(y)", want: "(. x (call f<Box<Int>> y))"},
		{input: "x.f<Box<Box<Int>>>(y)", want: "(. x (call f<Box<Box<Int>>> y))"},
		{input: "f<Box<Int>>(x) > 1", want: "(> (call f<Box<Int>> x) 1)"},
		{input: "1 < f<Box<Int>>(x)", want: "(< 1 (call f<Box<Int>> x))"},
		{input: "foo(f<Box<Int>>(x))", want: "(call foo (call f<Box<Int>> x))"},
		// Lower-case names are valid type names, so this has the same shape
		// as `a<b>(c)`, which has always been a generic call.
		{input: "a<b<c>>(d)", want: "(call a<b<c>> d)"},
	})
}

// Comparisons and shifts that share tokens with call type arguments keep
// their meaning. Unless noted, each expectation is how the parser read the
// input before #639.
func TestCallTypeArgumentLookaheadKeepsComparisonsAndShifts(t *testing.T) {
	runSexprCases(t, []sexprCase{
		{input: "a < b", want: "(< a b)"},
		{input: "a<b", want: "(< a b)"},
		{input: "a < b > c", want: "(chain a < b > c)"},
		{input: "a<b > c", want: "(chain a < b > c)"},
		{input: "a<b>c", want: "(chain a < b > c)"},
		{input: "a < b < c", want: "(chain a < b < c)"},
		{input: "a<b<c", want: "(chain a < b < c)"},
		{input: "a<b<c>d", want: "(chain a < b < c > d)"},
		{input: "a<b<c> d", want: "(chain a < b < c > d)"},
		{input: "a < b < c > d", want: "(chain a < b < c > d)"},
		{input: "0 <= a < b <= 10", want: "(chain 0 <= a < b <= 10)"},
		{input: "a<b<c>(d)", want: "(< a (call b<c> d))"},
		{input: "x < y and z > w", want: "(and (< x y) (> z w))"},
		{input: "x<y and z>w", want: "(and (< x y) (> z w))"},
		{input: "x<y and z>(w)", want: "(and (< x y) (> z w))"},
		{input: "a << b", want: "(<< a b)"},
		{input: "a >> b", want: "(>> a b)"},
		{input: "a<<b", want: "(<< a b)"},
		{input: "a>>b", want: "(>> a b)"},
		{input: "a<b >> c", want: "(< a (>> b c))"},
		{input: "a<b>>c", want: "(< a (>> b c))"},
		{input: "a<b>>(c)", want: "(< a (>> b c))"},
		{input: "a<b<<c", want: "(< a (<< b c))"},
		{input: "f<<g(1)", want: "(<< f (call g 1))"},
		{input: "a<1>(b)", want: "(chain a < 1 > b)"},
		// Closing `>>` not followed by `(` is a shift inside a comparison.
		{input: "a<b<c>>d", want: "(chain a < b < (>> c d))"},
		{input: "a<b<c>> d", want: "(chain a < b < (>> c d))"},
		{input: "a<b<c>>2", want: "(chain a < b < (>> c 2))"},
		{input: "m::a<b<c>>d", want: "(chain (:: m a) < b < (>> c d))"},
		// A nested `<` that does not open a clean type argument list means
		// the outer `<` is a comparison too.
		{input: "a<b<1>>(d)", want: "(chain a < b < (>> 1 d))"},
		{input: "a<b<-1>>(d)", want: "(chain a < b < (>> (- 1) d))"},
		{input: "a<b<-c>>(d)", want: "(chain a < b < (>> (- c) d))"},
		{input: "a<b<\"s\">>(d)", want: "(chain a < b < (>> \"s\" d))"},
		{input: "a<b<true>>(d)", want: "(chain a < b < (>> true d))"},
		{input: "a<b<not c>>(d)", want: "(chain a < b < (not (>> c d)))"},
		{input: "a<b<c + 1>>(d)", want: "(chain a < b < (+ c (>> 1 d)))"},
		{input: "a<b<c.d>>(e)", want: "(chain a < b < (>> (. c d) e))"},
		{input: "x.a<b<1>>(d)", want: "(chain (. x a) < b < (>> 1 d))"},
		// Main reported "Expected a type" for these two instead of reading
		// the comparison; the nested-list rule now applies to them as well.
		{input: "a<m::b<1>>(d)", want: "(chain a < (:: m b) < (>> 1 d))"},
		{input: "m::a<b<1>>(d)", want: "(chain (:: m a) < b < (>> 1 d))"},
		{input: "a<b<c<1>>>(e)", want: "(chain a < b < c < (>> 1 e))", errs: []string{"no '>>>' operator"}},
	})
}

// Probing `(a<b)` as a grouped function return type rolls back a nested
// type error. The rollback must not reject the call type arguments, so the
// missing '>' is still reported as it was before #639.
func TestCallTypeArgumentRecoveryAfterGroupedReturnProbe(t *testing.T) {
	runSexprCases(t, []sexprCase{
		{input: "f<fn()(a<b)", want: "(call f<Function> (< a b))", errs: []string{"Expected '>' after type arguments"}},
		{input: "m::f<fn()(a<b)", want: "(:: m (call f<Function> (< a b)))", errs: []string{"Expected '>' after type arguments"}},
		{input: "f<fn()(Box<Int>)>(x)", want: "(call f<Function> x)"},
	})
}
