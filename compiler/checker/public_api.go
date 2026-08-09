package checker

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/akonwi/ard/parse"
)

// validatePublicAPI ensures every type exposed by this module can be named by
// importing modules. Public nominal declarations are trusted at their boundary:
// their owning module validates their own fields and signatures separately.
func (c *Checker) validatePublicAPI() {
	locations := publicDeclarationLocations(c.input)
	fieldLocations := publicFieldLocations(c.input)
	aliases := publicTypeAliases(c.input)
	variables := topLevelVariableNames(c.input)

	for _, name := range sortedBoolMapKeys(aliases) {
		public := aliases[name]
		if public {
			if symbol := c.scope.symbols[name]; symbol != nil {
				c.validatePublicType("public type alias `"+name+"`", symbol.Type, locations[name])
			}
		}
	}

	for _, name := range sortedSymbolNames(c.scope.symbols) {
		symbol := c.scope.symbols[name]
		if _, alias := aliases[name]; alias {
			continue
		}
		if variables[name] {
			continue
		}
		loc := locations[name]
		switch declaration := symbol.Type.(type) {
		case *FunctionDef:
			if !declaration.Private && !declaration.IsTest {
				c.validatePublicFunction("function `"+name+"`", declaration, loc)
			}
		case *StructDef:
			canonical := canonicalStructDefinition(declaration)
			if canonical.Private || canonical.ModulePath != "" && canonical.ModulePath != c.modulePath {
				continue
			}
			for _, field := range sortedTypeMapKeys(canonical.Fields) {
				fieldLoc := loc
				if byField := fieldLocations[canonical.Name]; byField != nil {
					if specific, ok := byField[field]; ok {
						fieldLoc = specific
					}
				}
				c.validatePublicType(fmt.Sprintf("public field `%s`", field), canonical.Fields[field], fieldLoc)
			}
			owner := StructMethodOwner(canonical)
			for _, methodName := range sortedFunctionMapKeys(c.program.StructMethods[owner]) {
				method := c.program.StructMethods[owner][methodName]
				if !method.Private {
					c.validatePublicFunction(fmt.Sprintf("public method `%s.%s`", canonical.Name, methodName), method, loc)
				}
			}
		case *Trait:
			if declaration.private || declaration.ModulePath != "" && declaration.ModulePath != c.modulePath {
				continue
			}
			for i := range declaration.methods {
				method := &declaration.methods[i]
				c.validatePublicFunction(fmt.Sprintf("public trait method `%s.%s`", declaration.Name, method.Name), method, loc)
			}
		case *Enum:
			if declaration.Private || declaration.ModulePath != "" && declaration.ModulePath != c.modulePath {
				continue
			}
			for _, methodName := range sortedFunctionMapKeys(declaration.Methods) {
				method := declaration.Methods[methodName]
				if !method.Private {
					c.validatePublicFunction(fmt.Sprintf("public method `%s.%s`", declaration.Name, methodName), method, loc)
				}
			}
		case *Union:
			if declaration.Private || declaration.ModulePath != "" && declaration.ModulePath != c.modulePath {
				continue
			}
			for _, member := range declaration.Types {
				c.validatePublicType("public union `"+declaration.Name+"`", member, loc)
			}
		}
	}

	for _, statement := range c.program.Statements {
		variable, ok := statement.Stmt.(*VariableDef)
		if !ok || variable.Mutable {
			continue
		}
		c.validatePublicType("public variable `"+variable.Name+"`", variable.__type, locations[variable.Name])
	}
}

func (c *Checker) validatePublicFunction(context string, function *FunctionDef, loc parse.Location) {
	for _, parameter := range function.Parameters {
		parameterLoc := loc
		if parameter.Loc != (parse.Location{}) {
			parameterLoc = parameter.Loc
		}
		c.validatePublicType(fmt.Sprintf("%s parameter `%s`", context, parameter.Name), parameter.Type, parameterLoc)
	}
	c.validatePublicType(context+" return type", function.ReturnType, loc)
}

func (c *Checker) validatePublicType(context string, exposed Type, loc parse.Location) {
	if privateName := privateNominalType(exposed, map[uintptr]bool{}); privateName != "" {
		message := fmt.Sprintf("%s exposes private type `%s`", context, privateName)
		c.addDiagnostic(Diagnostic{
			Kind:    Error,
			Code:    DiagnosticCodePrivateTypeExposure,
			Message: message,
			Title:   "Public API exposes private type",
			Text:    "Private types cannot appear in declarations visible outside their module.",
			Primary: DiagnosticLabel{Span: c.sourceSpan(loc), Message: message},
		})
	}
}

