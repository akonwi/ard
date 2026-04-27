package go_backend

import (
	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
	"github.com/akonwi/ard/go_backend/lowering"
)

func lowerModuleToBackendIR(module checker.Module, packageName string, entrypoint bool) (*backendir.Module, error) {
	return lowering.LowerModuleToBackendIR(module, packageName, entrypoint, "")
}

func lowerExpressionToBackendIR(expr checker.Expression) backendir.Expr {
	return lowering.LowerExpressionToBackendIR(expr)
}

func lowerCheckerTypeToBackendIR(t checker.Type) backendir.Type {
	return lowering.LowerCheckerTypeToBackendIR(t)
}

func lowerFunctionDeclToBackendIR(def *checker.FunctionDef) backendir.Decl {
	return lowering.LowerFunctionDeclToBackendIR(def)
}

func lowerUnionDeclToBackendIR(def *checker.Union) backendir.Decl {
	return lowering.LowerUnionDeclToBackendIR(def)
}

func lowerExternTypeDeclToBackendIR(def *checker.ExternType) backendir.Decl {
	return lowering.LowerExternTypeDeclToBackendIR(def)
}

func lowerNonProducingToBackendIR(node checker.NonProducing) []backendir.Stmt {
	return lowering.LowerNonProducingToBackendIR(node)
}

func matchSubjectTempName(kind string) string {
	return lowering.MatchSubjectTempName(kind)
}

func topLevelExecutableStatements(stmts []checker.Statement) []checker.Statement {
	filtered := make([]checker.Statement, 0, len(stmts))
	for _, stmt := range stmts {
		switch stmt.Expr.(type) {
		case *checker.FunctionDef, *checker.ExternalFunctionDef:
			continue
		}
		switch stmt.Stmt.(type) {
		case *checker.StructDef, checker.StructDef,
			*checker.Enum, checker.Enum,
			*checker.Union, checker.Union,
			*checker.ExternType:
			continue
		}
		filtered = append(filtered, stmt)
	}
	return filtered
}
