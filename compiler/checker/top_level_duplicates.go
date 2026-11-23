package checker

import "github.com/akonwi/ard/parse"

// reportDuplicateTopLevelDeclarations reports each top-level declaration whose
// name an earlier declaration in the module already binds (#624). Values,
// functions, and types share one namespace, and Ard has no overloading, so
// any two declarations of a name conflict whatever their kinds. Static
// functions are keyed by their `Type::name` path.
//
// The first declaration keeps the name. Later ones are skipped by every other
// pass, so references bind to the first and a duplicate's body is not checked.
//
// Platform selection (ADR 0082) has already removed the siblings that are
// inactive on the current platform, so same-named declarations for different
// platforms never reach this pass.
func (c *Checker) reportDuplicateTopLevelDeclarations() {
	seen := map[string]parse.Location{}
	for _, stmt := range c.input.Statements {
		name, loc, ok := topLevelDeclarationName(stmt)
		if !ok {
			continue
		}
		if original, duplicate := seen[name]; duplicate {
			c.addDiagnostic(duplicateDeclarationDiagnostic{
				Name:          name,
				DuplicateSpan: c.sourceSpan(loc),
				OriginalSpan:  c.sourceSpan(original),
			}.build())
			c.markConflictingTopLevelDeclaration(stmt)
			continue
		}
		seen[name] = loc
	}
}

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
