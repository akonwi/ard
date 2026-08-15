package gotarget

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"
)

func TestSourceRendererEdgeCases(t *testing.T) {
	ident := func(name string) *ast.Ident { return ast.NewIdent(name) }
	emptyParams := func() *ast.FieldList { return &ast.FieldList{} }
	functionType := func() *ast.FuncType { return &ast.FuncType{Params: emptyParams()} }
	receiveChannel := func() *ast.ChanType {
		return &ast.ChanType{Dir: ast.RECV, Value: ident("int")}
	}

	file := &ast.File{Name: ident("edge"), Decls: []ast.Decl{
		&ast.GenDecl{Tok: token.VAR},
		&ast.GenDecl{Tok: token.TYPE, Specs: []ast.Spec{&ast.TypeSpec{
			Name: ident("Generic"),
			TypeParams: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ident("P")},
				Type:  &ast.StarExpr{X: ident("int")},
			}}},
			Type: &ast.StructType{Fields: &ast.FieldList{}},
		}}},
		&ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ident("nested")},
			Type:  &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: receiveChannel()},
		}}},
		&ast.FuncDecl{
			Name: ident("convert"),
			Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ident("f")}, Type: functionType()},
				{Names: []*ast.Ident{ident("ch")}, Type: &ast.ChanType{Dir: ast.SEND | ast.RECV, Value: ident("int")}},
			}}},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					&ast.CallExpr{Fun: functionType(), Args: []ast.Expr{ident("f")}},
				}},
				&ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{
					&ast.CallExpr{Fun: receiveChannel(), Args: []ast.Expr{ident("ch")}},
				}},
			}},
		},
	}}

	source, err := renderFile(file)
	if err != nil {
		t.Fatalf("renderFile error = %v", err)
	}
	got := string(source)
	for _, want := range []string{
		"var (\n)",
		"type Generic[P *int,] struct{}",
		"chan (<-chan int)",
		"(func())(f)",
		"(<-chan int)(ch)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered source missing %q:\n%s", want, got)
		}
	}

	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "edge.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("rendered source does not parse: %v\n%s", err, got)
	}
	if _, err := (&types.Config{}).Check("edge", fileSet, []*ast.File{parsed}, nil); err != nil {
		t.Fatalf("rendered source does not type-check: %v\n%s", err, got)
	}
}

func TestSourceRendererPreservesBinaryGrouping(t *testing.T) {
	file := &ast.File{Name: ast.NewIdent("edge"), Decls: []ast.Decl{&ast.FuncDecl{
		Name: ast.NewIdent("subtract"),
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ast.NewIdent("a"), ast.NewIdent("b"), ast.NewIdent("c")},
				Type:  ast.NewIdent("int"),
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: ast.NewIdent("int")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.BinaryExpr{
			X:  ast.NewIdent("a"),
			Op: token.SUB,
			Y:  &ast.BinaryExpr{X: ast.NewIdent("b"), Op: token.SUB, Y: ast.NewIdent("c")},
		}}}}},
	}}}

	source, err := renderFile(file)
	if err != nil {
		t.Fatalf("renderFile error = %v", err)
	}
	if !strings.Contains(string(source), "return a - (b - c)") {
		t.Fatalf("renderer changed right-associative grouping:\n%s", source)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "edge.go", source, parser.SkipObjectResolution); err != nil {
		t.Fatalf("rendered source does not parse: %v\n%s", err, source)
	}
}
