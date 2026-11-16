package parse

// DeclarationAttributes returns the attributes written before a top-level
// declaration, or nil for statements that cannot carry them.
func DeclarationAttributes(statement Statement) []Attribute {
	switch node := statement.(type) {
	case *FunctionDeclaration:
		return node.Attributes
	case *StaticFunctionDeclaration:
		return node.Attributes
	case *VariableDeclaration:
		return node.Attributes
	case *TypeDeclaration:
		return node.Attributes
	case *StructDefinition:
		return node.Attributes
	case *EnumDefinition:
		return node.Attributes
	case *TraitDefinition:
		return node.Attributes
	case *ImplBlock:
		return node.Attributes
	case *TraitImplementation:
		return node.Attributes
	}
	return nil
}

// setDeclarationAttributes attaches attributes to a top-level declaration. It
// reports false when the statement is not a declaration.
func setDeclarationAttributes(statement Statement, attributes []Attribute) bool {
	switch node := statement.(type) {
	case *FunctionDeclaration:
		node.Attributes = attributes
	case *StaticFunctionDeclaration:
		node.Attributes = attributes
	case *VariableDeclaration:
		node.Attributes = attributes
	case *TypeDeclaration:
		node.Attributes = attributes
	case *StructDefinition:
		node.Attributes = attributes
	case *EnumDefinition:
		node.Attributes = attributes
	case *TraitDefinition:
		node.Attributes = attributes
	case *ImplBlock:
		node.Attributes = attributes
	case *TraitImplementation:
		node.Attributes = attributes
	default:
		return false
	}
	return true
}

// DeclarationStart is where a top-level declaration's source begins,
// including the attributes written before it.
func DeclarationStart(statement Statement) Point {
	if attributes := DeclarationAttributes(statement); len(attributes) > 0 {
		return attributes[0].Location.Start
	}
	if statement == nil {
		return Point{}
	}
	return statement.GetLocation().Start
}
