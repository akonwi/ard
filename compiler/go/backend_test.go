package gotarget

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/version"
)

func TestZeroValueForForeignNumericTypeUsesUnderlyingZero(t *testing.T) {
	program := &air.Program{Types: []air.TypeInfo{
		{ID: 1, Kind: air.TypeInt},
		{ID: 2, Kind: air.TypeForeignType, Name: "time::Duration", Value: 1, ForeignTarget: "go", ForeignNamespace: "time", ForeignQualifier: "time", ForeignSymbol: "Duration"},
	}}
	l := &lowerer{program: program}
	zero, err := l.zeroValueExpr(2)
	if err != nil {
		t.Fatalf("zeroValueExpr error = %v", err)
	}
	lit, ok := zero.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT || lit.Value != "0" {
		t.Fatalf("foreign numeric zero = %#v, want integer literal 0", zero)
	}
}

func TestTypesForModuleKeepsOwnedTypesWithOwningModule(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "a.ard", Types: []air.TypeID{1}},
			{ID: 1, Path: "b.ard", Types: []air.TypeID{2}},
		},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeStruct, Name: "A"},
			{ID: 2, Kind: air.TypeStruct, Name: "B"},
			{ID: 3, Kind: air.TypeTraitObject, Name: "OwnedTraitObject", Trait: 0},
			{ID: 4, Kind: air.TypeTraitObject, Name: "Synthetic", Trait: 99},
		},
		Traits: []air.Trait{{ID: 0, Name: "Owned", ModulePath: "a.ard"}},
	}
	l := &lowerer{program: program}
	moduleA := l.typesForModule(0, 1)
	if got := typeNames(moduleA); strings.Join(got, ",") != "A,OwnedTraitObject" {
		t.Fatalf("module A types = %v, want A,OwnedTraitObject", got)
	}
	moduleB := l.typesForModule(1, 1)
	if got := typeNames(moduleB); strings.Join(got, ",") != "B,Synthetic" {
		t.Fatalf("module B types = %v, want B,Synthetic", got)
	}
}

func typeNames(types []air.TypeInfo) []string {
	out := make([]string, len(types))
	for i, typ := range types {
		out[i] = typ.Name
	}
	return out
}
func TestTraitInterfaceTypeNameUsesNaturalVisibility(t *testing.T) {
	l := &lowerer{program: &air.Program{Traits: []air.Trait{
		{ID: 0, Name: "Renderable", ModulePath: "view.ard"},
		{ID: 1, Name: "internal_drawable", ModulePath: "view.ard", Private: true},
		{ID: 2, Name: "ToString", ModulePath: "ard/string"},
	}}}
	if got := l.traitInterfaceTypeName(l.program.Traits[0]); got != "Renderable" {
		t.Fatalf("public trait interface name = %q, want Renderable", got)
	}
	if got := l.traitInterfaceTypeName(l.program.Traits[1]); got != "internalDrawable" {
		t.Fatalf("private trait interface name = %q, want internalDrawable", got)
	}
	if got := l.traitInterfaceTypeName(l.program.Traits[2]); got != "ToString" {
		t.Fatalf("stdlib trait interface name = %q, want ToString", got)
	}
}
func TestTraitInterfaceTypeNameFallsBackOnCrossModuleTraitCollision(t *testing.T) {
	l := &lowerer{program: &air.Program{Traits: []air.Trait{
		{ID: 0, Name: "Drawable", ModulePath: "ui/drawable.ard"},
		{ID: 1, Name: "Drawable", ModulePath: "svg/drawable.ard"},
	}}}
	if got := l.traitInterfaceTypeName(l.program.Traits[0]); got != "ardTrait_Drawable_0" {
		t.Fatalf("first colliding trait interface name = %q, want legacy fallback", got)
	}
	if got := l.traitInterfaceTypeName(l.program.Traits[1]); got != "ardTrait_Drawable_1" {
		t.Fatalf("second colliding trait interface name = %q, want legacy fallback", got)
	}
}
func TestTraitInterfaceTypeNameFallsBackOnTopLevelCollision(t *testing.T) {
	l := &lowerer{program: &air.Program{
		Traits:    []air.Trait{{ID: 0, Name: "Drawable", ModulePath: "traits.ard"}, {ID: 1, Name: "Encodable", ModulePath: "encoding.ard"}},
		Types:     []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "Drawable", ModulePath: "types.ard"}},
		Functions: []air.Function{{ID: 0, Module: 0, Name: "encodable"}},
		Globals:   []air.Global{{ID: 0, Module: 0, Name: "configured"}},
	}}
	if got := l.traitInterfaceTypeName(l.program.Traits[0]); got != "ardTrait_Drawable_0" {
		t.Fatalf("trait colliding with type = %q, want legacy fallback", got)
	}
	if got := l.traitInterfaceTypeName(l.program.Traits[1]); got != "Encodable" {
		t.Fatalf("trait name should take precedence over function collisions = %q, want Encodable", got)
	}
}
func TestTraitInterfaceTypeExprQualifiesCrossModuleInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "traits.ard"},
			{ID: 1, Path: "consumer.ard"},
		},
		Traits: []air.Trait{{ID: 0, Name: "Drawable", ModulePath: "traits.ard"}},
		Types:  []air.TypeInfo{{ID: 1, Kind: air.TypeTraitObject, Name: "Drawable", Trait: 0}},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	typ, err := l.goType(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(typ); got != "traits.Drawable" {
		t.Fatalf("cross-module trait type = %q, want traits.Drawable", got)
	}
	if got := l.currentImports["traits"]; got != "generated/traits" {
		t.Fatalf("registered import = %q, want generated/traits", got)
	}
}
func TestTraitInterfaceTypeExprKeepsSameModuleUnqualifiedInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{{ID: 0, Path: "traits.ard"}},
		Traits:  []air.Trait{{ID: 0, Name: "Drawable", ModulePath: "traits.ard"}},
		Types:   []air.TypeInfo{{ID: 1, Kind: air.TypeTraitObject, Name: "Drawable", Trait: 0}},
	}
	l := &lowerer{program: program, currentModule: 0, currentImports: map[string]string{}, useModulePackages: true}
	typ, err := l.goType(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(typ); got != "Drawable" {
		t.Fatalf("same-module trait type = %q, want Drawable", got)
	}
	if len(l.currentImports) != 0 {
		t.Fatalf("same-module trait type registered imports: %#v", l.currentImports)
	}
}
func TestNamedTypeExprQualifiesCrossModuleInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "models/user.ard"},
			{ID: 1, Path: "consumer.ard"},
		},
		Types: []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "models/user.ard"}},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	typ, err := l.goType(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(typ); got != "user.User" {
		t.Fatalf("cross-module named type = %q, want user.User", got)
	}
	if got := l.currentImports["user"]; got != "generated/models/user" {
		t.Fatalf("registered import = %q, want generated/models/user", got)
	}
}
func TestNamedTypeExprKeepsSameModuleUnqualifiedInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{{ID: 0, Path: "models/user.ard"}},
		Types:   []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "models/user.ard"}},
	}
	l := &lowerer{program: program, currentModule: 0, currentImports: map[string]string{}, useModulePackages: true}
	typ, err := l.goType(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(typ); got != "User" {
		t.Fatalf("same-module named type = %q, want User", got)
	}
	if len(l.currentImports) != 0 {
		t.Fatalf("same-module named type registered imports: %#v", l.currentImports)
	}
}
func TestNamedTypeExprKeepsSinglePackageModeUnqualified(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "models/user.ard"},
			{ID: 1, Path: "consumer.ard"},
		},
		Types: []air.TypeInfo{{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "models/user.ard"}},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}}
	typ, err := l.goType(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(typ); got != "User" {
		t.Fatalf("single-package named type = %q, want User", got)
	}
	if len(l.currentImports) != 0 {
		t.Fatalf("single-package named type registered imports: %#v", l.currentImports)
	}
}
func TestPrivateUnionLowersToUnexportedNaturalTypeName(t *testing.T) {
	program := lowerSource(t, `
		private type internal_value = Int | Str

		private fn make_value() internal_value {
			1
		}
	`)
	for _, typ := range program.Types {
		if typ.Kind != air.TypeUnion || typ.Name != "internal_value" {
			continue
		}
		if !typ.Private {
			t.Fatal("private union did not preserve privacy in AIR")
		}
		if got := typeName(program, typ); got != "internalValue" {
			t.Fatalf("private union type name = %q, want internalValue", got)
		}
		return
	}
	t.Fatal("lowered program missing private union type")
}
func TestEnumVariantExprQualifiesCrossModuleInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "models/direction.ard", Types: []air.TypeID{1}},
			{ID: 1, Path: "consumer.ard"},
		},
		Types: []air.TypeInfo{{ID: 1, Kind: air.TypeEnum, Name: "Direction", ModulePath: "models/direction.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}}},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	if got := astExprName(l.enumVariantExpr(program.Types[0], program.Types[0].Variants[0])); got != "direction.DirectionDown" {
		t.Fatalf("cross-module enum variant expr = %q, want direction.DirectionDown", got)
	}
	if got := l.currentImports["direction"]; got != "generated/models/direction" {
		t.Fatalf("registered import = %q, want generated/models/direction", got)
	}
}
func TestLowerExprQualifiesCrossModuleCompositeLiteralsAndEnumCastsInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "models/user.ard", Types: []air.TypeID{1, 2}},
			{ID: 1, Path: "consumer.ard"},
		},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeStruct, Name: "User", ModulePath: "models/user.ard"},
			{ID: 2, Kind: air.TypeEnum, Name: "Direction", ModulePath: "models/user.ard", Variants: []air.VariantInfo{{Name: "Down", Discriminant: 0}}},
			{ID: 3, Kind: air.TypeInt, Name: "Int"},
		},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	makeStruct, err := l.lowerExpr(air.Function{Module: 1}, air.Expr{Kind: air.ExprMakeStruct, Type: 1})
	if err != nil {
		t.Fatal(err)
	}
	lit, ok := makeStruct.expr.(*ast.CompositeLit)
	if !ok {
		t.Fatalf("make struct lowered to %T, want *ast.CompositeLit", makeStruct.expr)
	}
	if got := astExprName(lit.Type); got != "user.User" {
		t.Fatalf("cross-module composite literal type = %q, want user.User", got)
	}
	enumVariant, err := l.lowerExpr(air.Function{Module: 1}, air.Expr{Kind: air.ExprEnumVariant, Type: 2, Variant: 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := astExprName(enumVariant.expr); got != "user.DirectionDown" {
		t.Fatalf("cross-module enum variant = %q, want user.DirectionDown", got)
	}
	left := loweredExpr{expr: ast.NewIdent("value")}
	right := loweredExpr{expr: &ast.BasicLit{Kind: token.INT, Value: "1"}}
	l.castEnumIntComparisonOperands(&left, 2, &right, 3)
	call, ok := right.expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("enum/int comparison cast lowered to %T, want *ast.CallExpr", right.expr)
	}
	if got := astExprName(call.Fun); got != "user.Direction" {
		t.Fatalf("cross-module enum/int cast type = %q, want user.Direction", got)
	}
	if got := l.currentImports["user"]; got != "generated/models/user" {
		t.Fatalf("registered import = %q, want generated/models/user", got)
	}
}
func TestLowerExprQualifiesCrossModuleUnionWrapAndMatchInModulePackageMode(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "models/value.ard", Types: []air.TypeID{3}},
			{ID: 1, Path: "consumer.ard"},
		},
		Types: []air.TypeInfo{
			{ID: 1, Kind: air.TypeInt, Name: "Int"},
			{ID: 2, Kind: air.TypeStr, Name: "Str"},
			{ID: 3, Kind: air.TypeUnion, Name: "Value", ModulePath: "models/value.ard", Members: []air.UnionMember{{Type: 1, Tag: 0, Name: "Int"}, {Type: 2, Tag: 1, Name: "Str"}}},
		},
	}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true, declaredLocals: map[air.LocalID]bool{}}
	wrap, err := l.lowerExpr(air.Function{Module: 1}, air.Expr{Kind: air.ExprUnionWrap, Type: 3, Tag: 0, Target: &air.Expr{Kind: air.ExprConstInt, Type: 1, Int: "7"}})
	if err != nil {
		t.Fatal(err)
	}
	lit, ok := wrap.expr.(*ast.CompositeLit)
	if !ok {
		t.Fatalf("union wrap lowered to %T, want *ast.CompositeLit", wrap.expr)
	}
	if got := astExprName(lit.Type); got != "value.Value" {
		t.Fatalf("cross-module union wrap type = %q, want value.Value", got)
	}
	if !compositeLitHasKey(lit, "ArdTag") || !compositeLitHasKey(lit, "Int") {
		t.Fatalf("union wrap literal missing exported keys ArdTag/Int: %#v", lit.Elts)
	}

	fn := air.Function{Module: 1, Locals: []air.Local{{ID: 0, Name: "input", Type: 3}, {ID: 1, Name: "value", Type: 1}}}
	match, err := l.lowerExpr(fn, air.Expr{
		Kind:   air.ExprMatchUnion,
		Type:   1,
		Target: &air.Expr{Kind: air.ExprLoadLocal, Type: 3, Local: 0},
		UnionCases: []air.UnionMatchCase{{
			Tag:   0,
			Local: 1,
			Body:  air.Block{Result: &air.Expr{Kind: air.ExprLoadLocal, Type: 1, Local: 1}},
		}},
		CatchAll: air.Block{Result: &air.Expr{Kind: air.ExprConstInt, Type: 1, Int: "0"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !stmtsHaveSelector(match.stmts, "ArdTag") || !stmtsHaveSelector(match.stmts, "Int") {
		t.Fatalf("union match did not use exported selectors ArdTag/Int: %#v", match.stmts)
	}
	if got := l.currentImports["value"]; got != "generated/models/value" {
		t.Fatalf("registered import = %q, want generated/models/value", got)
	}
}

func TestFunctionExprQualifiesCrossModuleInModulePackageMode(t *testing.T) {
	program := &air.Program{Modules: []air.Module{
		{ID: 0, Path: "service.ard"},
		{ID: 1, Path: "consumer.ard"},
	}}
	fn := air.Function{ID: 0, Module: 0, Name: "make_user"}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	if got := astExprName(l.functionExpr(fn)); got != "service.MakeUser" {
		t.Fatalf("cross-module function expr = %q, want service.MakeUser", got)
	}
	if got := l.currentImports["service"]; got != "generated/service" {
		t.Fatalf("registered import = %q, want generated/service", got)
	}
}
func TestFunctionExprKeepsSinglePackageModeUnqualified(t *testing.T) {
	program := &air.Program{Modules: []air.Module{
		{ID: 0, Path: "service.ard"},
		{ID: 1, Path: "consumer.ard"},
	}}
	fn := air.Function{ID: 0, Module: 0, Name: "make_user"}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}}
	if got := astExprName(l.functionExpr(fn)); got != "MakeUser" {
		t.Fatalf("single-package function expr = %q, want MakeUser", got)
	}
	if len(l.currentImports) != 0 {
		t.Fatalf("single-package function expr registered imports: %#v", l.currentImports)
	}
}
func TestGlobalExprQualifiesCrossModuleInModulePackageMode(t *testing.T) {
	program := &air.Program{Modules: []air.Module{
		{ID: 0, Path: "config.ard"},
		{ID: 1, Path: "consumer.ard"},
	}}
	global := air.Global{ID: 0, Module: 0, Name: "default_name"}
	l := &lowerer{program: program, currentModule: 1, currentImports: map[string]string{}, useModulePackages: true}
	if got := astExprName(l.globalExpr(global)); got != "config.DefaultName" {
		t.Fatalf("cross-module global expr = %q, want config.DefaultName", got)
	}
	if got := l.currentImports["config"]; got != "generated/config" {
		t.Fatalf("registered import = %q, want generated/config", got)
	}
}
func TestGlobalExprKeepsSameModuleUnqualifiedInModulePackageMode(t *testing.T) {
	program := &air.Program{Modules: []air.Module{{ID: 0, Path: "config.ard"}}}
	global := air.Global{ID: 0, Module: 0, Name: "default_name"}
	l := &lowerer{program: program, currentModule: 0, currentImports: map[string]string{}, useModulePackages: true}
	if got := astExprName(l.globalExpr(global)); got != "DefaultName" {
		t.Fatalf("same-module global expr = %q, want DefaultName", got)
	}
	if len(l.currentImports) != 0 {
		t.Fatalf("same-module global expr registered imports: %#v", l.currentImports)
	}
}

func lowerProgramAST(t testing.TB, program *air.Program, options Options) map[string]*ast.File {
	t.Helper()
	files, err := lowerProgram(program, options)
	if err != nil {
		t.Fatalf("lower program: %v", err)
	}
	return files
}

func astFilesHaveImport(files map[string]*ast.File, alias string, importPath string) bool {
	for _, file := range files {
		if astFileHasImport(file, alias, importPath) {
			return true
		}
	}
	return false
}

func astFileHasImport(file *ast.File, alias string, importPath string) bool {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.IMPORT {
			continue
		}
		for _, specNode := range gen.Specs {
			spec, ok := specNode.(*ast.ImportSpec)
			if !ok || spec.Path == nil || strings.Trim(spec.Path.Value, "\"") != importPath {
				continue
			}
			actualAlias := ""
			if spec.Name != nil {
				actualAlias = spec.Name.Name
			}
			if actualAlias == alias {
				return true
			}
		}
	}
	return false
}