func privateNominalType(current Type, seen map[uintptr]bool) string {
	if current == nil {
		return ""
	}
	value := reflect.ValueOf(current)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		identity := value.Pointer()
		if seen[identity] {
			return ""
		}
		seen[identity] = true
	}

	switch value := current.(type) {
	case *MutableRef:
		return privateNominalType(value.of, seen)
	case *Maybe:
		return privateNominalType(value.of, seen)
	case *Result:
		if name := privateNominalType(value.val, seen); name != "" {
			return name
		}
		return privateNominalType(value.err, seen)
	case *List:
		return privateNominalType(value.of, seen)
	case *Slice:
		return privateNominalType(value.of, seen)
	case *FixedArray:
		return privateNominalType(value.of, seen)
	case *Chan:
		return privateNominalType(value.of, seen)
	case *Receiver:
		return privateNominalType(value.of, seen)
	case *Sender:
		return privateNominalType(value.of, seen)
	case *Map:
		if name := privateNominalType(value.key, seen); name != "" {
			return name
		}
		return privateNominalType(value.value, seen)
	case *FunctionDef:
		for _, parameter := range value.Parameters {
			if name := privateNominalType(parameter.Type, seen); name != "" {
				return name
			}
		}
		return privateNominalType(value.ReturnType, seen)
	case *TypeVar:
		return privateNominalType(value.actual, seen)
	case *StructDef:
		canonical := canonicalStructDefinition(value)
		if canonical.Private {
			return canonical.Name
		}
		for _, argument := range value.TypeArgs {
			if name := privateNominalType(argument, seen); name != "" {
				return name
			}
		}
		return ""
	case *Enum:
		if value.Private {
			return value.Name
		}
		return ""
	case *Union:
		if value.Private {
			return value.Name
		}
		return ""
	case *Trait:
		if value.private {
			return value.Name
		}
		return ""
	case *ForeignType:
		for _, argument := range value.TypeArgs {
			if name := privateNominalType(argument, seen); name != "" {
				return name
			}
		}
	}
	return ""
}

func sortedBoolMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSymbolNames(values map[string]*Symbol) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedTypeMapKeys(values map[string]Type) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedFunctionMapKeys(values map[string]*FunctionDef) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func topLevelVariableNames(program *parse.Program) map[string]bool {
	variables := map[string]bool{}
	if program == nil {
		return variables
	}
	for _, statement := range program.Statements {
		switch declaration := statement.(type) {
		case parse.VariableDeclaration:
			variables[declaration.Name] = true
		case *parse.VariableDeclaration:
			variables[declaration.Name] = true
		}
	}
	return variables
}

func publicFieldLocations(program *parse.Program) map[string]map[string]parse.Location {
	locations := map[string]map[string]parse.Location{}
	if program == nil {
		return locations
	}
	record := func(declaration parse.StructDefinition) {
		fields := map[string]parse.Location{}
		for _, field := range declaration.Fields {
			if field.Type != nil {
				fields[field.Name.Name] = field.Type.GetLocation()
			}
		}
		locations[declaration.Name.Name] = fields
	}
	for _, statement := range program.Statements {
		switch declaration := statement.(type) {
		case parse.StructDefinition:
			record(declaration)
		case *parse.StructDefinition:
			record(*declaration)
		}
	}
	return locations
}

func publicTypeAliases(program *parse.Program) map[string]bool {
	aliases := map[string]bool{}
	if program == nil {
		return aliases
	}
	for _, statement := range program.Statements {
		switch declaration := statement.(type) {
		case parse.TypeDeclaration:
			if len(declaration.Type) == 1 {
				aliases[declaration.Name.Name] = !declaration.Private
			}
		case *parse.TypeDeclaration:
			if len(declaration.Type) == 1 {
				aliases[declaration.Name.Name] = !declaration.Private
			}
		}
	}
	return aliases
}

func publicDeclarationLocations(program *parse.Program) map[string]parse.Location {
	locations := map[string]parse.Location{}
	if program == nil {
		return locations
	}
	for _, statement := range program.Statements {
		switch declaration := statement.(type) {
		case parse.FunctionDeclaration:
			locations[declaration.Name] = declaration.Location
		case *parse.FunctionDeclaration:
			locations[declaration.Name] = declaration.Location
		case parse.StaticFunctionDeclaration:
			locations[declaration.Path.String()] = declaration.Location
		case *parse.StaticFunctionDeclaration:
			locations[declaration.Path.String()] = declaration.Location
		case parse.StructDefinition:
			locations[declaration.Name.Name] = declaration.Location
		case *parse.StructDefinition:
			locations[declaration.Name.Name] = declaration.Location
		case parse.TraitDefinition:
			locations[declaration.Name.Name] = declaration.Location
		case *parse.TraitDefinition:
			locations[declaration.Name.Name] = declaration.Location
		case parse.EnumDefinition:
			locations[declaration.Name] = declaration.Location
		case *parse.EnumDefinition:
			locations[declaration.Name] = declaration.Location
		case parse.TypeDeclaration:
			locations[declaration.Name.Name] = declaration.Location
		case *parse.TypeDeclaration:
			locations[declaration.Name.Name] = declaration.Location
		case parse.VariableDeclaration:
			locations[declaration.Name] = declaration.Location
		case *parse.VariableDeclaration:
			locations[declaration.Name] = declaration.Location
		}
	}
	return locations
}
