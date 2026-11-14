package parse

import "testing"

// Bitwise operators follow Go's precedence ladder (ADR 0080): `<<`, `>>` and
// `&` share a level with `*`, `/` and `%`; `|` and `^` share a level with `+`
// and `-`; all of them bind tighter than comparisons.
func TestBitwiseOperators(t *testing.T) {
	id := func(name string) *Identifier { return &Identifier{Name: name} }
	num := func(v string) *NumLiteral { return &NumLiteral{Value: v} }
	bin := func(op Operator, l, r Expression) *BinaryExpression {
		return &BinaryExpression{Operator: op, Left: l, Right: r}
	}
	un := func(op Operator, operand Expression) *UnaryExpression {
		return &UnaryExpression{Operator: op, Operand: operand}
	}
	program := func(s Statement) Program {
		return Program{Imports: []Import{}, Statements: []Statement{s}}
	}

	runTests(t, []test{
		{name: "and", input: `a & b`, output: program(bin(BitAnd, id("a"), id("b")))},
		{name: "or", input: `a | b`, output: program(bin(BitOr, id("a"), id("b")))},
		{name: "xor", input: `a ^ b`, output: program(bin(BitXor, id("a"), id("b")))},
		{name: "not", input: `~a`, output: program(un(BitNot, id("a")))},
		{name: "shift left", input: `a << 2`, output: program(bin(ShiftLeft, id("a"), num("2")))},
		{name: "shift right", input: `a >> 2`, output: program(bin(ShiftRight, id("a"), num("2")))},
		{name: "shift left without spaces", input: `a<<b`, output: program(bin(ShiftLeft, id("a"), id("b")))},
		{name: "shift right without spaces", input: `a>>b`, output: program(bin(ShiftRight, id("a"), id("b")))},
		{
			name:   "and binds tighter than equality",
			input:  `mods & flag != 0`,
			output: program(bin(NotEqual, bin(BitAnd, id("mods"), id("flag")), num("0"))),
		},
		{
			name:   "and binds tighter than == on the right",
			input:  `a & b == 0`,
			output: program(bin(Equal, bin(BitAnd, id("a"), id("b")), num("0"))),
		},
		{
			name:   "or binds tighter than comparison",
			input:  `a | b > c`,
			output: program(bin(GreaterThan, bin(BitOr, id("a"), id("b")), id("c"))),
		},
		{
			name:   "shift binds tighter than comparison",
			input:  `a >> 1 < b`,
			output: program(bin(LessThan, bin(ShiftRight, id("a"), num("1")), id("b"))),
		},
		{
			name:   "comparison against a shift",
			input:  `a > b >> 1`,
			output: program(bin(GreaterThan, id("a"), bin(ShiftRight, id("b"), num("1")))),
		},
		{
			name:   "and binds tighter than or",
			input:  `a | b & c`,
			output: program(bin(BitOr, id("a"), bin(BitAnd, id("b"), id("c")))),
		},
		{
			name:   "or and xor share a level, left to right",
			input:  `a | b ^ c`,
			output: program(bin(BitXor, bin(BitOr, id("a"), id("b")), id("c"))),
		},
		{
			name:   "and shares a level with addition's tighter neighbour",
			input:  `a & b + c`,
			output: program(bin(Plus, bin(BitAnd, id("a"), id("b")), id("c"))),
		},
		{
			name:   "shift binds tighter than addition",
			input:  `a + b << 1`,
			output: program(bin(Plus, id("a"), bin(ShiftLeft, id("b"), num("1")))),
		},
		{
			name:   "shift then subtraction",
			input:  `1 << n - 1`,
			output: program(bin(Minus, bin(ShiftLeft, num("1"), id("n")), num("1"))),
		},
		{
			name:   "shift and multiply are left associative",
			input:  `a * b << c`,
			output: program(bin(ShiftLeft, bin(Multiply, id("a"), id("b")), id("c"))),
		},
		{
			name:   "and then shift are left associative",
			input:  `a & b << c`,
			output: program(bin(ShiftLeft, bin(BitAnd, id("a"), id("b")), id("c"))),
		},
		{
			name:   "or and addition share a level",
			input:  `a + b | c`,
			output: program(bin(BitOr, bin(Plus, id("a"), id("b")), id("c"))),
		},
		{
			name:   "not binds tighter than and",
			input:  `a & ~b`,
			output: program(bin(BitAnd, id("a"), un(BitNot, id("b")))),
		},
		{
			name:   "not of a member",
			input:  `~vaxis::ModShift`,
			output: program(un(BitNot, &StaticProperty{Target: id("vaxis"), Property: id("ModShift")})),
		},
		{
			name:   "negation as a right operand",
			input:  `p & -p`,
			output: program(bin(BitAnd, id("p"), un(Minus, id("p")))),
		},
		{
			name:   "address-of as a right operand",
			input:  `a & &b`,
			output: program(bin(BitAnd, id("a"), &MutRef{Operand: id("b"), Ampersand: true, ReadOnly: true})),
		},
		{
			name:   "packing bytes",
			input:  `(hi << 8) | lo`,
			output: program(bin(BitOr, bin(ShiftLeft, id("hi"), num("8")), id("lo"))),
		},
		{
			name:  "shifts inside a range",
			input: `0..1 << n`,
			output: program(&RangeExpression{
				Start: num("0"),
				End:   bin(ShiftLeft, num("1"), id("n")),
			}),
		},
	})
}