func astFilesContain(files map[string]*ast.File, match func(ast.Node) bool) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if match(node) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func astFilesHaveSelector(files map[string]*ast.File, qualifier string, selectorName string) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || selector.Sel == nil || selector.Sel.Name != selectorName {
				return true
			}
			ident, ok := selector.X.(*ast.Ident)
			if ok && ident.Name == qualifier {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func astFilesHaveCall(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return false
		}
		return astCallName(call) == name
	})
}

func astFilesHaveFuncWithPrefix(files map[string]*ast.File, prefix string) bool {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && strings.HasPrefix(fn.Name.Name, prefix) {
				return true
			}
		}
	}
	return false
}

func astFilesHaveFuncContaining(files map[string]*ast.File, part string) bool {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && strings.Contains(fn.Name.Name, part) {
				return true
			}
		}
	}
	return false
}

func astFilesHaveTypeSpec(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		typ, ok := node.(*ast.TypeSpec)
		return ok && typ.Name != nil && typ.Name.Name == name
	})
}

func astFilesHaveTypeSwitchCase(files map[string]*ast.File, typeName string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		clause, ok := node.(*ast.CaseClause)
		if !ok {
			return false
		}
		for _, expr := range clause.List {
			if astExprName(expr) == typeName {
				return true
			}
		}
		return false
	})
}

func astFilesHaveValueSpec(files map[string]*ast.File, name string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return false
		}
		for _, ident := range value.Names {
			if ident.Name == name {
				return true
			}
		}
		return false
	})
}

func astCallName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if ident, ok := fun.X.(*ast.Ident); ok {
			return ident.Name + "." + fun.Sel.Name
		}
		return fun.Sel.Name
	case *ast.IndexExpr:
		return astExprName(fun.X)
	case *ast.IndexListExpr:
		return astExprName(fun.X)
	}
	return ""
}

func compositeLitHasKey(lit *ast.CompositeLit, key string) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if ident, ok := kv.Key.(*ast.Ident); ok && ident.Name == key {
			return true
		}
	}
	return false
}

