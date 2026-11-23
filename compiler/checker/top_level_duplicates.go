package checker

import "github.com/akonwi/ard/compiler/parse"

// topLevelDeclarationName is the name a top-level declaration binds and the
// location of that name.
func topLevelDeclarationName(stmt parse.Statement) (string, parse.Location, bool) {
	if name, loc, ok := topLevelTypeDeclarationName(stmt); ok {
		return name, loc, true
	}
	var name string
	var loc, fallback parse.Location
	switch s := stmt.(type) {
	case *parse.FunctionDeclaration:
		name, loc, fallback = s.Name, s.NameLocation, s.GetLocation()
	case *parse.StaticFunctionDeclaration:
		name, loc, fallback = s.Path.String(), s.NameLocation, s.Path.GetLocation()
	case *parse.VariableDeclaration:
		name, loc, fallback = s.Name, s.NameLocation, s.GetLocation()
	default:
		return "", parse.Location{}, false
	}
	if name == "" || name == "_" {
		return "", parse.Location{}, false
	}
	if loc == (parse.Location{}) {
		loc = fallback
	}
	return name, loc, true
}

// markConflictingTopLevelDeclaration records a top-level declaration whose
// name is already taken, by an earlier declaration or a builtin type. It
// binds nothing, and no pass checks it.
func (c *Checker) markConflictingTopLevelDeclaration(stmt parse.Statement) {
	if c.nameConflictingTopLevelStatements == nil {
		c.nameConflictingTopLevelStatements = map[parse.Statement]bool{}
	}
	c.nameConflictingTopLevelStatements[stmt] = true
}

func (c *Checker) isConflictingTopLevelDeclaration(stmt parse.Statement) bool {
	return c.nameConflictingTopLevelStatements != nil && c.nameConflictingTopLevelStatements[stmt]
}
