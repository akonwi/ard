package checker

import "github.com/akonwi/ard/parse"

// testBodyName names the synthetic functions WrapStatementRuns creates.
const testBodyName = "__checker_test_body__"

// WrapStatementRuns moves each run of consecutive executable statements in a
// test input into its own synthetic function at the run's position. Many
// checker tests are written as bare statements for brevity, but only
// declarations may appear at the top level of a module (#533). Module-level
// `let` and `mut` stay declarations, so names resolve as they do in source
// order, and statements keep their parsed locations, so expected diagnostics
// are unchanged. Inputs that contain only declarations are returned as is.
//
// It is defined in a test file so both internal and external checker tests
// can use it without adding it to the package API.
func WrapStatementRuns(program *parse.Program) *parse.Program {
	wrapped := &parse.Program{Imports: program.Imports}
	var run *parse.FunctionDeclaration
	for _, stmt := range program.Statements {
		if IsTopLevelDeclaration(stmt) {
			run = nil
			wrapped.Statements = append(wrapped.Statements, stmt)
			continue
		}
		if run == nil {
			run = &parse.FunctionDeclaration{Name: testBodyName, Location: stmt.GetLocation()}
			wrapped.Statements = append(wrapped.Statements, run)
		}
		run.Body = append(run.Body, stmt)
		run.Location.End = stmt.GetLocation().End
	}
	return wrapped
}

// UnwrapStatementRuns replaces each synthetic function from WrapStatementRuns
// with its checked body statements, restoring source order. A statement that
// failed to check leaves an empty placeholder in a function body; it is
// dropped, matching how failed module-level statements are left out.
func UnwrapStatementRuns(statements []Statement) []Statement {
	unwrapped := make([]Statement, 0, len(statements))
	for _, stmt := range statements {
		if fn, ok := stmt.Expr.(*FunctionDef); ok && fn.Name == testBodyName {
			if fn.Body != nil {
				for _, bodyStmt := range fn.Body.Stmts {
					if bodyStmt.Expr == nil && bodyStmt.Stmt == nil && !bodyStmt.Break {
						continue
					}
					unwrapped = append(unwrapped, bodyStmt)
				}
			}
			continue
		}
		unwrapped = append(unwrapped, stmt)
	}
	return unwrapped
}