func stmtsHaveSelector(stmts []ast.Stmt, selectorName string) bool {
	for _, stmt := range stmts {
		found := false
		ast.Inspect(stmt, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && selector.Sel.Name == selectorName {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func astExprName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.IndexExpr:
		return astExprName(e.X)
	case *ast.IndexListExpr:
		return astExprName(e.X)
	case *ast.StarExpr:
		return "*" + astExprName(e.X)
	case *ast.ArrayType:
		return "[]" + astExprName(e.Elt)
	}
	return ""
}

func astFilesFunc(files map[string]*ast.File, name string) (*ast.FuncDecl, bool) {
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Name != nil && fn.Name.Name == name {
				return fn, true
			}
		}
	}
	return nil, false
}

func astFuncHasBlankAssignString(fn *ast.FuncDecl, value string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || ident.Name != "_" || i >= len(assign.Rhs) {
				continue
			}
			lit, ok := assign.Rhs[i].(*ast.BasicLit)
			if ok && lit.Kind == token.STRING && lit.Value == value {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func astFuncHasReturnString(fn *ast.FuncDecl, value string) bool {
	if fn == nil || fn.Body == nil {
		return false
	}
	found := false
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			lit, ok := result.(*ast.BasicLit)
			if ok && lit.Kind == token.STRING && lit.Value == value {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func astFilesHaveEmptyStructType(files map[string]*ast.File) bool {
	for _, file := range files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			structType, ok := node.(*ast.StructType)
			if ok && (structType.Fields == nil || len(structType.Fields.List) == 0) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}
func TestLowerProgramTakesAddressOfLocalMutTraitArgs(t *testing.T) {
	program := lowerSource(t, `
		struct Counter { value: Int }

		impl Counter {
			fn mut bump() { self.value = self.value + 1 }
		}

		trait Bumpable {
			fn poke(mut c: Counter)
		}

		struct Doubler {}

		impl Bumpable for Doubler {
			fn poke(mut c: Counter) {
				c.bump()
				c.bump()
			}
		}

		fn main() {
			mut c = Counter{value: 0}
			let d: Bumpable = Doubler{}
			d.poke(c)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(strings.ToLower(astCallName(call)), "poke") {
			return false
		}
		for _, arg := range call.Args {
			addr, ok := arg.(*ast.UnaryExpr)
			if !ok || addr.Op != token.AND {
				continue
			}
			ident, identOK := addr.X.(*ast.Ident)
			if identOK && ident.Name == "c" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing address-of for local mutable trait dispatch arg")
	}
}
func TestLowerProgramPassesMutTraitArgsByPointer(t *testing.T) {
	program := lowerSource(t, `
		struct Counter { value: Int }

		impl Counter {
			fn mut bump() { self.value = self.value + 1 }
		}

		trait Bumpable {
			fn poke(mut c: Counter)
		}

		struct Doubler {}

		impl Bumpable for Doubler {
			fn poke(mut c: Counter) {
				c.bump()
				c.bump()
			}
		}

		fn invoke(b: Bumpable, mut c: Counter) {
			b.poke(c)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Doubler_Bumpable_poke") || len(call.Args) < 2 {
			return false
		}
		ident, ok := call.Args[1].(*ast.Ident)
		return ok && ident.Name == "c"
	}) {
		t.Fatal("generated AST missing pointer trait dispatch arg")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Doubler_Bumpable_poke") || len(call.Args) < 2 {
			return false
		}
		star, ok := call.Args[1].(*ast.StarExpr)
		if !ok {
			return false
		}
		ident, identOK := star.X.(*ast.Ident)
		return identOK && ident.Name == "c"
	}) {
		t.Fatal("generated AST dereferences mutable trait dispatch arg")
	}
}
func TestLowerProgramDereferencesMutParamForNonMutMethodCall(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			value: Int,
		}

		impl Box {
			fn mut bump() {
				self.value = self.value + 1
			}

			fn peek() Int {
				self.value
			}
		}

		fn process(mut b: Box) Int {
			b.bump()
			b.peek()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Box_bump") || len(call.Args) == 0 {
			return false
		}
		ident, ok := call.Args[0].(*ast.Ident)
		return ok && ident.Name == "b"
	}) {
		t.Fatal("generated AST missing mut method pointer call")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || !strings.Contains(astCallName(call), "Box_peek") || len(call.Args) == 0 {
			return false
		}
		star, ok := call.Args[0].(*ast.StarExpr)
		if !ok {
			return false
		}
		ident, identOK := star.X.(*ast.Ident)
		return identOK && ident.Name == "b"
	}) {
		t.Fatal("generated AST missing deref for non-mut method call on mut param")
	}
}
func TestGenerateSourcesFormatsSimpleProgram(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(1, 2)
		}
	`)

	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("GenerateSources error = %v", err)
	}
	source, ok := sources["test/test.go"]
	if !ok {
		t.Fatalf("generated sources missing test/test.go: %#v", mapsKeys(sources))
	}
	got := string(source)
	if !strings.Contains(got, "package test") {
		t.Fatalf("generated entry module missing package declaration:\n%s", got)
	}
	if !strings.Contains(got, "func Add(a int, b int) int") {
		t.Fatalf("generated source missing lowered add function:\n%s", got)
	}
	if !strings.Contains(got, "return a + b") {
		t.Fatalf("generated source missing arithmetic return:\n%s", got)
	}
	// `main` is a separate synthetic package that calls the entry module's Main.
	mainSource, ok := sources["main.go"]
	if !ok {
		t.Fatalf("generated sources missing synthetic main.go: %#v", mapsKeys(sources))
	}
	mainGot := string(mainSource)
	if !strings.Contains(mainGot, "package main") || !strings.Contains(mainGot, "func main()") {
		t.Fatalf("synthetic main missing package/func main:\n%s", mainGot)
	}
	if !strings.Contains(mainGot, ".Main()") {
		t.Fatalf("synthetic main does not call the entry Main:\n%s", mainGot)
	}
}
func TestLowerProgramOmitsTestsUnlessIncluded(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int { 1 }
		test fn check() Void!Str { Result::ok(()) }
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}

	productionFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if _, ok := astFilesFunc(productionFiles, "Check"); ok {
		t.Fatal("production AST includes test function")
	}

	testFiles := lowerProgramAST(t, program, Options{PackageName: "main", IncludeTests: true, SuppressMain: true})
	if _, ok := astFilesFunc(testFiles, "Check"); !ok {
		t.Fatal("test AST missing test function")
	}
}
func TestLowerProgramDiscardsFinalExprInVoidFunction(t *testing.T) {
	program := lowerSource(t, `
		fn main() {
			"Hello"
		}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "Main")
	if !ok {
		t.Fatal("generated AST missing main function")
	}
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		t.Fatalf("generated AST gives void main a return type: %#v", fn.Type.Results)
	}
	if !astFuncHasBlankAssignString(fn, `"Hello"`) {
		t.Fatalf("generated AST does not discard final expression: %#v", fn.Body)
	}
	if astFuncHasReturnString(fn, `"Hello"`) {
		t.Fatalf("generated AST returns final expression from void function: %#v", fn.Body)
	}
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST still uses anonymous empty struct for Void")
	}
}
func TestLowerProgramUsesStructForVoidResultValues(t *testing.T) {
	program := lowerSource(t, `
		fn ok() Void!Str {
			Result::ok(())
		}

		fn main() Void {
			ok()
		}
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "Ok")
	if !ok || fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
		t.Fatalf("generated AST missing ok return type: %#v", fn)
	}
	resultType, ok := fn.Type.Results.List[0].Type.(*ast.IndexListExpr)
	if !ok || astExprName(resultType.X) != "ardruntime.Result" || len(resultType.Indices) != 2 || !isEmptyStructType(resultType.Indices[0]) || astExprName(resultType.Indices[1]) != "string" {
		t.Fatalf("generated AST missing void result container return type using struct{}: %#v", fn.Type.Results.List[0].Type)
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		kv, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := kv.Key.(*ast.Ident)
		lit, litOK := kv.Value.(*ast.CompositeLit)
		return keyOK && key.Name == "Value" && litOK && isEmptyStructType(lit.Type)
	}) {
		t.Fatal("generated AST missing struct{}{} Void value")
	}
}

func isEmptyStructType(expr ast.Expr) bool {
	st, ok := expr.(*ast.StructType)
	return ok && (st.Fields == nil || len(st.Fields.List) == 0)
}
func TestLowerProgramMaterializesVoidGlobalInitializers(t *testing.T) {
	program := lowerSource(t, `
		fn touch() Void { () }
		let saved = touch()
		fn main() Void { saved }
	`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok {
			return false
		}
		for _, expr := range value.Values {
			call, ok := expr.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Touch") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST uses no-value Void call as global initializer")
	}
	if !astFilesHaveCall(files, "Touch") {
		t.Fatal("generated AST does not materialize Void global initializer call")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		lit, ok := ret.Results[0].(*ast.CompositeLit)
		return ok && isEmptyStructType(lit.Type)
	}) {
		t.Fatal("generated AST does not return struct{}{} for materialized global")
	}
}
func TestRenderTestRunnerUsesStructForVoidResult(t *testing.T) {
	result := parse.Parse([]byte(`
		test fn check() Void!Str { Result::ok(()) }
	`), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.LowerWithTests(c.Module())
	if err != nil {
		t.Fatalf("lower with tests: %v", err)
	}
	runner := renderTestRunner(program, []TestCase{{Name: "check", DisplayName: "check", Function: program.Tests[0].Function}}, false)
	if !strings.Contains(runner, "func() runtime.Result[struct{}, string]") {
		t.Fatalf("test runner missing void result container using struct{}:\n%s", runner)
	}
}
func TestRunProgramExecutesGoErrorOnlyFunction(t *testing.T) {
	program := lowerSource(t, `
		use go:os

		fn main() {
			try os::Setenv("ARD_TEST_DIRECT_GO", "ok") -> err { panic(err) }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoCommaOkFunction(t *testing.T) {
	program := lowerSource(t, `
		use go:os

		fn main() {
			try os::Setenv("ARD_LOOKUP_TEST", "ok") -> err { panic(err) }
			let value = os::LookupEnv("ARD_LOOKUP_TEST").expect("missing")
			if value != "ok" {
				panic("bad lookup")
			}
			try os::Unsetenv("ARD_LOOKUP_TEST") -> err { panic(err) }
			if os::LookupEnv("ARD_LOOKUP_TEST").is_some() {
				panic("expected missing env")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoSliceFunctionCalls(t *testing.T) {
	program := lowerSource(t, `
		use go:sort
		use go:strings

		fn main() {
			mut values = [3, 1, 2]
			sort::Ints(values)
			if values.at(0) != 1 {
				panic("not sorted")
			}

			let parts = strings::Split("a,b", ",")
			if parts.size() != 2 or parts.at(0) != "a" {
				panic("bad split")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoPrimitiveScalarFunction(t *testing.T) {
	program := lowerSource(t, `
		use go:fmt
		use go:math

		fn main() {
			let bits: Uint32 = math::Float32bits(1.5)
			try fmt::Println(bits) -> err { panic(err) }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoNamedScalarLiteralCall(t *testing.T) {
	program := lowerSource(t, `
		use go:time

		fn main() {
			time::Sleep(1)
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoPackageConstant(t *testing.T) {
	program := lowerSource(t, `
		use go:time

		fn main() {
			time::Sleep(time::Nanosecond)
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoOpaqueNamedTypes(t *testing.T) {
	program := lowerSource(t, `
		use go:fmt
		use go:time

		fn main() {
			let loc = try time::LoadLocation("UTC") -> err { panic(err) }
			let when = time::Date(2024, time::January, 2, 0, 0, 0, 0, loc)
			try fmt::Println(when) -> err { panic(err) }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesGoFmtPrintln(t *testing.T) {
	program := lowerSource(t, `
		use go:fmt

		fn main() {
			try fmt::Println("hello") -> err { panic(err) }
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}

func TestRunProgramExecutesSimpleMain(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsModuleLevelLetCapturedByClosure(t *testing.T) {
	program := lowerSource(t, `
		let refresh_event = "inbox.refresh"

		fn main() {
			let event = refresh_event
			let read: fn() Str = fn() { event }
			if not read() == "inbox.refresh" {
				panic("wrong event")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsTransitiveSameNamedStructsFromDifferentModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{"models", "tui"} {
		if err := os.MkdirAll(filepath.Join(tempDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"models/inbox.ard": `
struct Store {
  item: Str,
}

fn new() Store {
  Store{item: "inbox"}
}
`,
		"models/issues.ard": `
struct Store {
  column: Str,
}

fn new() Store {
  Store{column: "issues"}
}
`,
		"tui/inbox_screen.ard": `
use app/models/inbox

struct Screen {
  store: inbox::Store,
}

fn new() Screen {
  Screen{store: inbox::new()}
}

impl Screen {
  fn item() Str { self.store.item }
}
`,
		"tui/issues_screen.ard": `
use app/models/issues

struct Screen {
  store: issues::Store,
}

fn new() Screen {
  Screen{store: issues::new()}
}

impl Screen {
  fn column() Str { self.store.column }
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/tui/inbox_screen
use app/tui/issues_screen

fn main() {
  let inbox = inbox_screen::new()
  let issues = issues_screen::new()
  if not inbox.item() == "inbox" {
    panic("wrong inbox screen")
  }
  if not issues.column() == "issues" {
    panic("wrong issues screen")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsSameNamedStructMethodsFromDifferentModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, module := range []struct{ name, label string }{{"left", "left"}, {"right", "right"}} {
		source := fmt.Sprintf(`
struct Store {
  label: Str,
}

fn new() Store {
  Store{label: %q}
}

impl Store {
  fn value() Str { self.label }
}
`, module.label)
		if err := os.WriteFile(filepath.Join(tempDir, module.name+".ard"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/left
use app/right

fn main() {
  if not left::new().value() == "left" {
    panic("wrong left value")
  }
  if not right::new().value() == "right" {
    panic("wrong right value")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsSameNamedStructsFromDifferentModules(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	modelsDir := filepath.Join(tempDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "inbox.ard"), []byte(`
struct Store {
  item: Str,
}

fn new() Store {
  Store{item: "inbox"}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelsDir, "issues.ard"), []byte(`
struct Store {
  column: Str,
}

fn new() Store {
  Store{column: "issues"}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/models/inbox
use app/models/issues

fn main() {
  let inbox_store = inbox::new()
  let issues_store = issues::new()
  if not inbox_store.item == "inbox" {
    panic("wrong inbox store")
  }
  if not issues_store.column == "issues" {
    panic("wrong issues store")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedModuleLevelLetCapturedByClosure(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let refresh_event = "inbox.refresh"

fn run() {
  let event = refresh_event
  let read: fn() Str = fn() { event }
  if not read() == "inbox.refresh" {
    panic("wrong event")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  feature::run()
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsModuleGlobalInitializerCallingInstanceMethod(t *testing.T) {
	program := lowerSource(t, `
		struct Source {}

		impl Source {
			fn value() Str { "ok" }
		}

		let source = Source{}
		let saved = source.value()

		fn main() {
			if not saved == "ok" {
				panic("wrong saved value")
			}
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedTraitObjectModuleGlobal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
trait Named {
  fn name() Str
}

struct Item {}

impl Named for Item {
  fn name() Str { "item" }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

let saved: feature::Named = feature::Item{}

fn main() {
  if not saved.name() == "item" {
    panic("wrong saved trait")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedFunctionSymbolReadingModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let refresh_event = "inbox.refresh"

fn event_name() Str {
  refresh_event
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let event_name: fn() Str = feature::event_name
  if not event_name() == "inbox.refresh" {
    panic("wrong event")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedFunctionValuedModuleLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let handler: fn() Str = fn() { "ok" }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let handler: fn() Str = feature::handler
  if not handler() == "ok" {
    panic("wrong handler symbol")
  }
  if not feature::handler() == "ok" {
    panic("wrong handler call")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedTraitMethodReadingModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let label = "imported"

trait Named {
  fn name() Str
}

struct Item {}

impl Named for Item {
  fn name() Str { label }
}

fn run() {
  let item: Named = Item{}
  if not item.name() == "imported" {
    panic("wrong trait name")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() { feature::run() }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSupportsImportedInstanceMethodReadingModuleLevelLet(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"app\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "feature.ard"), []byte(`
let label = "instance"

struct Item {}

impl Item {
  fn name() Str { label }
}

fn make() Item { Item{} }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(tempDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`
use app/feature

fn main() {
  let item = feature::make()
  if not item.name() == "instance" {
    panic("wrong instance name")
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	if err := RunProgram(program, []string{"ard", "run", mainPath}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestRunProgramSpecializesGenericEmptyListLocal(t *testing.T) {
	program := lowerSource(t, `
		fn drop(from: [$T], till: Int) [$T] {
			mut out: [$T] = []
			for item, idx in from {
				if idx >= till {
					out.push(item)
				}
			}
			out
		}

		fn main() Bool {
			let dropped = drop([1, 2, 3], 1)
			dropped.size() == 2 and dropped.at(0) == 2
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestBuildProgramSpecializesNestedGenericLambdasPerOuterBinding(t *testing.T) {
	workspace := t.TempDir()
	sharedDir := filepath.Join(workspace, "state-shared")
	appDir := filepath.Join(workspace, "state-app")
	for _, dir := range []string{sharedDir, appDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "ard.toml"), []byte("name = \"state-shared\"\nard = \">= 0.23.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sharedDir, "shared.ard"), []byte(`struct State<$T> { handle: Int }

fn _stateful<$T>(
  init: fn(Int) $T,
  build: fn(Int) Int,
) Int {
  let _ = init(0)
  build(0)
}

fn stateful<$T>(
  init: fn() $T,
  build: fn(State<$T>) Int,
) Int {
  _stateful(
    init: fn(h: Int) $T {
      init()
    },
    build: fn(h: Int) Int {
      build(State{handle: h})
    },
  )
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "ard.toml"), []byte("name = \"state-app\"\nard = \">= 0.23.0\"\n\n[dependencies]\nstate-shared = { path = \"../state-shared\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"main.ard": `use state-app/a
use state-app/b

fn main() Void {
  a::go()
  b::go()
}
`,
		"a.ard": `use state-shared/shared

struct State { x: Int }

fn go() Void {
  let _ = shared::stateful(
    fn() State { State{x: 1} },
    fn(s: shared::State<State>) Int { 0 },
  )
}
`,
		"b.ard": `use state-shared/shared

struct State { y: Str }

fn go() Void {
  let _ = shared::stateful(
    fn() State { State{y: "hi"} },
    fn(s: shared::State<State>) Int { 0 },
  )
}
`,
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(appDir, name), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mainPath := filepath.Join(appDir, "main.ard")
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}
	if _, err := BuildProgram(program, filepath.Join(appDir, "app"), loaded.ProjectInfo); err != nil {
		t.Fatalf("build: %v", err)
	}
}
func TestRunProgramAllowsModuleWithoutEntry(t *testing.T) {
	program := lowerSource(t, `
		fn add(a: Int, b: Int) Int {
			a + b
		}
	`)

	if err := RunProgram(program, []string{"ard", "run", "sample.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
}
func TestLowerProgramSupportsStructsAndEnums(t *testing.T) {
	program := lowerSource(t, `
		enum Direction {
			Up, Down
		}

		struct User {
			name: Str,
			age: Int,
		}

		fn direction() Direction {
			Direction::Down
		}

		fn next_age() Int {
			let user = User{name: "Ada", age: 41}
			user.age + 1
		}

		fn main() Int {
			next_age()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveTypeSpec(files, "Direction") {
		t.Fatal("generated AST missing enum type")
	}
	if !astFilesHaveValueSpec(files, "DirectionDown") {
		t.Fatal("generated AST missing enum constants")
	}
	if !astFilesHaveTypeSpec(files, "User") {
		t.Fatal("generated AST missing struct type")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.CompositeLit)
		if !ok || astExprName(lit.Type) != "User" {
			return false
		}
		hasName := false
		hasAge := false
		for _, elem := range lit.Elts {
			kv, ok := elem.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := kv.Key.(*ast.Ident)
			if !keyOK {
				continue
			}
			if key.Name == "Name" {
				value, ok := kv.Value.(*ast.BasicLit)
				hasName = ok && value.Value == `"Ada"`
			}
			if key.Name == "Age" {
				value, ok := kv.Value.(*ast.BasicLit)
				hasAge = ok && value.Value == "41"
			}
		}
		return hasName && hasAge
	}) {
		t.Fatal("generated AST missing struct literal lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD {
			return false
		}
		selector, ok := binary.X.(*ast.SelectorExpr)
		return ok && selector.Sel.Name == "Age"
	}) {
		t.Fatal("generated AST missing field access lowering")
	}
}
func TestLowerProgramSupportsTryMaybeCatchAndEarlyReturn(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn missing() Int? {
			maybe::none()
		}

		fn with_default() Int {
			let value = try missing() -> _ { 42 }
			value
		}

		fn passthrough() Int? {
			let value = try missing()
			maybe::some(value)
		}

		fn main() Int {
			with_default()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		ident, ok := ret.Results[0].(*ast.Ident)
		return ok && strings.HasPrefix(ident.Name, "_tmp_")
	}) {
		t.Fatal("generated AST missing try early return lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			lit, ok := rhs.(*ast.BasicLit)
			if ok && lit.Value == "42" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing try catch lowering")
	}
}
func TestLowerProgramPropagatesTryResultAcrossDifferentResultValueTypes(t *testing.T) {
	program := lowerSource(t, `
		fn read_text() Str!Str {
			Result::err("bad")
		}

		fn parse() Int!Str {
			let text = try read_text()
			let _ignore = text
			Result::ok(1)
		}

		fn main() Int!Str {
			parse()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return false
		}
		lit, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok || astExprName(lit.Type) != "ardruntime.Result" {
			return false
		}
		for _, elem := range lit.Elts {
			kv, ok := elem.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, keyOK := kv.Key.(*ast.Ident)
			if !keyOK || key.Name != "Err" {
				continue
			}
			if value, ok := kv.Value.(*ast.Ident); ok && strings.HasPrefix(value.Name, "_tmp_") {
				return true
			}
			if selector, ok := kv.Value.(*ast.SelectorExpr); ok {
				if ident, ok := selector.X.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "_tmp_") && selector.Sel.Name == "Err" {
					return true
				}
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing result error propagation conversion")
	}
}
func TestArtifactWorkspacePreservesGoModuleFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := artifactWorkspace(dir, "run")
	if err != nil {
		t.Fatalf("artifact workspace: %v", err)
	}
	goMod := []byte("module generated\n\nrequire example.com/cached v1.0.0\n")
	goSum := []byte("example.com/cached v1.0.0 h1:abc\n")
	if err := os.WriteFile(filepath.Join(workspace, "go.mod"), goMod, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "go.sum"), goSum, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "stale.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	workspace, err = artifactWorkspace(dir, "run")
	if err != nil {
		t.Fatalf("artifact workspace: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "go.mod")); err != nil || string(got) != string(goMod) {
		t.Fatalf("preserved go.mod = %q, %v", string(got), err)
	}
	if got, err := os.ReadFile(filepath.Join(workspace, "go.sum")); err != nil || string(got) != string(goSum) {
		t.Fatalf("preserved go.sum = %q, %v", string(got), err)
	}
	if fileExists(filepath.Join(workspace, "stale.go")) {
		t.Fatal("artifact workspace kept stale generated file")
	}
}
func TestLowerProgramUsesRuntimeMaybeForRecursiveNullableFields(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		struct Node { value: Int, parent: Node? }

		fn main() Int {
			let root = Node{value: 1, parent: maybe::none()}
			let child = Node{value: 2, parent: maybe::some(root)}
			child.parent.expect("").value
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Names[0].Name != "Parent" {
			return false
		}
		indexed, ok := field.Type.(*ast.IndexExpr)
		return ok && astExprName(indexed.X) == "ardruntime.Maybe" && astExprName(indexed.Index) == "Node"
	}) {
		t.Fatal("generated AST missing runtime Maybe recursive nullable field")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		field, ok := node.(*ast.Field)
		if !ok || len(field.Names) != 1 || field.Names[0].Name != "Parent" {
			return false
		}
		star, ok := field.Type.(*ast.StarExpr)
		return ok && astExprName(star.X) == "Node"
	}) {
		t.Fatal("generated AST lowered recursive nullable field as pointer")
	}
}
func TestLowerProgramUsesExpectedLocalTypeForMaybeNone(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			let found: Int? = maybe::none()
			found.is_none()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "ardruntime.None" {
			return false
		}
		indexed, ok := call.Fun.(*ast.IndexExpr)
		return ok && astExprName(indexed.Index) == "int"
	}) {
		t.Fatal("generated AST missing typed maybe none")
	}
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST used untyped maybe none")
	}
}
func TestLowerProgramUsesExpectedDefaultTypeForResultOr(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn fetch() Int?!Str {
			let empty: Int? = maybe::none()
			Result::ok(empty)
		}

		fn main() Bool {
			let value = fetch().or(maybe::none())
			value.is_none()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesHaveEmptyStructType(files) {
		t.Fatal("generated AST used untyped maybe default")
	}
}
func TestLowerProgramSkipsVoidAssignmentForStatementMatchBranches(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Bool {
			match maybe::some(1) {
				value => value == 1,
				_ => (),
			}
			false
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			ident, ok := rhs.(*ast.Ident)
			if ok && ident.Name == "nil" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST assigned nil in statement match lowering")
	}
}
func TestTypeNameUsesModulePathAndUniqueFallback(t *testing.T) {
	program := &air.Program{}
	inbox := typeName(program, air.TypeInfo{ID: 1, Name: "Store", ModulePath: "app/models/inbox"})
	issues := typeName(program, air.TypeInfo{ID: 2, Name: "Store", ModulePath: "app/models/issues"})
	if inbox != "App_models_inbox__Store" || issues != "App_models_issues__Store" {
		t.Fatalf("module type names = %q, %q", inbox, issues)
	}

	left := typeName(program, air.TypeInfo{ID: 3, Name: "Request"})
	right := typeName(program, air.TypeInfo{ID: 4, Name: "Request"})
	if left == right {
		t.Fatalf("fallback type names should be unique, got %q", left)
	}
}

// A `mut <direct-Go handle>` struct field is a pointer-valued handle (e.g.
// *sql.DB), lowered as a plain pointer field with no mutable-reference (&/*)
// machinery, since the Ard value already IS the Go pointer (ADR 0031).
func TestLowerProgramUsesPointersForMutableStructParams(t *testing.T) {
	program := lowerSource(t, `
		struct Response {
			body: Str,
		}

		fn set_body(mut res: Response) Void {
			res.body = "ok"
		}

		fn main() Void {
			mut res = Response{body: ""}
			set_body(res)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	fn, ok := astFilesFunc(files, "SetBody")
	if !ok || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
		t.Fatalf("generated AST missing set_body function")
	}
	paramType, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok || astExprName(paramType.X) != "Response" {
		t.Fatalf("generated AST missing pointer mutable param lowering: %#v", fn.Type.Params.List[0].Type)
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "SetBody" || len(call.Args) == 0 {
			return false
		}
		addr, ok := call.Args[0].(*ast.UnaryExpr)
		if !ok || addr.Op != token.AND {
			return false
		}
		ident, ok := addr.X.(*ast.Ident)
		return ok && ident.Name == "res"
	}) {
		t.Fatal("generated AST missing pointer call lowering")
	}
}
func TestLowerProgramSupportsCapturedClosureSort(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [3, 1, 2]
			let bias = 0
			items.sort(fn(a: Int, b: Int) Bool {
				a + bias < b + bias
			})
			items.at(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveCall(files, "sort.SliceStable") {
		t.Fatal("generated AST missing list sort lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		lit, ok := node.(*ast.FuncLit)
		return ok && lit.Type.Params != nil && len(lit.Type.Params.List) == 2 && lit.Type.Results != nil && len(lit.Type.Results.List) == 1 && astExprName(lit.Type.Results.List[0].Type) == "bool"
	}) {
		t.Fatal("generated AST missing closure literal lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		ident, ok := node.(*ast.Ident)
		return ok && strings.HasPrefix(ident.Name, "bias")
	}) {
		t.Fatal("generated AST missing closure capture usage")
	}
	if astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should inline local closure body instead of emitting an anon helper")
	}
}
func TestLowerProgramInlinesNestedImmediateClosures(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Int {
			let bias = 2
			let result = maybe::some(40).map(fn(value) {
				maybe::some(value).map(fn(inner) { inner + bias }).or(0)
			})
			result.or(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should inline nested immediate closures instead of emitting anon helpers")
	}
	funcLits := 0
	astFilesContain(files, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			funcLits++
		}
		return false
	})
	if funcLits < 2 {
		t.Fatalf("generated AST missing nested function literals: got %d", funcLits)
	}
}
func TestLowerProgramKeepsHelperForMutableCaptureClosure(t *testing.T) {
	program := lowerSource(t, `
		use ard/maybe

		fn main() Int {
			mut total = 0
			let result = maybe::some(1).map(fn(value) {
				total = total + value
				total
			})
			result.or(0)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should keep helper for mutable capture closure")
	}
}
func TestLowerProgramKeepsHelperForRetainedClosure(t *testing.T) {
	program := lowerSource(t, `
		fn make_adder(offset: Int) fn(Int) Int {
			fn(value: Int) Int {
				value + offset
			}
		}

		fn main() Int {
			let add = make_adder(2)
			add(3)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesHaveFuncContaining(files, "anon_func") {
		t.Fatal("generated AST should keep helper for retained closure")
	}
}
func TestLowerProgramEmitsGoMethodWrapperForInherentImpl(t *testing.T) {
	program := lowerSource(t, `
		struct Box {
			value: Int,
		}

		impl Box {
			fn Count() Int {
				self.value
			}
		}

		fn main() Int {
			let box = Box{value: 7}
			box.Count()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Count" || len(fn.Recv.List) != 1 {
			return false
		}
		foundCall := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Box_Count") {
				foundCall = true
				return false
			}
			return true
		})
		return foundCall
	}) {
		t.Fatal("generated AST missing Go method wrapper for inherent impl")
	}
}
func TestLowerProgramEmitsGoMethodWrapperForTraitImpl(t *testing.T) {
	program := lowerSource(t, `
		trait Labeled {
			fn Label() Str
		}

		struct Button {
			text: Str,
		}

		impl Labeled for Button {
			fn Label() Str {
				self.text
			}
		}

		fn label(value: Labeled) Str {
			value.Label()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Label" || len(fn.Recv.List) != 1 {
			return false
		}
		foundCall := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Button_Labeled_Label") {
				foundCall = true
				return false
			}
			return true
		})
		return foundCall
	}) {
		t.Fatal("generated AST missing Go method wrapper for trait impl")
	}
}
func TestLowerProgramEmitsGoInterfaceForTraitObject(t *testing.T) {
	program := lowerSource(t, `
		trait Renderable {
			fn render() Str
			fn area(scale: Int) Int
		}

		struct Block {
			title: Str,
		}

		impl Renderable for Block {
			fn render() Str {
				self.title
			}

			fn area(scale: Int) Int {
				scale
			}
		}

		fn draw(value: Renderable) Str {
			value.render()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || typeSpec.Name.Name != "Renderable" {
			return false
		}
		iface, ok := typeSpec.Type.(*ast.InterfaceType)
		if !ok || iface.Methods == nil || len(iface.Methods.List) != 2 {
			return false
		}
		methods := map[string]*ast.FuncType{}
		for _, method := range iface.Methods.List {
			if len(method.Names) != 1 {
				return false
			}
			fnType, ok := method.Type.(*ast.FuncType)
			if !ok {
				return false
			}
			methods[method.Names[0].Name] = fnType
		}
		render, ok := methods["Render"]
		if !ok || render.Params == nil || len(render.Params.List) != 0 || render.Results == nil || len(render.Results.List) != 1 || astExprName(render.Results.List[0].Type) != "string" {
			return false
		}
		area, ok := methods["Area"]
		return ok && area.Params != nil && len(area.Params.List) == 1 && astExprName(area.Params.List[0].Type) == "int" && area.Results != nil && len(area.Results.List) == 1 && astExprName(area.Results.List[0].Type) == "int"
	}) {
		t.Fatal("generated AST missing Go interface for Ard trait")
	}
	if !astFilesHaveTypeSpec(files, "ardMutTrait_Renderable_0") {
		t.Fatal("generated AST should keep mutable trait reference type")
	}
}
func TestLowerProgramSkipsGoMethodWrapperWhenStructFieldCollides(t *testing.T) {
	program := lowerSource(t, `
		trait Named {
			fn Name() Str
		}

		struct User {
			name: Str,
		}

		impl Named for User {
			fn Name() Str {
				self.name
			}
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		return ok && fn.Recv != nil && fn.Name != nil && fn.Name.Name == "Name"
	}) {
		t.Fatal("generated AST should not emit Go method wrapper that collides with a struct field")
	}
}
func TestLowerProgramSkipsGoMethodWrapperForReservedStructReceiverMethods(t *testing.T) {
	program := lowerSource(t, `
		struct Payload {
			value: Int,
		}

		impl Payload {
			fn MarshalJSONTo() Int {
				self.value
			}

			fn UnmarshalJSONFrom() Int {
				self.value
			}
		}

		fn main() Int {
			let payload = Payload{value: 1}
			payload.MarshalJSONTo() + payload.UnmarshalJSONFrom()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, reserved := range []string{"MarshalJSONTo", "UnmarshalJSONFrom"} {
		if astFilesContain(files, func(node ast.Node) bool {
			fn, ok := node.(*ast.FuncDecl)
			return ok && fn.Recv != nil && fn.Name != nil && fn.Name.Name == reserved
		}) {
			t.Fatalf("generated AST should not emit Go method wrapper %s reserved for generated JSON helpers", reserved)
		}
	}
}
func TestLowerProgramPassesPointerReceiverForMutatingTraitImpl(t *testing.T) {
	program := lowerSource(t, `
		trait Writer {
			fn write(text: Str)
		}

		struct Buffer {
			contents: Str,
		}

		impl Writer for Buffer {
			fn mut write(text: Str) {
				self.contents = self.contents + text
			}
		}

		fn send(w: Writer) {
			w.write("hi")
		}

		fn main() {
			mut buffer = Buffer{contents: ""}
			send(buffer)
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name == nil || typeSpec.Name.Name != "Writer" {
			return false
		}
		_, ok = typeSpec.Type.(*ast.InterfaceType)
		return ok
	}) {
		t.Fatal("generated AST missing native Go interface for trait with mutating impl")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Name == nil || !strings.Contains(fn.Name.Name, "Buffer_Writer_write") || fn.Type.Params == nil || len(fn.Type.Params.List) == 0 {
			return false
		}
		if len(fn.Type.Params.List[0].Names) == 0 || fn.Type.Params.List[0].Names[0].Name != "self" {
			return false
		}
		_, ok = fn.Type.Params.List[0].Type.(*ast.StarExpr)
		return ok
	}) {
		t.Fatal("generated AST missing pointer receiver for mutating trait impl")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Name == nil || fn.Name.Name != "Write" || len(fn.Recv.List) != 1 {
			return false
		}
		_, ok = fn.Recv.List[0].Type.(*ast.StarExpr)
		return ok
	}) {
		t.Fatal("generated AST missing pointer Go method wrapper for mutating trait impl")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || astCallName(call) != "Send" || len(call.Args) != 1 {
			return false
		}
		conversion, ok := call.Args[0].(*ast.CallExpr)
		if !ok || astCallName(conversion) != "Writer" || len(conversion.Args) != 1 {
			return false
		}
		addr, ok := conversion.Args[0].(*ast.UnaryExpr)
		return ok && addr.Op == token.AND
	}) {
		t.Fatal("generated AST missing address-of when passing mutating impl to native trait interface")
	}
}
func TestLowerProgramUsesCallSiteImportsForCrossModuleTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"nestprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "tui", "core")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"tui/core/widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"tui/core/text.ard": `
use nestprobe/tui/core/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn mut render(frame: widget::Frame) { () }
}

fn plain(content: Str) widget::Widget {
  Text{content: content}
}
`,
		"tui/core/box.ard": `
use nestprobe/tui/core/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn mut render(frame: widget::Frame) {
    self.child.render(frame)
  }
}

fn wrap(child: widget::Widget) widget::Widget {
  Box{child: child}
}
`,
		"commands/demo.ard": `
use nestprobe/tui/core/widget
use nestprobe/tui/core/text as textw
use nestprobe/tui/core/box as boxw

fn run() {
  let f = widget::Frame{size: 10}
  let demo = boxw::wrap(textw::plain("hi"))
  demo.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use nestprobe/commands/demo

fn main() {
  demo::run()
}
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	generatedFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(generatedFiles, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST should use native interface dispatch for call-site trait dispatch")
	}
}
func TestLowerProgramUsesAliasOriginImportsForTraitObjectDispatch(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "ard.toml"), []byte("name = \"aliasprobe\"\nard = \">= 0.13.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{filepath.Join(tempDir, "commands"), filepath.Join(tempDir, "widgets")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"widget.ard": `
struct Frame { size: Int }

trait Widget {
  fn render(frame: Frame)
}
`,
		"widgets/text.ard": `
use aliasprobe/widget

struct Text { content: Str }

impl widget::Widget for Text {
  fn mut render(frame: widget::Frame) { () }
}

fn new(content: Str) widget::Widget { Text{content: content} }
`,
		"widgets/box.ard": `
use aliasprobe/widget

struct Box { child: widget::Widget }

impl widget::Widget for Box {
  fn mut render(frame: widget::Frame) { self.child.render(frame) }
}

fn new(child: widget::Widget) widget::Widget { Box{child: child} }
`,
		"facade_let.ard": `
use aliasprobe/widgets/text
use aliasprobe/widgets/box

let make_text = text::new
let make_box = box::new
`,
		"commands/demo.ard": `
use aliasprobe/widget
use aliasprobe/facade_let as facade

fn run() {
  let f = widget::Frame{size: 10}
  let w = facade::make_box(facade::make_text("hi"))
  w.render(f)
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, filepath.FromSlash(name)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	resolver, err := checker.NewModuleResolver(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	result := parse.Parse([]byte(`
use aliasprobe/widgets/text
use aliasprobe/widgets/box
use aliasprobe/commands/demo

fn main() { demo::run() }
`), filepath.Join(tempDir, "main.ard"))
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New(filepath.Join(tempDir, "main.ard"), result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	generatedFiles := lowerProgramAST(t, program, Options{PackageName: "main"})
	if astFilesContain(generatedFiles, func(node ast.Node) bool {
		_, ok := node.(*ast.TypeSwitchStmt)
		return ok
	}) {
		t.Fatal("generated AST should use native interface dispatch for aliased-constructor trait dispatch")
	}
}

func TestLowerProgramSupportsListSwapAndMapKeys(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut items = [1, 2, 3]
			items.swap(0, 2)
			let values = ["b": 2, "a": 1]
			let keys = values.keys()
			items.at(0) + keys.size()
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) == 0 {
			return false
		}
		_, ok = assign.Lhs[0].(*ast.IndexExpr)
		return ok
	}) {
		t.Fatal("generated AST missing list swap lowering")
	}
	if !astFilesHaveCall(files, "ardSortedStringKeys") {
		t.Fatal("generated AST missing map keys lowering")
	}
}
func TestLowerProgramEmitsOnlyUsedImports(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			1
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, importPath := range []string{"bufio", "strconv", "strings"} {
		if astFilesHaveImport(files, "", importPath) {
			t.Fatalf("generated AST included unused runtime import %q", importPath)
		}
	}
}
func TestLowerProgramSupportsFieldMutation(t *testing.T) {
	program := lowerSource(t, `
		struct Counter {
			value: Int,
		}

		fn bump(counter: Counter) Int {
			mut current = counter
			current.value = current.value + 1
			current.value
		}

		fn main() Int {
			bump(Counter{value: 1})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			return false
		}
		lhs, ok := assign.Lhs[0].(*ast.SelectorExpr)
		if !ok || lhs.Sel.Name != "Value" {
			return false
		}
		binary, ok := assign.Rhs[0].(*ast.BinaryExpr)
		if !ok || binary.Op != token.ADD {
			return false
		}
		rhsSelector, ok := binary.X.(*ast.SelectorExpr)
		lit, litOK := binary.Y.(*ast.BasicLit)
		return ok && rhsSelector.Sel.Name == "Value" && litOK && lit.Value == "1"
	}) {
		t.Fatal("generated AST missing field mutation lowering")
	}
}
func TestLowerProgramSupportsIfAndWhile(t *testing.T) {
	program := lowerSource(t, `
		fn main() Int {
			mut count = 0
			while count < 3 {
				count = count + 1
			}
			if count == 3 {
				count
			} else {
				0
			}
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		stmt, ok := node.(*ast.ForStmt)
		if !ok {
			return false
		}
		cond, ok := stmt.Cond.(*ast.BinaryExpr)
		lit, litOK := cond.Y.(*ast.BasicLit)
		return ok && cond.Op == token.LSS && litOK && lit.Value == "3"
	}) {
		t.Fatal("generated AST missing while lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		stmt, ok := node.(*ast.IfStmt)
		if !ok {
			return false
		}
		cond, ok := stmt.Cond.(*ast.BinaryExpr)
		lit, litOK := cond.Y.(*ast.BasicLit)
		return ok && cond.Op == token.EQL && litOK && lit.Value == "3"
	}) {
		t.Fatal("generated AST missing if lowering")
	}
	if !astFilesContain(files, func(node ast.Node) bool {
		value, ok := node.(*ast.ValueSpec)
		if !ok || astExprName(value.Type) != "int" {
			return false
		}
		for _, name := range value.Names {
			if strings.HasPrefix(name.Name, "_tmp_") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("generated AST missing expression temp lowering")
	}
}
func TestWriteProgramUsesEmbeddedArdModuleForReleaseVersion(t *testing.T) {
	original := version.Version
	version.Version = "v0.19.1"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") {
		t.Fatalf("go.mod missing Ard module requirement:\n%s", goMod)
	}
	if !strings.Contains(goMod, "replace github.com/akonwi/ard => ./.ard/ard-module") {
		t.Fatalf("release go.mod missing embedded module replace:\n%s", goMod)
	}
	if strings.Contains(goMod, "/home/runner") {
		t.Fatalf("release go.mod must not contain CI source path:\n%s", goMod)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ard", "ard-module", "runtime", "maybe.go")); err != nil {
		t.Fatalf("embedded runtime module not written: %v", err)
	}
}
func TestWriteProgramUsesLocalReplaceForDevVersion(t *testing.T) {
	original := version.Version
	version.Version = "dev"
	t.Cleanup(func() { version.Version = original })

	program := lowerSource(t, `
		fn main() Void {
		}
	`)
	dir := t.TempDir()
	if err := writeProgram(dir, program, Options{PackageName: "main"}); err != nil {
		t.Fatalf("writeProgram error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}
	goMod := string(data)
	if !strings.Contains(goMod, "require github.com/akonwi/ard v0.0.0") || !strings.Contains(goMod, "replace github.com/akonwi/ard =>") {
		t.Fatalf("dev go.mod missing local replace:\n%s", goMod)
	}
}
func TestBuildProgramProducesBinary(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)

	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "ard-bin")
	builtPath, err := BuildProgram(program, outputPath)
	if err != nil {
		t.Fatalf("BuildProgram error = %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("built binary stat error = %v", err)
	}
}
func TestRunProgramPreservesArtifactsUnderArdOut(t *testing.T) {
	program := lowerSource(t, `
		fn main() Void {
			()
		}
	`)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projectDir := t.TempDir()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := RunProgram(program, []string{"ard", "run", "main.ard"}); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(projectDir, "ard-out", "go", "run", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected generated sources under %s", filepath.Join(projectDir, "ard-out", "go", "run"))
	}
}
func TestRunBinaryNameSanitizesProjectName(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		want        string
	}{
		{name: "empty", projectName: "", want: "ard-program"},
		{name: "dot dot", projectName: "..", want: "ard-program"},
		{name: "plain", projectName: "tinear", want: "tinear"},
		{name: "hyphen", projectName: "demo-app", want: "demo-app"},
		{name: "path chars", projectName: `bad/name:with*chars?`, want: "bad_name_with_chars_"},
		{name: "only invalid chars", projectName: `/**`, want: "ard-program"},
		{name: "reserved windows name", projectName: "CON", want: "ard-CON"},
		{name: "reserved windows name with extension", projectName: "nul.txt", want: "ard-nul.txt"},
		{name: "trims spaces and dots", projectName: " team. ", want: "team"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runBinaryName(&checker.ProjectInfo{ProjectName: tt.projectName})
			if got != tt.want {
				t.Fatalf("runBinaryName(%q) = %q, want %q", tt.projectName, got, tt.want)
			}
		})
	}
	if got := runBinaryName(nil); got != "ard-program" {
		t.Fatalf("runBinaryName(nil) = %q, want ard-program", got)
	}
}
func TestRunProgramNamesBinaryAfterProject(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"tinear\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte(`fn main() Int { 1 }
`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := frontend.LoadModule(mainPath)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		t.Fatalf("lower: %v", err)
	}

	if err := RunProgram(program, []string{"ard", "run", mainPath}, loaded.ProjectInfo); err != nil {
		t.Fatalf("RunProgram error = %v", err)
	}

	workspaceDir := filepath.Join(projectDir, "ard-out", "go", "run")
	binaryInfo, err := os.Stat(filepath.Join(workspaceDir, ".bin", "tinear"))
	if err != nil || binaryInfo.IsDir() {
		t.Fatalf("project-named binary stat = %v, info = %#v", err, binaryInfo)
	}
	if _, err := os.Stat(filepath.Join(workspaceDir, "ard-program")); !os.IsNotExist(err) {
		t.Fatalf("legacy ard-program binary should not exist, stat error = %v", err)
	}
}
func TestArtifactWorkspaceUsesProjectLocalArdOut(t *testing.T) {
	projectDir := t.TempDir()
	mainPath := filepath.Join(projectDir, "main.ard")
	if err := os.WriteFile(mainPath, []byte("fn main() {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	root, err := artifactRootDir(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	if root != projectDir {
		t.Fatalf("artifact root = %q, want %q", root, projectDir)
	}
	workspace, err := artifactWorkspace(mainPath, "build")
	if err != nil {
		t.Fatal(err)
	}
	if workspace != filepath.Join(projectDir, "ard-out", "go", "build") {
		t.Fatalf("workspace = %q, want %q", workspace, filepath.Join(projectDir, "ard-out", "go", "build"))
	}
}

func mapsKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func joinGeneratedSources(sources map[string][]byte) string {
	keys := mapsKeys(sources)
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.Write(sources[key])
		b.WriteByte('\n')
	}
	return b.String()
}

func writeGeneratedSourcesForTest(t testing.TB, dir string, sources map[string][]byte) {
	t.Helper()
	for name, source := range sources {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create source dir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
	}
}

func buildProgramFromGeneratedSources(t *testing.T, program *air.Program, outputName string) {
	t.Helper()
	tempDir := t.TempDir()
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatalf("generate sources: %v", err)
	}
	writeGeneratedSourcesForTest(t, tempDir, sources)
	goMod, err := generatedGoMod(tempDir, program, nil)
	if err != nil {
		t.Fatalf("generate go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := buildGeneratedProgram(tempDir, filepath.Join(tempDir, outputName)); err != nil {
		t.Fatalf("build generated program: %v", err)
	}
}
func TestInlineClosureBuildsWithPredeclaredNameParameter(t *testing.T) {
	program := lowerSource(t, `use ard/maybe
fn main() {
  maybe::some(1).map(fn(int64) { int64 + 1 }).or(0)
}`)
	buildProgramFromGeneratedSources(t, program, "inline-closure-predeclared-param")
}
func TestInlineClosureBuildsWhenParamCollidesWithCaptureRewrite(t *testing.T) {
	program := lowerSource(t, `use ard/maybe
fn main() {
  let x_0 = 10
  maybe::some(1).map(fn(x) { x + x_0 }).or(0)
}`)
	buildProgramFromGeneratedSources(t, program, "inline-closure-capture-param-collision")
}
func TestLowererRenamesImportAliasPathConflicts(t *testing.T) {
	l := &lowerer{currentImports: map[string]string{}}
	first := l.registerImport("fmt", "example.com/fmt")
	second := l.registerImport("fmt", "fmt")
	if first != "fmt_1" || second != "fmt" {
		t.Fatalf("aliases = %q, %q; want fmt_1, fmt", first, second)
	}
	if l.importErr != nil {
		t.Fatalf("importErr = %v, want nil", l.importErr)
	}
}
func TestLocalNameKeepsBareNameWhenShadowingUnusedOuter(t *testing.T) {
	input := `
		fn f(x: Int) Int {
			mut total = 0
			for x in [1, 2, 3] {
				total = total + x
			}
			total + x
		}
		fn main() Int {
			f(100)
		}
	`
	program := lowerSource(t, input)
	sources, err := GenerateSources(program, Options{PackageName: "main"})
	if err != nil {
		t.Fatal(err)
	}
	joined := joinGeneratedSources(sources)
	// The loop variable shadows the parameter only inside the loop body, where
	// the parameter is never referenced, so it keeps the bare name. The post-loop
	// use of the parameter resolves to the (still bare) parameter.
	if !strings.Contains(joined, "func F(x int)") {
		t.Fatalf("expected bare parameter name:\n%s", joined)
	}
	if !strings.Contains(joined, "x := x_list[x_index]") {
		t.Fatalf("expected bare shadowing loop variable:\n%s", joined)
	}
}
func TestRenderTestRunnerAliasesImportsAroundTopLevelNames(t *testing.T) {
	program := &air.Program{Functions: []air.Function{
		{ID: 0, Module: 0, Name: "os", Private: true},
		{ID: 1, Module: 0, Name: "runtime", Private: true},
	}}
	runner := renderTestRunner(program, nil, false)
	if !strings.Contains(runner, "os_1 \"os\"") || !strings.Contains(runner, "runtime_1 \"github.com/akonwi/ard/runtime\"") {
		t.Fatalf("test runner did not alias conflicting imports:\n%s", runner)
	}
	if !strings.Contains(runner, "os_1.Stderr") || !strings.Contains(runner, "runtime_1.Result") {
		t.Fatalf("test runner did not use aliased imports:\n%s", runner)
	}
}
func TestLowererImportAliasAvoidsSinglePackageTopLevelNamesAcrossModules(t *testing.T) {
	program := &air.Program{
		Modules: []air.Module{
			{ID: 0, Path: "a.ard"},
			{ID: 1, Path: "b.ard", Functions: []air.FunctionID{0}},
		},
		Functions: []air.Function{{ID: 0, Module: 1, Name: "fmt", Private: true}},
	}
	l := &lowerer{program: program, currentModule: 0, currentImports: map[string]string{}}
	if got := l.registerImport("fmt", "strings"); got != "fmt_2" {
		t.Fatalf("single-package conflicting import alias = %q, want fmt_2", got)
	}
}
func TestLowerStructFieldsAreAlwaysExportedWithJSONTags(t *testing.T) {
	program := lowerSource(t, `struct User {
  first_name: Str
  type: Int
}

private struct internal_config {
  secret_key: Str
}

fn make_user() User { User{first_name: "Ada", type: 1} }
fn main() internal_config { internal_config{secret_key: "s"} }`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	for _, field := range []string{"FirstName", "Type"} {
		if !astFilesHaveStructField(files, "User", field) {
			t.Fatalf("generated public User missing exported field %s", field)
		}
	}
	// Fields are always exported, even on private structs, so the struct is
	// serializable; the wire name is pinned via a json tag to the Ard name.
	if !astFilesHaveStructField(files, "internalConfig", "SecretKey") {
		t.Fatal("generated private internal_config missing exported field SecretKey")
	}
	if !astFilesHaveStructFieldTag(files, "User", "FirstName", "`json:\"first_name\"`") {
		t.Fatal("generated User.FirstName missing json tag pinned to the Ard field name")
	}
	if !astFilesHaveStructFieldTag(files, "internalConfig", "SecretKey", "`json:\"secret_key\"`") {
		t.Fatal("generated internal_config.SecretKey missing json tag pinned to the Ard field name")
	}
}

func astFilesHaveStructFieldTag(files map[string]*ast.File, typeName string, fieldName string, tag string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != typeName {
			return false
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == fieldName {
					return field.Tag != nil && field.Tag.Value == tag
				}
			}
		}
		return false
	})
}

func astFilesHaveStructField(files map[string]*ast.File, typeName string, fieldName string) bool {
	return astFilesContain(files, func(node ast.Node) bool {
		typeSpec, ok := node.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != typeName {
			return false
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				if name.Name == fieldName {
					return true
				}
			}
		}
		return false
	})
}

func lowerSource(t *testing.T, input string) *air.Program {
	t.Helper()
	return lowerSourceWithCheckOptions(t, input, checker.CheckOptions{})
}

func lowerSourceWithCheckOptions(t *testing.T, input string, options checker.CheckOptions) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("test.ard", result.Program, nil, options)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
func TestLowerGenericStructEmitsGoGeneric(t *testing.T) {
	program := lowerSource(t, `struct Box {
  value: [$T]
}

fn wrap(items: [$T]) Box<$T> { Box{value: items} }

fn main() Int {
  let b = wrap([1, 2, 3])
  b.value.size()
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	// The generic definition is one Go generic type `type Box[T any]`.
	if !astFilesContain(files, func(node ast.Node) bool {
		spec, ok := node.(*ast.TypeSpec)
		return ok && spec.Name.Name == "Box" && spec.TypeParams != nil && len(spec.TypeParams.List) == 1
	}) {
		t.Fatal("generated AST missing generic type def Box[T any]")
	}
	// The instantiation is referenced as Box[int].
	if !astFilesContain(files, func(node ast.Node) bool {
		idx, ok := node.(*ast.IndexExpr)
		if !ok {
			return false
		}
		base, ok := idx.X.(*ast.Ident)
		return ok && base.Name == "Box"
	}) {
		t.Fatal("generated AST missing Box[int] instantiation")
	}
}
func TestLowerGenericFunctionEmitsGoGeneric(t *testing.T) {
	program := lowerSource(t, `fn pair(a: $T, b: $T) [$T] {
  [a, b]
}

fn main() Int {
  let xs = pair(1, 2)
  let ys = pair("a", "b")
  xs.size() + ys.size()
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	// One generic Go function `func Pair[T any](...) []T`.
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		return ok && fn.Name.Name == "Pair" && fn.Type.TypeParams != nil && len(fn.Type.TypeParams.List) == 1
	}) {
		t.Fatal("generated AST missing generic func Pair[T any]")
	}
	// Calls are instantiated, e.g. Pair[int](...).
	count := 0
	for _, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			idx, ok := call.Fun.(*ast.IndexExpr)
			if !ok {
				return true
			}
			if base, ok := idx.X.(*ast.Ident); ok && base.Name == "Pair" {
				count++
			}
			return true
		})
	}
	if count != 2 {
		t.Fatalf("expected 2 instantiated Pair[...] calls, found %d", count)
	}
}
func TestLowerGenericStructMethodEmitsGoGenericReceiver(t *testing.T) {
	program := lowerSource(t, `struct Box {
  item: $T
}

impl Box {
  fn get() $T {
    self.item
  }
}

fn main() Int {
  let b: Box<Int> = Box{item: 42}
  b.get()
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	// A real Go generic-receiver method `func (self Box[T]) Get() T`.
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Name.Name != "Get" {
			return false
		}
		idx, ok := fn.Recv.List[0].Type.(*ast.IndexExpr)
		if !ok {
			return false
		}
		base, ok := idx.X.(*ast.Ident)
		return ok && base.Name == "Box"
	}) {
		t.Fatal("generated AST missing generic-receiver method func (self Box[T]) Get()")
	}
}

// A Go struct literal may set a func-typed field; the Ard closure passes
// through as the Go func value (Gap 2: func-typed direct-Go struct fields).
func lowerMainArdSource(t *testing.T, input string) *air.Program {
	t.Helper()
	result := parse.Parse([]byte(input), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return program
}
func TestLowerCollapsesMainArdIntoRootPackage(t *testing.T) {
	program := lowerMainArdSource(t, `fn main() {
  ()
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})

	root, ok := files["main.go"]
	if !ok {
		t.Fatal("missing root main.go")
	}
	if root.Name.Name != "main" {
		t.Fatalf("root package = %q, want main", root.Name.Name)
	}
	hasFuncMain := false
	for _, decl := range root.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == "main" {
			hasFuncMain = true
		}
	}
	if !hasFuncMain {
		t.Fatal("root main.go missing func main()")
	}
	// No separate synthetic package and no main_ rename remain.
	for name := range files {
		if strings.Contains(name, "main_") {
			t.Fatalf("unexpected main_ artifact: %s", name)
		}
	}
}
func TestLowerSynthesizesMainForNonMainEntryModule(t *testing.T) {
	// test.ard (package test) keeps the synthetic root package main importing it.
	program := lowerSource(t, `fn main() {
  ()
}`)
	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	root, ok := files["main.go"]
	if !ok || root.Name.Name != "main" {
		t.Fatal("missing synthetic root package main")
	}
	// The entry module is still its own importable package.
	if !astFilesContain(files, func(node ast.Node) bool {
		fn, ok := node.(*ast.FuncDecl)
		return ok && fn.Name.Name == "Main"
	}) {
		t.Fatal("expected exported Main in the entry module's package")
	}
}

func TestLowerProgramSupportsVoidTraitObjectDispatchWithoutStdlib(t *testing.T) {
	program := lowerSource(t, `
		trait Greet {
			fn say()
		}

		struct Cat {
			name: Str,
		}

		impl Greet for Cat {
			fn say() {
				()
			}
		}

		fn invoke(g: Greet) {
			g.say()
		}

		fn main() {
			invoke(Cat{name: "milo"})
		}
	`)

	files := lowerProgramAST(t, program, Options{PackageName: "main"})
	if !astFilesContain(files, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		return ok && strings.Contains(astCallName(call), "Cat_Greet_say")
	}) {
		t.Fatal("generated AST missing void trait dispatch call")
	}
	if astFilesContain(files, func(node ast.Node) bool {
		assign, ok := node.(*ast.AssignStmt)
		if !ok {
			return false
		}
		for _, rhs := range assign.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if ok && strings.Contains(astCallName(call), "Cat_Greet_say") {
				return true
			}
		}
		return false
	}) {
		t.Fatal("void trait dispatch call should not be assigned")
	}
}
