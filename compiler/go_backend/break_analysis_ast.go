package go_backend

import "github.com/akonwi/ard/checker"

func statementsContainBreak(stmts []checker.Statement) bool {
	for _, stmt := range stmts {
		if stmt.Break {
			return true
		}
		if stmt.Stmt != nil && nonProducingContainsBreak(stmt.Stmt) {
			return true
		}
		if stmt.Expr != nil && exprContainsBreak(stmt.Expr) {
			return true
		}
	}
	return false
}

func nonProducingContainsBreak(stmt checker.NonProducing) bool {
	switch s := stmt.(type) {
	case *checker.WhileLoop:
		return statementsContainBreak(s.Body.Stmts)
	case *checker.ForLoop:
		return statementsContainBreak(s.Body.Stmts)
	case *checker.ForIntRange:
		return statementsContainBreak(s.Body.Stmts)
	case *checker.ForInStr:
		return statementsContainBreak(s.Body.Stmts)
	case *checker.ForInList:
		return statementsContainBreak(s.Body.Stmts)
	case *checker.ForInMap:
		return statementsContainBreak(s.Body.Stmts)
	default:
		return false
	}
}

func exprContainsBreak(expr checker.Expression) bool {
	switch v := expr.(type) {
	case *checker.If:
		if statementsContainBreak(v.Body.Stmts) {
			return true
		}
		if v.ElseIf != nil && exprContainsBreak(v.ElseIf) {
			return true
		}
		return v.Else != nil && statementsContainBreak(v.Else.Stmts)
	case *checker.BoolMatch:
		return statementsContainBreak(v.True.Stmts) || statementsContainBreak(v.False.Stmts)
	case *checker.IntMatch:
		for _, block := range v.IntCases {
			if block != nil && statementsContainBreak(block.Stmts) {
				return true
			}
		}
		for _, block := range v.RangeCases {
			if block != nil && statementsContainBreak(block.Stmts) {
				return true
			}
		}
		return v.CatchAll != nil && statementsContainBreak(v.CatchAll.Stmts)
	case *checker.ConditionalMatch:
		for _, matchCase := range v.Cases {
			if statementsContainBreak(matchCase.Body.Stmts) {
				return true
			}
		}
		return v.CatchAll != nil && statementsContainBreak(v.CatchAll.Stmts)
	case *checker.OptionMatch:
		if v.Some != nil && v.Some.Body != nil && statementsContainBreak(v.Some.Body.Stmts) {
			return true
		}
		return v.None != nil && statementsContainBreak(v.None.Stmts)
	case *checker.ResultMatch:
		if v.Ok != nil && v.Ok.Body != nil && statementsContainBreak(v.Ok.Body.Stmts) {
			return true
		}
		return v.Err != nil && v.Err.Body != nil && statementsContainBreak(v.Err.Body.Stmts)
	case *checker.EnumMatch:
		for _, block := range v.Cases {
			if block != nil && statementsContainBreak(block.Stmts) {
				return true
			}
		}
		return v.CatchAll != nil && statementsContainBreak(v.CatchAll.Stmts)
	case *checker.UnionMatch:
		for _, matchCase := range v.TypeCases {
			if matchCase != nil && matchCase.Body != nil && statementsContainBreak(matchCase.Body.Stmts) {
				return true
			}
		}
		return v.CatchAll != nil && statementsContainBreak(v.CatchAll.Stmts)
	default:
		return false
	}
}
