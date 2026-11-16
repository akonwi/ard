package gotarget

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/token"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
)

func renderOperandSequence(t *testing.T, stmts []ast.Stmt, ops []operand) string {
	t.Helper()
	var out strings.Builder
	for _, stmt := range stmts {
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), stmt); err != nil {
			t.Fatalf("render stmt: %v", err)
		}
		out.WriteString(buf.String() + "\n")
	}
	for _, op := range ops {
		var buf bytes.Buffer
		if err := format.Node(&buf, token.NewFileSet(), op.expr); err != nil {
			t.Fatalf("render expr: %v", err)
		}
		out.WriteString("use " + buf.String() + "\n")
	}
	return out.String()
}

func setupStmt(l *lowerer, name string) []ast.Stmt {
	return []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{Fun: l.ident(name)}}}
}

func operandCall(l *lowerer, name string) ast.Expr {
	return &ast.CallExpr{Fun: l.ident(name)}
}

func TestSequenceOperands(t *testing.T) {
	tests := []struct {
		name string
		ops  func(l *lowerer) []operand
		want string
	}{
		{
			name: "no setup leaves every operand inline",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "a")}, {expr: operandCall(l, "b")}}
			},
			want: "use a()\nuse b()\n",
		},
		{
			name: "setup on the first operand only adds nothing",
			ops: func(l *lowerer) []operand {
				return []operand{{stmts: setupStmt(l, "setupA"), expr: operandCall(l, "a")}, {expr: operandCall(l, "b")}}
			},
			want: "setupA()\nuse a()\nuse b()\n",
		},
		{
			name: "earlier operands are evaluated before later setup",
			ops: func(l *lowerer) []operand {
				return []operand{
					{stmts: setupStmt(l, "setupA"), expr: operandCall(l, "a")},
					{expr: l.ident("local")},
					{stmts: setupStmt(l, "setupC"), expr: operandCall(l, "c")},
					{expr: operandCall(l, "d")},
				}
			},
			want: "setupA()\n_tmp_0 := a()\n_tmp_1 := local\nsetupC()\nuse _tmp_0\nuse _tmp_1\nuse c()\nuse d()\n",
		},
		{
			name: "inert operands stay inline",
			ops: func(l *lowerer) []operand {
				return []operand{
					{expr: &ast.BasicLit{Kind: token.INT, Value: "1"}},
					{expr: &ast.UnaryExpr{Op: token.SUB, X: &ast.BasicLit{Kind: token.FLOAT, Value: "1.5"}}},
					{expr: l.ident("nil")},
					{expr: &ast.FuncLit{Type: &ast.FuncType{Params: &ast.FieldList{}}, Body: &ast.BlockStmt{}}},
					{stmts: setupStmt(l, "setup"), expr: operandCall(l, "last")},
				}
			},
			want: "setup()\nuse 1\nuse -1.5\nuse nil\nuse func() {\n}\nuse last()\n",
		},
		{
			name: "places are fixed by address",
			ops: func(l *lowerer) []operand {
				return []operand{
					{expr: l.ident("variable"), place: true},
					{expr: &ast.StarExpr{X: operandCall(l, "pointer")}, place: true},
					{expr: &ast.SelectorExpr{X: operandCall(l, "holder"), Sel: l.ident("Field")}, place: true},
					{stmts: setupStmt(l, "setup"), expr: operandCall(l, "value")},
				}
			},
			want: "_tmp_0 := pointer()\n_tmp_1 := &holder().Field\nsetup()\nuse variable\nuse *_tmp_0\nuse *_tmp_1\nuse value()\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &lowerer{}
			ops := tt.ops(l)
			for i := range ops {
				ops[i].typ = air.NoType
			}
			stmts, err := l.sequenceOperands(ops)
			if err != nil {
				t.Fatalf("sequenceOperands: %v", err)
			}
			if got := renderOperandSequence(t, stmts, ops); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestSequenceOperandsInOrder(t *testing.T) {
	tests := []struct {
		name  string
		ops   func(l *lowerer) []operand
		order []int
		want  string
	}{
		{
			name: "no order sequences the operands as they are",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "a")}, {expr: operandCall(l, "b")}}
			},
			want: "use a()\nuse b()\n",
		},
		{
			name: "a call evaluated first but used later is evaluated first",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "a")}, {expr: operandCall(l, "b")}}
			},
			order: []int{1, 0},
			want:  "_tmp_0 := b()\nuse a()\nuse _tmp_0\n",
		},
		{
			name: "a constant out of order stays inline",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "a")}, {expr: &ast.BasicLit{Kind: token.INT, Value: "1"}}}
			},
			order: []int{1, 0},
			want:  "use a()\nuse 1\n",
		},
		{
			name: "a read used before a call evaluated before it waits for the call",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: l.ident("local")}, {expr: operandCall(l, "b")}}
			},
			order: []int{1, 0},
			want:  "_tmp_0 := b()\nuse local\nuse _tmp_0\n",
		},
		{
			name: "operands evaluated before a moved operand are evaluated first",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "callee")}, {expr: operandCall(l, "a")}, {expr: operandCall(l, "b")}}
			},
			order: []int{0, 2, 1},
			want:  "_tmp_0 := callee()\n_tmp_1 := b()\nuse _tmp_0\nuse a()\nuse _tmp_1\n",
		},
		{
			name: "setup of an operand evaluated first runs first",
			ops: func(l *lowerer) []operand {
				return []operand{{expr: operandCall(l, "a")}, {stmts: setupStmt(l, "setupB"), expr: operandCall(l, "b")}}
			},
			order: []int{1, 0},
			want:  "setupB()\n_tmp_0 := b()\nuse a()\nuse _tmp_0\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := &lowerer{}
			ops := tt.ops(l)
			for i := range ops {
				ops[i].typ = air.NoType
			}
			stmts, err := l.sequenceOperandsInOrder(ops, tt.order)
			if err != nil {
				t.Fatalf("sequenceOperandsInOrder: %v", err)
			}
			if got := renderOperandSequence(t, stmts, ops); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