func TestBitwiseOperatorErrors(t *testing.T) {
	at := func(row, col, endCol int) Location {
		return Location{Start: Point{Row: row, Col: col}, End: Point{Row: row, Col: endCol}}
	}
	tests := []struct {
		name    string
		input   string
		wantErr ParseError
	}{
		{
			name:    "&& after an expression",
			input:   "let a = true && false\n",
			wantErr: ParseError{Location: at(1, 14, 15), Message: "Unexpected '&&' after expression; use 'and' for logical and"},
		},
		{
			name:    "|| after an expression",
			input:   "let a = true || false\n",
			wantErr: ParseError{Location: at(1, 14, 15), Message: "Unexpected '||' after expression; use 'or' for logical or"},
		},
		{
			name:    "&mut after an expression",
			input:   "mut b = 1\nlet a = 5 &mut b\n",
			wantErr: ParseError{Location: at(2, 11, 14), Message: "Unexpected '&mut' after expression"},
		},
		{
			name:    "Go's prefix ^ for bitwise not",
			input:   "let a = ^b\n",
			wantErr: ParseError{Location: at(1, 9, 9), Message: "Unexpected '^'; use '~' for bitwise not"},
		},
		{
			name:    "Go's &^ and-not",
			input:   "let a = b &^ c\n",
			wantErr: ParseError{Location: at(1, 12, 12), Message: "Unexpected '^'; use '~' for bitwise not"},
		},
		{
			name:    "prefix !",
			input:   "let a = !b\n",
			wantErr: ParseError{Location: at(1, 9, 9), Message: "Unexpected '!'; use 'not' for logical not or '~' for bitwise not"},
		},
		{
			name:    "a stray | where an expression is expected",
			input:   "let a = | b\n",
			wantErr: ParseError{Location: at(1, 9, 9), Message: "Unexpected '|'; expected an expression"},
		},
		{
			name:    "spaced < < is not a shift",
			input:   "let a = b < < c\n",
			wantErr: ParseError{Location: at(1, 13, 12), Message: "Unexpected token: less_than"},
		},
		{
			name:    "Java's >>> logical shift",
			input:   "let a = b >>> c\n",
			wantErr: ParseError{Location: at(1, 11, 13), Message: "Ard has no '>>>' operator; shift an unsigned value for a logical shift"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Parse([]byte(tt.input), "test.ard")
			if len(result.Errors) == 0 || result.Errors[0] != tt.wantErr {
				t.Fatalf("errors = %v, want first %v", result.Errors, tt.wantErr)
			}
		})
	}
}

// A new line still ends an expression, so `&` starting the next line is the
// address-of prefix of a new statement, as with `-`.
func TestBitwiseOperatorsDoNotContinueAcrossLines(t *testing.T) {
	parseOK(t, "mut b = 1\nlet a = 5\n&mut b\n")
	parseOK(t, "let b = 1\nlet a = 5\n&b\n")
}

// Generic call type arguments never start with `<`, so an identifier
// followed by `<<` is a shift rather than a failed type argument list.
func TestShiftAfterIdentifierIsNotAGenericCall(t *testing.T) {
	result := parseOK(t, "let a = f<<g(1)\n")
	decl := result.Statements[0].(*VariableDeclaration)
	if bin, ok := decl.Value.(*BinaryExpression); !ok || bin.Operator != ShiftLeft {
		t.Fatalf("value = %#v, want a left shift", decl.Value)
	}
	parseOK(t, "let m: [Str: List<List<Int>>] = x\n")
	parseOK(t, "let a = decode<List<Int>>(x)\n")
	parseOK(t, "let a = pkg::decode<List<Int>>(x)\n")
}
