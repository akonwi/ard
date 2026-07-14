package checker

import (
	"fmt"
	gotypes "go/types"
	"maps"
	"math"
	"math/big"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
)

type Program struct {
	Imports               map[string]Module
	GoImports             map[string]*GoPackage
	Statements            []Statement
	StructMethods         map[MethodOwner]map[string]*FunctionDef
	ForeignInterfaceImpls map[MethodOwner][]*ForeignType
}

type Module interface {
	Path() string
	Get(name string) Symbol
	Program() *Program
	// Symbols returns the module's public symbols by name. The map is owned
	// by the module and must be treated as read-only.
	Symbols() map[string]Symbol
}

// deref follows TypeVar bindings to find the concrete type.
// Used during type unification to ensure we see resolved types.
// Only dereferences a single type node; for compound types use derefType.
//
// Example: If $T is bound to Int, deref($T) returns Int.
// If $T is bound to [$U], deref($T) returns [$U] (not the resolved contents).
func deref(t Type) Type {
	if typeVar, ok := t.(*TypeVar); ok && typeVar.bound {
		if typeVar.actual == t {
			return t // break self-referential cycle
		}
		return deref(typeVar.actual) // Recursively follow chains
	}
	return t
}

// derefType recursively dereferences a type, including through compound types.
// Walks the entire type tree, dereferencing TypeVar at each level.
// Used when parameter types must reflect bound generics before checking arguments.
// Only creates new instances when inner types actually change.
//
// Example: If $T is bound to Int, derefType([$T]) returns [Int].
// Ensures anonymous function parameters see fully resolved types.
func derefType(t Type) Type {
	return derefTypeSeen(t, map[Type]bool{})
}

// derefTypeSeen guards against cycles in named type graphs, such as a struct
// with an impl method returning the same struct type. Longer term, methods
// should probably live outside StructDef's type identity so value shape and
// method namespace cannot recursively contain each other.
func derefTypeSeen(t Type, seen map[Type]bool) Type {
	t = deref(t) // First dereference at the top level
	if t == nil {
		return nil
	}
	if seen[t] {
		return t
	}
	seen[t] = true
	switch typ := t.(type) {
	case *List:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &List{of: derefInner}
	case *Chan:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Chan{of: derefInner}
	case *Receiver:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Receiver{of: derefInner}
	case *Sender:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return &Sender{of: derefInner}
	case *Map:
		derefKey := derefTypeSeen(typ.key, seen)
		derefVal := derefTypeSeen(typ.value, seen)
		if derefKey == typ.key && derefVal == typ.value {
			return typ // No change, return original
		}
		return &Map{
			key:   derefKey,
			value: derefVal,
		}
	case *Maybe:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ // No change, return original
		}
		return &Maybe{of: derefInner}
	case *Result:
		derefVal := derefTypeSeen(typ.val, seen)
		derefErr := derefTypeSeen(typ.err, seen)
		if derefVal == typ.val && derefErr == typ.err {
			return typ // No change, return original
		}
		return &Result{
			val: derefVal,
			err: derefErr,
		}
	case *MutableRef:
		derefInner := derefTypeSeen(typ.of, seen)
		if derefInner == typ.of {
			return typ
		}
		return MakeMutableRef(derefInner)
	case *Union:
		newTypes := make([]Type, len(typ.Types))
		changed := false
		for i, t := range typ.Types {
			newTypes[i] = derefTypeSeen(t, seen)
			if newTypes[i] != typ.Types[i] {
				changed = true
			}
		}
		if !changed {
			return typ // No change, return original
		}
		return &Union{
			Name:       typ.Name,
			ModulePath: typ.ModulePath,
			Types:      newTypes,
			Private:    typ.Private,
		}
	case *StructDef:
		fieldsChanged := false
		newFields := make(map[string]Type, len(typ.Fields))
		for name, fieldType := range typ.Fields {
			derefFieldType := derefTypeSeen(fieldType, seen)
			newFields[name] = derefFieldType
			if derefFieldType != fieldType {
				fieldsChanged = true
			}
		}
		newTypeArgs := make([]Type, len(typ.TypeArgs))
		typeArgsChanged := false
		for i, typeArg := range typ.TypeArgs {
			newTypeArgs[i] = derefTypeSeen(typeArg, seen)
			if newTypeArgs[i] != typeArg {
				typeArgsChanged = true
			}
		}
		if !fieldsChanged && !typeArgsChanged {
			return typ
		}
		return &StructDef{
			Name:             typ.Name,
			ModulePath:       typ.ModulePath,
			Fields:           newFields,
			Self:             typ.Self,
			Traits:           typ.Traits,
			GenericParams:    append([]string(nil), typ.GenericParams...),
			DeclaredGenerics: typ.DeclaredGenerics,
			TypeArgs:         newTypeArgs,
			Private:          typ.Private,
		}
	case *FunctionDef:
		newParams := make([]Parameter, len(typ.Parameters))
		paramsChanged := false
		for i, param := range typ.Parameters {
			derefParamType := derefTypeSeen(param.Type, seen)
			newParams[i] = param
			newParams[i].Type = derefParamType
			if derefParamType != param.Type {
				paramsChanged = true
			}
		}
		derefReturnType := derefTypeSeen(typ.ReturnType, seen)
		returnChanged := derefReturnType != typ.ReturnType
		if !paramsChanged && !returnChanged {
			return typ // No change, return original
		}
		return &FunctionDef{
			Name:                    typ.Name,
			GenericParams:           append([]string(nil), typ.GenericParams...),
			Parameters:              newParams,
			ReturnType:              derefReturnType,
			InferReturnTypeFromBody: typ.InferReturnTypeFromBody,
			Body:                    typ.Body,
			Mutates:                 typ.Mutates,
			IsTest:                  typ.IsTest,
			Private:                 typ.Private,
			GenericBindings:         cloneTypeMap(typ.GenericBindings),
		}
	default:
		return t
	}
}

// referenceArgType returns the type used when matching an argument against a
// parameter, preserving a mutable-reference field's reference type. Reading a
// `mut T` field deref's to its value type `T` (write-through semantics); this
// recovers the underlying `mut T` so a stored handle (a mutable lvalue) can be
// borrowed back into `mut T` at a call site (ADR 0031).
func referenceArgType(expr Expression) Type {
	if ip, ok := expr.(*InstanceProperty); ok {
		return ip._type
	}
	return expr.Type()
}

// checkMutRef checks the explicit `mut <operand>` expression (ADR 0045).
func (c *Checker) checkMutRef(s *parse.MutRef) Expression {
	operand := c.checkExpr(s.Operand)
	if operand == nil {
		return nil
	}
	// A place whose storage is already a reference aliases: the result is
	// another reference to the same referent. Reads see through references,
	// so `mut` is the only spelling that propagates one (ADR 0045).
	fresh := false
	switch operand.(type) {
	case *Variable, *InstanceProperty, *ForeignFieldAccess:
		if !c.isMutable(operand) {
			c.addDiagnostic(immutableMutableReferenceDiagnostic{
				Place:           mutRefPlaceName(operand),
				Span:            c.sourceSpan(s.Operand.GetLocation()),
				DeclarationSpan: expressionBindingSpan(operand),
			}.build())
			return nil
		}
	default:
		// A value expression materializes fresh mutable storage; the
		// reference points at it, equivalent to binding a mut local first.
		fresh = true
	}
	var refType Type
	if foreign, ok := operand.Type().(*ForeignType); ok {
		if foreign.Pointer {
			// The place already holds a reference; aliasing copies it.
			refType = foreign
		} else if pointer := foreign.PointerForm(); pointer != nil {
			refType = pointer
		} else {
			c.addDiagnostic(unsupportedMutableReferenceDiagnostic{
				Type: foreign,
				Span: c.sourceSpan(s.GetLocation()),
			}.build())
			return nil
		}
	} else {
		refType = MakeMutableRef(operand.Type())
	}
	return &MutableRefExpr{Operand: operand, Fresh: fresh, _type: refType}
}

func expressionBindingSpan(expr Expression) *SourceSpan {
	var span SourceSpan
	switch e := expr.(type) {
	case *Variable:
		span = e.sym.declaredAt
	case Variable:
		span = e.sym.declaredAt
	case *InstanceProperty:
		return expressionBindingSpan(e.Subject)
	case *ForeignFieldAccess:
		return expressionBindingSpan(e.Subject)
	case *MutableRefExpr:
		return expressionBindingSpan(e.Operand)
	}
	if span.FilePath == "" {
		return nil
	}
	return &span
}

func (c Checker) isMutable(expr Expression) bool {
	switch e := expr.(type) {
	case *MutableRefExpr:
		return true
	case *Variable:
		// A value whose type is a mutable reference is itself mutable through the
		// reference, regardless of whether the binding is reassignable (ADR 0031).
		// Mirrors the InstanceProperty case so a `mut T` value is usable wherever a
		// `mut T` is expected (fields, params, mutating methods).
		if _, ok := mutableRefBase(e.sym.Type); ok {
			return true
		}
		if isPointerForeign(e.sym.Type) {
			return true
		}
		return e.sym.mutable
	case *InstanceProperty:
		if _, ok := mutableRefBase(e._type); ok {
			return true
		}
		if isPointerForeign(e._type) {
			return true
		}
		return c.isMutable(e.Subject)
	case *ForeignFieldAccess:
		return c.isMutable(e.Subject)
	}
	return false
}

type Checker struct {
	diagnostics                       []Diagnostic
	input                             *parse.Program
	scope                             *SymbolTable
	filePath                          string
	modulePath                        string
	program                           *Program
	halted                            bool
	moduleResolver                    *ModuleResolver
	options                           CheckOptions
	expectedExpr                      Type
	duplicateTopLevelTypeDeclarations map[parse.Statement]bool
	topLevelStructDeclarations        map[string]*parse.StructDefinition
	topLevelTypeAliases               map[string]*parse.TypeDeclaration
	hoistedTopLevelFunctions          map[*parse.FunctionDeclaration]*FunctionDef
	resolvingTopLevelStructs          map[string]bool
	resolvedTopLevelStructs           map[string]bool
	resolvingTopLevelAliases          map[string]bool
	resolvingTopLevelAliasEdges       []typeAliasResolutionEdge
	resolvingTopLevelAliasNames       []string
	recursiveTopLevelAliases          map[string]bool
	resolvedTopLevelAliases           map[string]bool
	genericContextStack               []map[string]bool
	methodGenericAllowlist            []map[string]bool
	discardExprContext                bool
	matchArmDiscardContext            bool
	deferredWorkDepth                 int
	reportedMapKeyErrors              map[parse.Location]bool
	emptyCollectionBinding            *collectionBindingContext
	goTypesContext                    *gotypes.Context
	spans                             *SpanIndex
	moduleFiles                       map[string]string
}

func New(filePath string, input *parse.Program, moduleResolver *ModuleResolver, options ...CheckOptions) *Checker {
	rootScope := makeScope(nil)
	checkOptions := normalizeCheckOptions(options)
	modulePath := filePath
	if checkOptions.ModulePath != "" {
		modulePath = checkOptions.ModulePath
	}
	c := &Checker{
		diagnostics:    []Diagnostic{},
		input:          input,
		filePath:       filePath,
		modulePath:     modulePath,
		moduleResolver: moduleResolver,
		options:        checkOptions,
		program: &Program{
			Imports:               map[string]Module{},
			GoImports:             map[string]*GoPackage{},
			Statements:            []Statement{},
			StructMethods:         map[MethodOwner]map[string]*FunctionDef{},
			ForeignInterfaceImpls: map[MethodOwner][]*ForeignType{},
		},
		scope:          &rootScope,
		goTypesContext: gotypes.NewContext(),
	}
	if checkOptions.RecordSpans {
		c.spans = &SpanIndex{}
		c.moduleFiles = map[string]string{}
	}

	return c
}

func (c *Checker) typeOwnerPath() string {
	if c.moduleResolver == nil && !strings.HasPrefix(c.modulePath, "ard/") {
		return ""
	}
	return c.modulePath
}

func (c *Checker) HasErrors() bool {
	return len(c.diagnostics) > 0
}

func (c *Checker) Diagnostics() []Diagnostic {
	return c.diagnostics
}

func isTopLevelExecutableStatement(stmt parse.Statement) bool {
	if isTopLevelTypeDeclaration(stmt) {
		return false
	}
	switch stmt.(type) {
	case *parse.FunctionDeclaration, *parse.StaticFunctionDeclaration, *parse.VariableDeclaration:
		return false
	default:
		return true
	}
}

func (c *Checker) Check() {
	c.primeGoResolver()
	seenImportAliases := map[string]parse.Location{}
	for _, imp := range c.input.Imports {
		if original, dup := seenImportAliases[imp.Name]; dup {
			c.addDiagnostic(duplicateImportDiagnostic{
				Name:           imp.Name,
				StatementStart: imp.GetStart(),
				DuplicateSpan:  c.sourceSpan(imp.PathLocation),
				OriginalSpan:   c.sourceSpan(original),
			}.build())
			continue
		}
		seenImportAliases[imp.Name] = imp.PathLocation

		if imp.Kind == parse.ImportKindGo {
			resolver := c.options.GoResolver
			if resolver == nil {
				resolver = ImporterGoPackageResolver{}
			}
			pkg, err := resolver.ResolveGoPackage(imp.Path)
			if err != nil {
				c.addDiagnostic(goImportResolutionDiagnostic{
					Path:  imp.Path,
					Cause: err.Error(),
					Span:  c.sourceSpan(imp.PathLocation),
				}.build())
				continue
			}
			c.program.GoImports[imp.Name] = pkg
			continue
		}

		if strings.HasPrefix(imp.Path, "ard/") {
			// Handle standard library imports
			if mod, ok := findInStdLib(imp.Path); ok {
				c.program.Imports[imp.Name] = mod
			} else {
				c.addUnresolvedReference(unknownModule, imp.Path, imp.GetLocation())
			}
		} else {
			// Handle user module imports
			if c.moduleResolver == nil {
				panic(fmt.Sprintf("No module resolver provided for user import: %s", imp.Path))
			}

			resolved, err := c.moduleResolver.ResolveImport(c.modulePath, imp.Path)
			if err != nil {
				c.addDiagnostic(ardImportResolutionDiagnostic{
					Path:  imp.Path,
					Cause: err.Error(),
					Span:  c.sourceSpan(imp.PathLocation),
				}.build())
				continue
			}
			filePath := filepath.Clean(resolved.FilePath)

			// Check if module is already cached
			if cachedModule, ok := c.moduleResolver.moduleCache[filePath]; ok {
				c.program.Imports[imp.Name] = cachedModule
				continue
			}
			if slices.Contains(c.moduleResolver.loadingChain, resolved.ModulePath) {
				chain := append(append([]string{}, c.moduleResolver.loadingChain...), resolved.ModulePath)
				c.addDiagnostic(circularImportDiagnostic{
					Chain:       chain,
					ClosingSpan: c.sourceSpan(imp.PathLocation),
				}.build())
				continue
			}
			c.moduleResolver.loadingChain = append(c.moduleResolver.loadingChain, resolved.ModulePath)

			// Load and parse the module file using the resolved package context.
			ast, err := c.moduleResolver.LoadModuleFile(filePath)
			if err != nil {
				c.moduleResolver.loadingChain = c.moduleResolver.loadingChain[:len(c.moduleResolver.loadingChain)-1]
				c.addDiagnostic(moduleLoadDiagnostic{
					ImportPath: imp.Path,
					TargetFile: filePath,
					Cause:      err.Error(),
					ImportSpan: c.sourceSpan(imp.PathLocation),
				}.build())
				continue
			}

			// Type-check the imported module
			importOptions := c.options
			userModule, diagnostics := check(ast, c.moduleResolver, filePath, resolved.ModulePath, importOptions)
			c.moduleResolver.loadingChain = c.moduleResolver.loadingChain[:len(c.moduleResolver.loadingChain)-1]
			if len(diagnostics) > 0 {
				// Add all diagnostics from the imported module
				for _, diag := range diagnostics {
					diag = reanchorCircularImportDiagnostic(diag, c.sourceSpan(imp.PathLocation))
					c.diagnostics = append(c.diagnostics, diag)
				}
				continue
			}

			// Set the correct module path for the module
			if um, ok := userModule.(*UserModule); ok {
				um.setFilePath(resolved.ModulePath)
			}
			if c.moduleFiles != nil {
				c.moduleFiles[resolved.ModulePath] = filePath
			}

			// Cache and add to imports
			c.moduleResolver.moduleCache[filePath] = userModule
			c.program.Imports[imp.Name] = userModule
		}
	}

	// Auto-import prelude modules (only for non-std lib)
	if !strings.HasPrefix(c.filePath, "ard/") {
		if mod, ok := findInStdLib("ard/int"); ok {
			c.program.Imports["Int"] = mod
		}
		if mod, ok := findInStdLib("ard/byte"); ok {
			c.program.Imports["Byte"] = mod
		}
		if mod, ok := findInStdLib("ard/rune"); ok {
			c.program.Imports["Rune"] = mod
		}
		if mod, ok := findInStdLib("ard/list"); ok {
			c.program.Imports["List"] = mod
		}
		if mod, ok := findInStdLib("ard/map"); ok {
			c.program.Imports["Map"] = mod
		}
		if mod, ok := findInStdLib("ard/string"); ok {
			c.program.Imports["Str"] = mod
		}
	}

	c.hoistTopLevelTypeDeclarations()
	c.predeclareTopLevelTypeAliases()
	c.populateTopLevelTypeDefinitions()
	c.hoistTopLevelFunctionSignatures()

	for i := range c.input.Statements {
		if stmt := c.checkedTopLevelTypeStatement(c.input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
			continue
		}
		if isTopLevelTypeDeclaration(c.input.Statements[i]) {
			continue
		}
		if isTopLevelExecutableStatement(c.input.Statements[i]) {
			previousScript := c.scope.inScript
			c.scope.inScript = true
			stmt := c.checkStmt(&c.input.Statements[i])
			c.scope.inScript = previousScript
			if stmt != nil {
				c.program.Statements = append(c.program.Statements, *stmt)
			}
		} else if stmt := c.checkStmt(&c.input.Statements[i]); stmt != nil {
			c.program.Statements = append(c.program.Statements, *stmt)
		}
		if c.halted {
			break
		}
	}

	c.validateTopLevelTypeAliases()
	c.checkRecursiveStructLayouts()

	// now that we're done with the aliases, use module paths for the import keys
	for alias, mod := range c.program.Imports {
		delete(c.program.Imports, alias)
		c.program.Imports[mod.Path()] = mod
	}
}

func (c *Checker) scanForUnresolvedGenerics() {
	for _, stmt := range c.program.Statements {
		if stmt.Expr == nil {
			continue
		}

		if typeVar, ok := stmt.Expr.Type().(*TypeVar); ok && typeVar.actual == nil {
			loc := parse.Location{}
			if locatable, ok := stmt.Expr.(interface{ GetLocation() parse.Location }); ok {
				loc = locatable.GetLocation()
			}

			c.addDiagnostic(unresolvedGenericDiagnostic{Generic: typeVar.String(), Span: c.sourceSpan(loc)}.build())
			break
		}
	}
}

// This should only be called after .Check()
// The returned module could be problematic if there are diagnostic errors.
func (c *Checker) Module() Module {
	return NewUserModule(c.modulePath, c.program, c.scope)
}

// check is an internal helper for recursive module checking.
// Use New() + Check() + Module() for the public API.
func check(input *parse.Program, moduleResolver *ModuleResolver, filePath string, modulePath string, options CheckOptions) (Module, []Diagnostic) {
	c := New(filePath, input, moduleResolver, options)
	c.modulePath = modulePath

	c.Check()

	return c.Module(), c.diagnostics
}

func (c *Checker) addError(msg string, location parse.Location) {
	c.diagnostics = append(c.diagnostics, NewDiagnostic(Error, msg, c.filePath, location))
}

func (c *Checker) addInvalidMatchPattern(message string, location parse.Location, label string) {
	c.addDiagnostic(invalidMatchPatternDiagnostic{LegacyMessage: message, Span: c.sourceSpan(location), Label: label}.build())
}

func (c *Checker) addInvalidForeignTypePattern(message string, location parse.Location, label string) {
	c.addDiagnostic(invalidForeignTypePatternDiagnostic{LegacyMessage: message, Span: c.sourceSpan(location), Label: label}.build())
}

func (c *Checker) addDuplicateMatchArm(kind DiagnosticKind, message string, location parse.Location, original *SourceSpan) {
	c.addDiagnostic(duplicateMatchArmDiagnostic{Kind: kind, LegacyMessage: message, Span: c.sourceSpan(location), OriginalSpan: original}.build())
}

func (c *Checker) addNonExhaustiveMatch(message string, location parse.Location, label string) {
	c.addDiagnostic(nonExhaustiveMatchDiagnostic{LegacyMessage: message, Span: c.sourceSpan(location), Label: label}.build())
}

func (c *Checker) addInvalidSelectArm(message string, location parse.Location, label string) {
	c.addDiagnostic(invalidSelectArmDiagnostic{LegacyMessage: message, Span: c.sourceSpan(location), Label: label}.build())
}

func (c *Checker) addWarning(msg string, location parse.Location) {
	c.diagnostics = append(c.diagnostics, NewDiagnostic(Warn, msg, c.filePath, location))
}

func (c *Checker) addDiagnostic(diagnostic Diagnostic) {
	c.diagnostics = append(c.diagnostics, diagnostic)
}

func (c *Checker) sourceSpan(location parse.Location) SourceSpan {
	return SourceSpan{FilePath: c.filePath, Location: location}
}

func (c *Checker) sourceSpanPtr(location parse.Location) *SourceSpan {
	span := c.sourceSpan(location)
	return &span
}

func sourceSpanIfPresent(span SourceSpan) *SourceSpan {
	if span.FilePath == "" {
		return nil
	}
	return &span
}

func declaredTypeLocation(declared parse.DeclaredType, fallback parse.Location) parse.Location {
	if declared == nil {
		return fallback
	}
	return declared.GetLocation()
}

func (c *Checker) addNonCallable(name string, location parse.Location, declaration *SourceSpan, style nonCallableLegacyStyle) {
	c.addDiagnostic(nonCallableDiagnostic{Name: name, Span: c.sourceSpan(location), DeclarationSpan: declaration, LegacyStyle: style}.build())
}

func (c *Checker) addArgumentCount(expected string, actual int, location parse.Location, legacyMessage string) {
	c.addDiagnostic(argumentCountDiagnostic{Expected: expected, Actual: actual, Span: c.sourceSpan(location), LegacyMessage: legacyMessage}.build())
}

func (c *Checker) addMissingArgument(parameter Parameter, location parse.Location) {
	c.addDiagnostic(missingArgumentDiagnostic{Parameter: parameter, Span: c.sourceSpan(location)}.build())
}

func (c *Checker) addUnknownNamedArgument(name string, location parse.Location, legacyMessage string) {
	c.addDiagnostic(argumentBindingDiagnostic{Kind: unknownNamedArgument, Name: name, Span: c.sourceSpan(location), LegacyMessage: legacyMessage}.build())
}

func (c *Checker) addNamedArgumentsUnsupported(targetKind string, location parse.Location) {
	c.addDiagnostic(namedArgumentsUnsupportedDiagnostic{TargetKind: targetKind, Span: c.sourceSpan(location)}.build())
}

func (c *Checker) addInvalidFunctionTypeArguments(name string, expected int, actual int, takesTypeArgs bool, location parse.Location, legacyMessage string) {
	c.addDiagnostic(invalidFunctionTypeArgumentsDiagnostic{Name: name, Expected: expected, Actual: actual, TakesTypeArgs: takesTypeArgs, Span: c.sourceSpan(location), LegacyMessage: legacyMessage}.build())
}

func (c *Checker) addTypeMismatch(expected Type, actual Type, location parse.Location) {
	c.addTypeMismatchWithLegacy(expected, actual, "", location)
}

func (c *Checker) addTypeMismatchWithLegacy(expected Type, actual Type, legacyMessage string, location parse.Location) {
	c.addDiagnostic(typeMismatchDiagnostic{
		Expected:      expected,
		Actual:        actual,
		ActualSpan:    c.sourceSpan(location),
		LegacyMessage: legacyMessage,
	}.build())
}

func (c *Checker) addMissingTypeArguments(typeName string, location parse.Location) {
	c.addDiagnostic(missingTypeArgumentsDiagnostic{TypeName: typeName, Span: c.sourceSpan(location)}.build())
}

func (c *Checker) addIncorrectTypeArgumentCount(expected int, actual int, legacyMessage string, location parse.Location) {
	c.addDiagnostic(incorrectTypeArgumentCountDiagnostic{
		Expected:      expected,
		Actual:        actual,
		LegacyMessage: legacyMessage,
		Span:          c.sourceSpan(location),
	}.build())
}

func (c *Checker) addMethodIntroducedGeneric(name string, reason methodIntroducedGenericReason, location parse.Location) {
	c.addDiagnostic(methodIntroducedGenericDiagnostic{Name: name, Reason: reason, Span: c.sourceSpan(location)}.build())
}

func (c *Checker) addIncorrectArgumentType(legacyMessage string, expected Type, actual Type, argumentLocation parse.Location, parameter Parameter, requiresMutable bool) {
	var parameterSpan *SourceSpan
	if parameter.declaredAt.FilePath != "" {
		span := parameter.declaredAt
		parameterSpan = &span
	}
	c.addDiagnostic(incorrectArgumentTypeDiagnostic{
		LegacyMessage:   legacyMessage,
		Expected:        expected,
		Actual:          actual,
		ArgumentSpan:    c.sourceSpan(argumentLocation),
		ParameterName:   parameter.Name,
		ParameterSpan:   parameterSpan,
		RequiresMutable: requiresMutable,
	}.build())
}

func (c *Checker) resolveModule(name string) Module {
	if mod, ok := c.program.Imports[name]; ok {
		return mod
	}

	if mod, ok := prelude[name]; ok {
		return mod
	}

	return nil
}

func (c *Checker) findModuleByPath(path string) Module {
	for _, mod := range c.program.Imports {
		if mod.Path() == path {
			return mod
		}
	}

	return nil
}

func namedTypeRequiresTypeArguments(t Type) bool {
	switch typ := t.(type) {
	case *StructDef:
		return (len(typ.GenericParams) > 0 && len(typ.TypeArgs) == 0) || hasGenericsInType(typ)
	default:
		return hasGenericsInType(typ)
	}
}

func collectGenericsFromType(t Type, params *[]string, seen map[string]bool) {
	switch t := t.(type) {
	case *TypeVar:
		if !seen[t.name] {
			*params = append(*params, t.name)
			seen[t.name] = true
		}
	case *List:
		collectGenericsFromType(t.of, params, seen)
	case *Chan:
		collectGenericsFromType(t.of, params, seen)
	case *Receiver:
		collectGenericsFromType(t.of, params, seen)
	case *Sender:
		collectGenericsFromType(t.of, params, seen)
	case *Map:
		collectGenericsFromType(t.key, params, seen)
		collectGenericsFromType(t.value, params, seen)
	case *FunctionDef:
		for _, p := range t.Parameters {
			collectGenericsFromType(p.Type, params, seen)
		}
		if t.ReturnType != nil {
			collectGenericsFromType(t.ReturnType, params, seen)
		}
	case *Maybe:
		collectGenericsFromType(t.of, params, seen)
	case *Result:
		collectGenericsFromType(t.val, params, seen)
		collectGenericsFromType(t.err, params, seen)
	case *StructDef:
		if len(t.TypeArgs) == 0 {
			for _, genericName := range t.GenericParams {
				if !seen[genericName] {
					*params = append(*params, genericName)
					seen[genericName] = true
				}
			}
		}
		for _, typeArg := range t.TypeArgs {
			collectGenericsFromType(typeArg, params, seen)
		}
	}
}

func (c *Checker) specializeAliasedType(originalType Type, typeArgs []parse.DeclaredType, loc parse.Location) Type {
	// 1. Collect generics from the original type
	genericParams := []string{}
	seenGenerics := make(map[string]bool)
	collectGenericsFromType(originalType, &genericParams, seenGenerics)

	if len(genericParams) == 0 {
		c.addDiagnostic(nonGenericTypeSpecializationDiagnostic{Span: c.sourceSpan(loc)}.build())
		return originalType
	}

	if len(typeArgs) != len(genericParams) {
		c.addIncorrectTypeArgumentCount(len(genericParams), len(typeArgs), "", loc)
		return originalType
	}

	if structDef, ok := originalType.(*StructDef); ok {
		if c.isResolvingStructDefinition(structDef) {
			c.addDiagnostic(recursiveGenericSelfReferenceDiagnostic{
				TypeName: structDef.Name,
				Span:     c.sourceSpan(loc),
			}.build())
			return structDef
		}
		c.ensureStructDefinitionResolved(structDef)
		genericParams = []string{}
		seenGenerics = make(map[string]bool)
		collectGenericsFromType(originalType, &genericParams, seenGenerics)
		if len(genericParams) == 0 && len(structDef.GenericParams) > 0 {
			genericParams = append(genericParams, structDef.GenericParams...)
		}
		if len(typeArgs) != len(genericParams) {
			c.addIncorrectTypeArgumentCount(len(genericParams), len(typeArgs), "", loc)
			return originalType
		}
		typeVarMap := make(map[string]*TypeVar, len(genericParams))
		for i, typeArg := range typeArgs {
			resolvedArgType := c.resolveType(typeArg)
			typeVarMap[genericParams[i]] = &TypeVar{name: genericParams[i], actual: resolvedArgType, bound: true}
		}
		return copyStructWithTypeVarMap(structDef, typeVarMap)
	}

	// 3. Replace generics
	specializedType := originalType
	for i, typeArg := range typeArgs {
		genericName := genericParams[i]
		resolvedArgType := c.resolveType(typeArg)
		specializedType = replaceGeneric(specializedType, genericName, resolvedArgType)
	}
	return specializedType
}

// validateMapKeyType reports a diagnostic when a map key type is not a valid Go
// map key. Map keys must be Go strictly-comparable so every Ard map lowers to a
// plain Go map (ADR 0031). Unresolved generic parameters are allowed; the
// constraint applies when they are instantiated.
func (c *Checker) validateMapKeyType(key Type, loc parse.Location) {
	if key == nil || isValidMapKeyType(key) {
		return
	}
	// Type annotations are resolved more than once, so dedupe by location to
	// avoid emitting the same map-key diagnostic multiple times.
	if c.reportedMapKeyErrors == nil {
		c.reportedMapKeyErrors = map[parse.Location]bool{}
	}
	if c.reportedMapKeyErrors[loc] {
		return
	}
	c.reportedMapKeyErrors[loc] = true
	c.addDiagnostic(invalidMapKeyTypeDiagnostic{KeyType: key, Span: c.sourceSpan(loc)}.build())
}

// makeMutableType resolves `mut T` annotations. A foreign Go named type's
// mutable form is its pointer form (`mut image::Point` is `*image.Point`),
// matching how Go signatures import pointer parameters, so both spellings
// produce the same type. Everything else wraps in an Ard mutable reference.
func (c *Checker) makeMutableType(inner Type) Type {
	if foreign, ok := inner.(*ForeignType); ok {
		if foreign.Pointer {
			return foreign
		}
		if pointer := foreign.PointerForm(); pointer != nil {
			return pointer
		}
	}
	return MakeMutableRef(inner)
}

// isComparableValueType reports whether a type can be compared with == / != per
// ADR 0031: only primitives and enums (and, via the caller, their nullable
// forms), plus foreign named scalars, which compare with the target's native
// ==. There is no structural equality over lists, maps, structs, unions, or
// Any.
func isComparableValueType(t Type) bool {
	if t == nil {
		return false
	}
	if isPrimitiveScalar(t) {
		return true
	}
	// A foreign named scalar (for example Go's time.Month or a status enum-like
	// type) compares with the target's native == on its scalar underlying.
	if foreign, ok := t.(*ForeignType); ok && !foreign.Pointer && foreign.Underlying != nil && isComparableValueType(foreign.Underlying) {
		return true
	}
	_, isEnum := t.(*Enum)
	return isEnum
}

func isValidMapKeyType(t Type) bool {
	switch ty := t.(type) {
	case *TypeVar:
		if ty.actual != nil {
			return isValidMapKeyType(ty.actual)
		}
		return true
	case *Maybe, *List, *Map, *Result, *Union, *FunctionDef, *Trait, *anyType:
		return false
	default:
		return true
	}
}

func scalarTypeByName(name string) Type {
	switch name {
	case "Int8":
		return Int8
	case "Int16":
		return Int16
	case "Int32":
		return Int32
	case "Int64":
		return Int64
	case "Uint":
		return Uint
	case "Uint8":
		return Uint8
	case "Uint16":
		return Uint16
	case "Uint32":
		return Uint32
	case "Uint64":
		return Uint64
	case "Uintptr":
		return Uintptr
	case "Float32":
		return Float32
	}
	return nil
}

func (c *Checker) resolveType(t parse.DeclaredType) Type {
	// Defense in depth for the parser's type contract (issue #258 class): a
	// nil DeclaredType reaching the checker with a clean parse is a parser
	// bug. Recovery on error-carrying trees leaves nil holes on purpose, and
	// tooling (LSP) checks such trees, so the internal-error report only
	// fires when the parse was clean; either way checking degrades to an
	// unknown type instead of dereferencing nil.
	if t == nil {
		if !c.options.HasParseErrors {
			c.addDiagnostic(malformedTypeNodeDiagnostic{Span: c.sourceSpan(parse.Location{})}.build())
		}
		return &TypeVar{name: "unknown"}
	}
	var baseType Type
	switch ty := t.(type) {
	case *parse.StringType:
		baseType = Str
	case *parse.IntType:
		baseType = Int
	case *parse.FloatType:
		baseType = Float64
	case *parse.BooleanType:
		baseType = Bool
	case *parse.VoidType:
		baseType = Void

	case *parse.MutableType:
		baseType = c.makeMutableType(c.resolveType(ty.Inner))
	case parse.MutableType:
		baseType = c.makeMutableType(c.resolveType(ty.Inner))
	case *parse.FunctionType:
		// Convert each parameter type and return type
		params := make([]Parameter, len(ty.Params))
		for i, param := range ty.Params {
			mutable := false
			if i < len(ty.ParamMutability) {
				mutable = ty.ParamMutability[i]
			}
			paramType := c.resolveType(param)
			if mutable {
				// A `mut pkg::T` parameter in function-type position takes the
				// foreign type's pointer form, matching named `mut` parameters,
				// so annotations unify with imported Go signatures. Non-foreign
				// types keep the Mutable-flag representation that call-site
				// mutability checking relies on.
				if foreign, ok := paramType.(*ForeignType); ok && !foreign.Pointer {
					if pointer := foreign.PointerForm(); pointer != nil {
						paramType = pointer
					}
				}
			}
			params[i] = Parameter{
				Name:    fmt.Sprintf("arg%d", i),
				Type:    paramType,
				Mutable: mutable,
			}
		}
		// If no return type specified, default to Void
		var returnType Type = Void
		if ty.Return != nil {
			returnType = c.resolveType(ty.Return)
		}

		// Create a FunctionDef from the function type syntax
		baseType = &FunctionDef{
			Name:       "<function>",
			Parameters: params,
			ReturnType: returnType,
		}
	case *parse.List:
		of := c.resolveType(ty.Element)
		baseType = MakeList(of)
	case *parse.FixedArray:
		of := c.resolveType(ty.Element)
		baseType = MakeFixedArray(of, ty.Length)
	case *parse.Map:
		key := c.resolveType(ty.Key)
		value := c.resolveType(ty.Value)
		c.validateMapKeyType(key, ty.Key.GetLocation())
		baseType = MakeMap(key, value)
	case *parse.ResultType:
		val := c.resolveType(ty.Val)
		err := c.resolveType(ty.Err)
		baseType = MakeResult(val, err)
	case *parse.CustomType:
		switch t.GetName() {
		case "Any":
			baseType = Any
			break
		case "Byte":
			baseType = Byte
			break
		case "Rune":
			baseType = Rune
			break
		case "Maybe":
			if len(ty.TypeArgs) != 1 {
				c.addIncorrectTypeArgumentCount(1, len(ty.TypeArgs), "Generic type Maybe requires type arguments", ty.GetLocation())
				return &TypeVar{name: "unknown"}
			}
			baseType = MakeMaybe(c.resolveType(ty.TypeArgs[0]))
			break
		case "Chan":
			if len(ty.TypeArgs) != 1 {
				c.addIncorrectTypeArgumentCount(1, len(ty.TypeArgs), "Generic type Chan requires type arguments", ty.GetLocation())
				return &TypeVar{name: "unknown"}
			}
			baseType = MakeChan(c.resolveType(ty.TypeArgs[0]))
			break
		case "Receiver":
			if len(ty.TypeArgs) != 1 {
				c.addIncorrectTypeArgumentCount(1, len(ty.TypeArgs), "Generic type Receiver requires type arguments", ty.GetLocation())
				return &TypeVar{name: "unknown"}
			}
			baseType = MakeReceiver(c.resolveType(ty.TypeArgs[0]))
			break
		case "Sender":
			if len(ty.TypeArgs) != 1 {
				c.addIncorrectTypeArgumentCount(1, len(ty.TypeArgs), "Generic type Sender requires type arguments", ty.GetLocation())
				return &TypeVar{name: "unknown"}
			}
			baseType = MakeSender(c.resolveType(ty.TypeArgs[0]))
			break
		default:
			baseType = scalarTypeByName(t.GetName())
		}
		if baseType != nil {
			break
		}
		if ty.Type.Target == nil && c.topLevelTypeAliases != nil {
			if _, ok := c.topLevelTypeAliases[t.GetName()]; ok {
				resolvedAlias := c.resolveTopLevelTypeAliasReference(t.GetName(), ty.GetLocation())
				if c.recursiveTopLevelAliases[t.GetName()] {
					baseType = resolvedAlias
					break
				}
			}
		}

		if sym, ok := c.scope.get(t.GetName()); ok {
			if isNominalType(sym.Type) && !strings.Contains(t.GetName(), "::") {
				c.recordTypeRef(ty.GetLocation(), t.GetName())
			}
			if len(ty.TypeArgs) > 0 {
				baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
			} else {
				if namedTypeRequiresTypeArguments(sym.Type) {
					c.addMissingTypeArguments(t.GetName(), ty.GetLocation())
				}
				baseType = sym.Type
			}
			break
		}
		if ty.Type.Target != nil {
			targetName := ty.Type.Target.(*parse.Identifier).Name
			if goPkg := c.program.GoImports[targetName]; goPkg != nil {
				if goType := goPkg.Types[ty.Type.Property.(*parse.Identifier).Name]; goType != nil {
					baseType = goType
					break
				}
			}
			mod := c.resolveModule(targetName)
			if mod != nil {
				// at some point, this will need to unwrap the property down to root for nested paths: `mod::sym::more`
				propName := ty.Type.Property.(*parse.Identifier).Name
				sym := mod.Get(propName)
				if !sym.IsZero() {
					if c.spans != nil {
						c.spans.add(SpanRecord{
							Loc:    ty.GetLocation(),
							Target: &SpanTarget{Kind: TargetType, Module: mod.Path(), File: c.moduleFiles[mod.Path()], Symbol: propName},
						})
					}
					if len(ty.TypeArgs) > 0 {
						baseType = c.specializeAliasedType(sym.Type, ty.TypeArgs, ty.GetLocation())
					} else {
						if namedTypeRequiresTypeArguments(sym.Type) {
							c.addMissingTypeArguments(fmt.Sprintf("%s::%s", ty.Type.Target, ty.Type.Property), ty.GetLocation())
						}
						baseType = sym.Type
					}
					break
				}
			}
		}
		c.addUnresolvedReference(unrecognizedType, t.GetName(), t.GetLocation())
		return &TypeVar{name: "unknown"}
	case *parse.GenericType:
		if !c.genericAllowedInCurrentMethod(ty.Name) {
			c.addMethodIntroducedGeneric(ty.Name, methodGenericInvalidOccurrence, ty.GetLocation())
		}
		if existing := c.scope.findGeneric(ty.Name); existing != nil {
			baseType = existing
		} else {
			baseType = &TypeVar{name: ty.Name}
		}
	default:
		panic(fmt.Errorf("unrecognized type: %s", t.GetName()))
	}

	// If the type is nullable, wrap it in a Maybe
	if t.IsNullable() {
		return &Maybe{of: baseType}
	}

	return baseType
}

func (c *Checker) destructurePath(expr *parse.StaticFunction) (string, string) {
	absolute := expr.Target.String() + "::" + expr.Function.Name
	parts := strings.Split(absolute, "::")

	switch len(parts) {
	case 3:
		return parts[0], strings.Join(parts[1:], "::")
	case 2:
		return parts[0], parts[1]
	default:
		return parts[0], parts[0]
	}
}

func (c *Checker) pushFunctionGenericContext(fnDef *FunctionDef, extraParams ...string) {
	params := genericParamsForFunction(fnDef)
	params = appendUniqueStrings(params, extraParams...)
	if len(params) == 0 {
		c.genericContextStack = append(c.genericContextStack, nil)
		return
	}
	context := make(map[string]bool, len(params))
	for _, param := range params {
		context[param] = true
	}
	c.genericContextStack = append(c.genericContextStack, context)
}

func (c *Checker) popFunctionGenericContext() {
	if len(c.genericContextStack) == 0 {
		return
	}
	c.genericContextStack = c.genericContextStack[:len(c.genericContextStack)-1]
}

func (c *Checker) pushMethodGenericAllowlist(params []string) {
	allowed := make(map[string]bool, len(params))
	for _, param := range params {
		allowed[param] = true
	}
	c.methodGenericAllowlist = append(c.methodGenericAllowlist, allowed)
}

func (c *Checker) popMethodGenericAllowlist() {
	if len(c.methodGenericAllowlist) == 0 {
		return
	}
	c.methodGenericAllowlist = c.methodGenericAllowlist[:len(c.methodGenericAllowlist)-1]
}

func (c *Checker) genericAllowedInCurrentMethod(name string) bool {
	if len(c.methodGenericAllowlist) == 0 {
		return true
	}
	return c.methodGenericAllowlist[len(c.methodGenericAllowlist)-1][name]
}

func (c *Checker) genericInCurrentContext(name string) bool {
	if len(c.genericContextStack) == 0 {
		return false
	}
	current := c.genericContextStack[len(c.genericContextStack)-1]
	return current[name]
}

func (c *Checker) validateExplicitCallTypeArg(typeArg parse.DeclaredType) {
	params := []string{}
	seen := map[string]bool{}
	collectGenericParamsFromDeclaredType(typeArg, &params, seen)
	for _, param := range params {
		if c.genericInCurrentContext(param) {
			continue
		}
		c.addDiagnostic(unboundGenericTypeArgumentDiagnostic{
			Name: param,
			Span: c.sourceSpan(typeArg.GetLocation()),
		}.build())
	}
}

func (c *Checker) resolveCallTypeArgs(typeArgs []parse.DeclaredType) []Type {
	if len(typeArgs) == 0 {
		return nil
	}
	resolved := make([]Type, len(typeArgs))
	for i, typeArg := range typeArgs {
		resolved[i] = c.resolveType(typeArg)
		c.validateExplicitCallTypeArg(typeArg)
	}
	return resolved
}

func (c *Checker) checkUnsafeCast(s *parse.StaticFunction) Expression {
	modName, _ := c.destructurePath(s)
	if !c.hasExplicitImportAlias("ard/unsafe", modName) {
		c.addError("unsafe::cast requires importing ard/unsafe", s.Target.GetLocation())
		return nil
	}
	callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)
	if len(callTypeArgs) != 1 {
		c.addInvalidFunctionTypeArguments("unsafe::cast", 1, len(callTypeArgs), true, s.GetLocation(), "unsafe::cast requires exactly one explicit type argument")
		return nil
	}
	if len(s.Function.Args) != 1 {
		c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
		return nil
	}
	if s.Function.Args[0].Name != "" && s.Function.Args[0].Name != "value" {
		name := s.Function.Args[0].Name
		c.addUnknownNamedArgument(name, s.Function.Args[0].GetLocation(), "unknown argument: "+name)
		return nil
	}
	arg := c.checkExprAs(s.Function.Args[0].Value, Any)
	if arg == nil {
		return nil
	}
	if !c.areCompatible(Any, arg.Type()) {
		c.addTypeMismatch(Any, arg.Type(), s.Function.Args[0].Value.GetLocation())
		return nil
	}
	targetType := callTypeArgs[0]
	return &UnsafeCast{Value: arg, TargetType: targetType, ReturnType: MakeMaybe(targetType)}
}

// isDynamicMatchSubject reports whether a match subject participates in
// dynamic foreign type tests (ADR 0042): opaque Any values and foreign Go
// interface values.
func isDynamicMatchSubject(t Type) bool {
	if _, ok := t.(*anyType); ok {
		return true
	}
	foreign, ok := t.(*ForeignType)
	return ok && foreign.Interface
}

// checkForeignTypeMatch checks a match whose subject is Any or a foreign Go
// interface value. Arms name concrete foreign Go named types and bind the
// narrowed value; the dynamic type set is open, so a catch-all is required.
func (c *Checker) checkForeignTypeMatch(s *parse.MatchExpression, subject Expression, allowMixedVoid bool) Expression {
	cases := []ForeignTypeCase{}
	seen := map[string]SourceSpan{}
	var catchAll *Block
	var catchAllSpan *SourceSpan
	for _, matchCase := range s.Cases {
		switch p := matchCase.Pattern.(type) {
		case *parse.Identifier:
			if p.Name != "_" {
				c.addInvalidForeignTypePattern("Match on a dynamic value requires foreign type patterns like pkg::Type(binding) or a catch-all '_'", matchCase.Pattern.GetLocation(), "expected `pkg::Type(binding)` or `_`")
				continue
			}
			if catchAll != nil {
				c.addDuplicateMatchArm(Warn, "Duplicate catch-all case", matchCase.Pattern.GetLocation(), catchAllSpan)
				continue
			}
			span := c.sourceSpan(matchCase.Pattern.GetLocation())
			catchAllSpan = &span
			catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
		case *parse.StaticFunction:
			nsIdent, ok := p.Target.(*parse.Identifier)
			if !ok {
				c.addInvalidForeignTypePattern("Foreign type pattern must be qualified as pkg::Type(binding)", matchCase.Pattern.GetLocation(), "qualify this pattern as `pkg::Type(binding)`")
				continue
			}
			goPkg := c.program.GoImports[nsIdent.Name]
			if goPkg == nil {
				c.addUnresolvedReference(unknownGoNamespace, nsIdent.Name, nsIdent.GetLocation())
				continue
			}
			typ := goPkg.Types[p.Function.Name]
			if typ == nil {
				if reason := goPkg.UnsupportedTypes[p.Function.Name]; reason != "" {
					qualified := nsIdent.Name + "::" + p.Function.Name
					legacy := fmt.Sprintf("Unsupported Go type %s: %s", qualified, reason)
					c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "type", Name: qualified, Reason: reason, Span: c.sourceSpan(matchCase.Pattern.GetLocation()), LegacyMessage: legacy}.build())
				} else {
					c.addUnresolvedReference(unrecognizedType, fmt.Sprintf("%s::%s", nsIdent.Name, p.Function.Name), matchCase.Pattern.GetLocation())
				}
				continue
			}
			foreign, ok := typ.(*ForeignType)
			if !ok || foreign.Interface {
				legacy := fmt.Sprintf("Foreign type pattern must name a concrete foreign type, got %s::%s", nsIdent.Name, p.Function.Name)
				c.addInvalidForeignTypePattern(legacy, matchCase.Pattern.GetLocation(), "this pattern does not name a concrete foreign type")
				continue
			}
			if len(p.Function.Args) != 1 {
				c.addInvalidForeignTypePattern("Foreign type pattern requires exactly one binding, like pkg::Type(binding)", matchCase.Pattern.GetLocation(), "provide exactly one binding")
				continue
			}
			bindingIdent, ok := p.Function.Args[0].Value.(*parse.Identifier)
			if !ok {
				c.addInvalidForeignTypePattern("Foreign type pattern binding must be an identifier", p.Function.Args[0].GetLocation(), "use an identifier binding here")
				continue
			}
			if original, exists := seen[foreign.String()]; exists {
				c.addDuplicateMatchArm(Warn, fmt.Sprintf("Duplicate case: %s", foreign), matchCase.Pattern.GetLocation(), &original)
				continue
			}
			seen[foreign.String()] = c.sourceSpan(matchCase.Pattern.GetLocation())
			body := c.checkMatchArmBlock(matchCase.Body, func() {
				if bindingIdent.Name != "_" {
					c.scope.add(bindingIdent.Name, foreign, false)
				}
			})
			cases = append(cases, ForeignTypeCase{Type: foreign, Binding: bindingIdent.Name, Body: body})
		default:
			c.addInvalidForeignTypePattern("Match on a dynamic value requires foreign type patterns like pkg::Type(binding) or a catch-all '_'", matchCase.Pattern.GetLocation(), "expected `pkg::Type(binding)` or `_`")
		}
	}
	if catchAll == nil {
		c.addNonExhaustiveMatch("Match on a dynamic value requires a catch-all '_' case because the type set is open", s.GetLocation(), "add a catch-all `_` case for this open type set")
		return nil
	}
	var resultType Type
	for _, matchCase := range cases {
		if matchCase.Body == nil {
			continue
		}
		var ok bool
		resultType, ok = mergeMatchResultType(c, resultType, matchCase.Body.Type(), s.GetLocation(), allowMixedVoid)
		if !ok {
			return nil
		}
	}
	var ok bool
	resultType, ok = mergeMatchResultType(c, resultType, catchAll.Type(), s.GetLocation(), allowMixedVoid)
	if !ok {
		return nil
	}
	return &ForeignTypeMatch{Subject: subject, Cases: cases, CatchAll: catchAll, ResultType: resultType}
}

func (c *Checker) checkUnsafeIsNil(s *parse.StaticFunction) Expression {
	modName, _ := c.destructurePath(s)
	if !c.hasExplicitImportAlias("ard/unsafe", modName) {
		c.addError("unsafe::is_nil requires importing ard/unsafe", s.Target.GetLocation())
		return nil
	}
	if len(s.Function.TypeArgs) != 0 {
		c.addInvalidFunctionTypeArguments("unsafe::is_nil", 0, len(s.Function.TypeArgs), false, s.GetLocation(), "unsafe::is_nil does not accept type arguments")
		return nil
	}
	if len(s.Function.Args) != 1 {
		c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
		return nil
	}
	if s.Function.Args[0].Name != "" && s.Function.Args[0].Name != "value" {
		name := s.Function.Args[0].Name
		c.addUnknownNamedArgument(name, s.Function.Args[0].GetLocation(), "unknown argument: "+name)
		return nil
	}
	arg := c.checkExprAs(s.Function.Args[0].Value, Any)
	if arg == nil {
		return nil
	}
	if !c.areCompatible(Any, arg.Type()) {
		c.addTypeMismatch(Any, arg.Type(), s.Function.Args[0].Value.GetLocation())
		return nil
	}
	return &UnsafeIsNil{Value: arg}
}

func (c *Checker) hasExplicitImportAlias(path string, alias string) bool {
	for _, imp := range c.input.Imports {
		if imp.Path == path && imp.Name == alias {
			return true
		}
	}
	return false
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]bool, len(values)+len(additions))
	for _, value := range values {
		seen[value] = true
	}
	for _, value := range additions {
		if !seen[value] {
			values = append(values, value)
			seen[value] = true
		}
	}
	return values
}

func genericParamsForType(typ Type) []string {
	params := []string{}
	seen := map[string]bool{}
	collectGenericsFromType(typ, &params, seen)
	return params
}

func genericParamsForFunction(fnDef *FunctionDef) []string {
	if fnDef == nil {
		return nil
	}
	genericParams := []string{}
	seen := map[string]bool{}
	for _, param := range fnDef.GenericParams {
		if !seen[param] {
			genericParams = append(genericParams, param)
			seen[param] = true
		}
	}
	for _, param := range fnDef.Parameters {
		collectGenericsFromType(param.Type, &genericParams, seen)
	}
	if fnDef.ReturnType != nil {
		collectGenericsFromType(fnDef.ReturnType, &genericParams, seen)
	}
	return genericParams
}

func methodUsesOnlyReceiverGenerics(fnDef *FunctionDef, receiverGenerics []string) bool {
	allowed := make(map[string]bool, len(receiverGenerics))
	for _, param := range receiverGenerics {
		allowed[param] = true
	}
	params := genericParamsForFunction(fnDef)
	collectGenericsFromBlock(fnDef.Body, &params, mapSeenStrings(params))
	for _, param := range params {
		if !allowed[param] {
			return false
		}
	}
	return true
}

func mapSeenStrings(values []string) map[string]bool {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		seen[value] = true
	}
	return seen
}

func collectGenericsFromBlock(block *Block, params *[]string, seen map[string]bool) {
	if block == nil {
		return
	}
	for _, stmt := range block.Stmts {
		switch s := stmt.Stmt.(type) {
		case *VariableDef:
			collectGenericsFromType(s.Type(), params, seen)
		case *Reassignment:
			if s.Target != nil {
				collectGenericsFromType(s.Target.Type(), params, seen)
			}
		}
	}
}

func (c *Checker) explicitMethodGenericParams(fnDef *FunctionDef, subject Type) []string {
	if fnDef == nil {
		return nil
	}
	if len(fnDef.GenericParams) > 0 {
		return append([]string(nil), fnDef.GenericParams...)
	}
	params := genericParamsForFunction(fnDef)
	structType, ok := subject.(*StructDef)
	if !ok {
		return params
	}
	originalDef := c.structDefinition(structType)
	if originalDef == nil || len(originalDef.GenericParams) == 0 {
		return params
	}
	receiverGenerics := make(map[string]bool, len(originalDef.GenericParams))
	for _, param := range originalDef.GenericParams {
		receiverGenerics[param] = true
	}
	out := make([]string, 0, len(params))
	for _, param := range params {
		if !receiverGenerics[param] {
			out = append(out, param)
		}
	}
	return out
}

func formatTypeForDisplay(t Type) string {
	// For StructDef with generic fields, show type parameters
	if structDef, ok := t.(*StructDef); ok {
		if len(structDef.TypeArgs) > 0 {
			return structDef.String()
		}
		if resultType, hasResult := structDef.Fields["result"]; hasResult {
			// The result field's type indicates the generic parameter
			return fmt.Sprintf("%s<%s>", structDef.String(), resultType.String())
		}
	}
	return t.String()
}

func mergeMatchResultType(c *Checker, current Type, next Type, loc parse.Location, allowMixedVoid bool) (Type, bool) {
	if current == nil {
		return next, true
	}
	if expected, got, ok := mixedVoidMatchTypes(current, next); ok && !allowMixedVoid {
		c.addDiagnostic(branchTypeMismatchDiagnostic{
			Expected:      expected,
			Actual:        got,
			ActualSpan:    c.sourceSpan(loc),
			LegacyMessage: matchBranchTypeMismatch(expected, got),
			Title:         "Incompatible match branch types",
		}.build())
		return nil, false
	}
	merged, ok := commonResultType(current, next)
	if ok {
		return merged, true
	}
	if c.expectedExpr != nil && c.areCompatible(c.expectedExpr, current) && c.areCompatible(c.expectedExpr, next) {
		return c.expectedExpr, true
	}
	c.addDiagnostic(branchTypeMismatchDiagnostic{
		Expected:      current,
		Actual:        next,
		ActualSpan:    c.sourceSpan(loc),
		LegacyMessage: typeMismatch(current, next),
		Title:         "Incompatible match branch types",
	}.build())
	return nil, false
}

func mixedVoidMatchTypes(left Type, right Type) (Type, Type, bool) {
	if left == nil || right == nil || left == right {
		return nil, nil, false
	}
	if left == Void {
		return right, left, true
	}
	if right == Void {
		return left, right, true
	}
	return nil, nil, false
}

func matchBranchTypeMismatch(expected Type, got Type) string {
	return fmt.Sprintf("Type mismatch in match branches: expected %s, got %s", formatTypeForDisplay(expected), formatTypeForDisplay(got))
}

func typeMismatch(expected, got Type) string {
	exMsg := formatTypeForDisplay(expected)
	if _, isTrait := expected.(*Trait); isTrait {
		exMsg = "implementation of " + exMsg
	}
	return fmt.Sprintf("Type mismatch: Expected %s, got %s", exMsg, formatTypeForDisplay(got))
}

func (c *Checker) areCompatible(expected Type, actual Type) bool {
	if _, ok := expected.(*anyType); ok {
		return true
	}
	// A foreign named scalar narrows to its underlying primitive (ADR 0028
	// boundary coercions): the conversion is total, so `term::EventTitle`
	// flows anywhere a Str value is expected. The reverse direction stays
	// explicit, and contexts that need an identity (mutable places, equality)
	// must exclude the coercion with foreignScalarNarrows.
	if foreignScalarNarrows(expected, actual) {
		return true
	}
	// The reverse direction: a Str or Bool value widens into a foreign named
	// scalar type where the Go conversion is total (e.g. "demo.quit" flows
	// into a ui::IntentType parameter, field, or map key).
	if foreignScalarWidens(expected, actual) {
		return true
	}
	if trait, ok := expected.(*Trait); ok {
		return actual.hasTrait(trait)
	}
	if iface, ok := expected.(*ForeignType); ok && iface.Interface {
		// A named empty Go interface accepts any value, matching Go's own
		// assignability rules.
		if iface.EmptyInterface() {
			return true
		}
		actualBase, _ := mutableRefBase(actual)
		if actualForeign, ok := actualBase.(*ForeignType); ok {
			return actualForeign.equal(iface) || foreignGoAssignableTo(actualForeign, iface)
		}
		if def, ok := actualBase.(*StructDef); ok {
			return c.structImplementsForeignInterface(def, iface) && !c.foreignInterfaceImplRequiresPointer(def, iface)
		}
	}
	// A `mut T` value satisfies an expected value `T`: the reader receives a
	// dereferenced copy. Contexts that require mutable identity (mutable
	// parameters, assignment targets) check mutability separately.
	if ref, ok := actual.(*MutableRef); ok {
		if _, expectsRef := expected.(*MutableRef); !expectsRef {
			return c.areCompatible(expected, ref.Of())
		}
	}
	// A named Go map type accepts an Ard map with the same key/value shape,
	// mirroring Go's unnamed-to-named assignability.
	if foreign, ok := expected.(*ForeignType); ok && !foreign.Pointer && foreign.MapKey != nil && foreign.MapValue != nil {
		if actualMap, ok := actual.(*Map); ok {
			return foreign.MapKey.equal(actualMap.Key()) && foreign.MapValue.equal(actualMap.Value())
		}
	}
	// A named Go slice type accepts an Ard list with the same element type,
	// mirroring Go's unnamed-to-named assignability.
	if foreign, ok := expected.(*ForeignType); ok && !foreign.Pointer && foreign.Elem != nil {
		if actualList, ok := actual.(*List); ok {
			return foreign.Elem.equal(actualList.of)
		}
	}
	// A named Go array type accepts an Ard fixed array with the same element
	// type and length, mirroring Go's unnamed-to-named assignability.
	if foreign, ok := expected.(*ForeignType); ok && !foreign.Pointer {
		if expectedArray, ok := foreign.Underlying.(*FixedArray); ok {
			if actualArray, ok := actual.(*FixedArray); ok {
				return expectedArray.length == actualArray.length && expectedArray.of.equal(actualArray.of)
			}
		}
	}
	// The reverse also mirrors Go: a named Go array value flows where an Ard
	// fixed array with the same element type and length is expected.
	if expectedArray, ok := expected.(*FixedArray); ok {
		if foreign, ok := actual.(*ForeignType); ok && !foreign.Pointer {
			if actualArray, ok := foreign.Underlying.(*FixedArray); ok {
				return expectedArray.length == actualArray.length && expectedArray.of.equal(actualArray.of)
			}
		}
	}
	// A named Go func type accepts an Ard function value with a matching
	// signature, mirroring Go's unnamed-to-named assignability.
	if foreign, ok := expected.(*ForeignType); ok && !foreign.Pointer {
		if expectedFn, ok := foreign.Underlying.(*FunctionDef); ok {
			if _, isFn := actual.(*FunctionDef); isFn {
				return c.areCompatible(expectedFn, actual)
			}
		}
	}
	// The reverse also mirrors Go: a named Go func value flows where an Ard
	// function type is expected (named-to-unnamed assignability).
	if expectedFn, ok := expected.(*FunctionDef); ok {
		if foreign, ok := actual.(*ForeignType); ok && !foreign.Pointer {
			if actualFn, ok := foreign.Underlying.(*FunctionDef); ok {
				return c.areCompatible(expectedFn, actualFn)
			}
		}
	}
	return expected.equal(actual)
}

func (c *Checker) checkForeignInterfaceImplementation(s *parse.TraitImplementation, iface *ForeignType) *Statement {
	typeSym, ok := c.scope.get(s.ForType.Name)
	if !ok {
		c.addUnresolvedReference(undefinedType, s.ForType.Name, s.ForType.GetLocation())
		return nil
	}
	targetType, ok := typeSym.Type.(*StructDef)
	if !ok {
		c.addDiagnostic(invalidImplementationTargetDiagnostic{
			Target:        s.ForType.Name,
			ContractKind:  "Go interface",
			Span:          c.sourceSpan(s.ForType.GetLocation()),
			LegacyMessage: fmt.Sprintf("%s cannot implement a Go interface", s.ForType.Name),
		}.build())
		return nil
	}
	if !iface.MethodsLoaded && iface.LoadMethods != nil {
		iface.Methods, iface.UnsupportedMethods = iface.LoadMethods(false)
		iface.MethodsLoaded = true
	}

	validImpl := true
	for goName, reason := range iface.UnsupportedMethods {
		validImpl = false
		qualified := fmt.Sprintf("%s::%s.%s", iface.Qualifier, iface.Name, goName)
		legacy := fmt.Sprintf("Unsupported Go interface method %s: %s", qualified, reason)
		c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "interface method", Name: qualified, Reason: reason, Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy}.build())
	}
	implementedMethods := map[string]bool{}
	invalidImplementedMethods := map[string]bool{}
	pendingMethods := map[string]*FunctionDef{}
	pendingMethodSpans := map[string]SourceSpan{}
	receiverGenerics := genericParamsForType(targetType)
	for _, method := range s.Methods {
		if len(method.TypeParams) > 0 {
			validImpl = false
			c.addMethodIntroducedGeneric("", methodGenericExplicitDeclaration, method.GetLocation())
			invalidImplementedMethods[method.Name] = true
			continue
		}
		var interfaceMethod *FunctionDef
		var interfaceMethodName string
		for goName, m := range iface.Methods {
			if goMethodNameToArdName(goName) == method.Name {
				copy := *m
				interfaceMethod = &copy
				interfaceMethodName = goName
				break
			}
		}
		if interfaceMethod == nil {
			c.addDiagnostic(unexpectedImplementationMethodDiagnostic{
				Method:       method.Name,
				Contract:     iface.String(),
				ContractKind: "Go interface",
				Span:         c.sourceSpan(method.GetLocation()),
			}.build())
			continue
		}
		implementedMethods[method.Name] = true
		if len(method.Parameters) != len(interfaceMethod.Parameters) {
			validImpl = false
			c.addDiagnostic(implementationParameterCountDiagnostic{
				Method: method.Name, Expected: len(interfaceMethod.Parameters), Actual: len(method.Parameters), Span: c.sourceSpan(method.GetLocation()),
			}.build())
			invalidImplementedMethods[method.Name] = true
			continue
		}
		params := make([]Parameter, len(method.Parameters))
		valid := true
		for i, param := range method.Parameters {
			paramType, paramMutable := c.resolveParameterType(param.Type)
			if paramType == nil {
				c.addUnresolvedReference(unrecognizedType, param.Type.GetName(), param.Type.GetLocation())
				valid = false
				continue
			}
			expectedType := interfaceMethod.Parameters[i].Type
			// The impl method's signature becomes the generated Go method's
			// signature, so the foreign scalar narrowing coercion cannot apply:
			// the Go types must line up for interface satisfaction.
			if !c.areCompatible(expectedType, paramType) || foreignScalarNarrows(expectedType, paramType) || foreignScalarWidens(expectedType, paramType) || foreignFuncCoerces(expectedType, paramType) {
				c.addTypeMismatch(expectedType, paramType, param.GetLocation())
				valid = false
			}
			if paramMutable && mutableParamNeedsGoPointer(paramType) {
				legacy := fmt.Sprintf("Go interface method '%s' parameter '%s' cannot be mutable because it would change the Go ABI", method.Name, param.Name)
				c.addDiagnostic(implementationParameterMutabilityDiagnostic{
					Method: method.Name, Parameter: param.Name, ExpectedMutable: false, Span: c.sourceSpan(param.GetLocation()), LegacyMessage: legacy,
				}.build())
				valid = false
			}
			params[i] = Parameter{Name: param.Name, Type: paramType, Mutable: paramMutable, Loc: param.GetLocation(), declaredAt: c.sourceSpan(param.GetLocation())}
		}
		returnType := Type(Void)
		if method.ReturnType != nil {
			returnType = c.resolveType(method.ReturnType)
			if returnType == nil {
				c.addUnresolvedReference(unrecognizedReturnType, method.ReturnType.GetName(), method.ReturnType.GetLocation())
				valid = false
			}
		}
		if returnType != nil && (!c.areCompatible(interfaceMethod.ReturnType, returnType) || foreignScalarNarrows(interfaceMethod.ReturnType, returnType) || foreignScalarWidens(interfaceMethod.ReturnType, returnType) || foreignFuncCoerces(interfaceMethod.ReturnType, returnType)) {
			location := method.GetLocation()
			if method.ReturnType != nil {
				location = method.ReturnType.GetLocation()
			}
			legacy := fmt.Sprintf("Go interface method '%s' has return type of %s", method.Name, interfaceMethod.ReturnType)
			c.addDiagnostic(implementationReturnTypeDiagnostic{
				Method: method.Name, Expected: interfaceMethod.ReturnType, Actual: returnType, Span: c.sourceSpan(location), LegacyMessage: legacy,
			}.build())
			valid = false
		}
		if !valid {
			validImpl = false
			invalidImplementedMethods[method.Name] = true
			continue
		}
		c.pushMethodGenericAllowlist(receiverGenerics)
		fnDef := c.checkFunction(&method, func() {
			c.scope.add(s.Receiver.Name, targetType, method.Mutates)
		}, receiverGenerics...)
		c.popMethodGenericAllowlist()
		if fnDef == nil {
			validImpl = false
			invalidImplementedMethods[method.Name] = true
			continue
		}
		if !methodUsesOnlyReceiverGenerics(fnDef, receiverGenerics) {
			validImpl = false
			c.addMethodIntroducedGeneric("", methodGenericSemanticLeak, method.GetLocation())
			invalidImplementedMethods[method.Name] = true
			continue
		}
		fnDef.Receiver = s.Receiver.Name
		fnDef.Mutates = method.Mutates
		fnDef.Name = goMethodNameToArdName(interfaceMethodName)
		if _, exists := c.structMethod(targetType, fnDef.Name); exists {
			validImpl = false
			invalidImplementedMethods[method.Name] = true
			c.addDiagnostic(duplicateMethodDiagnostic{Method: fnDef.Name, Span: c.sourceSpan(method.GetLocation())}.build())
			continue
		}
		if _, exists := pendingMethods[fnDef.Name]; exists {
			validImpl = false
			invalidImplementedMethods[method.Name] = true
			original := pendingMethodSpans[fnDef.Name]
			c.addDiagnostic(duplicateMethodDiagnostic{Method: fnDef.Name, Span: c.sourceSpan(method.GetLocation()), OriginalSpan: &original}.build())
			continue
		}
		pendingMethods[fnDef.Name] = fnDef
		pendingMethodSpans[fnDef.Name] = c.sourceSpan(method.GetLocation())
	}

	for goName := range iface.Methods {
		ardName := goMethodNameToArdName(goName)
		if !implementedMethods[ardName] && !invalidImplementedMethods[ardName] {
			validImpl = false
			legacy := fmt.Sprintf("Missing method '%s' in Go interface '%s'", ardName, iface.String())
			c.addDiagnostic(missingImplementationMethodDiagnostic{
				Method: ardName, Contract: iface.String(), ContractKind: "Go interface", Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy,
			}.build())
		}
	}
	if !validImpl {
		return nil
	}
	for _, method := range pendingMethods {
		c.addStructMethod(targetType, method)
	}
	owner := StructMethodOwner(targetType)
	c.program.ForeignInterfaceImpls[owner] = append(c.program.ForeignInterfaceImpls[owner], iface)
	return &Statement{Stmt: targetType}
}

func (c *Checker) structImplementsForeignInterface(def *StructDef, iface *ForeignType) bool {
	if def == nil || iface == nil {
		return false
	}
	for _, implemented := range c.program.ForeignInterfaceImpls[StructMethodOwner(def)] {
		if implemented != nil && implemented.equal(iface) {
			return true
		}
	}
	return false
}

// freshContainerSatisfiesMutable reports whether arg may be passed to a
// mutable parameter of paramType despite not being a mutable place: a freshly
// constructed list/map literal is new storage with no other observer, and
// descriptor-shaped parameters (slices, maps) lower without pointers, so the
// callee mutating the temporary is sound. Idiomatic Go passes container
// literals to such parameters directly.
func freshContainerSatisfiesMutable(paramType Type, arg Expression) bool {
	if mutableParamNeedsGoPointer(paramType) {
		return false
	}
	switch arg.(type) {
	case *ListLiteral, *MapLiteral:
		return true
	}
	return false
}

func mutableParamNeedsGoPointer(t Type) bool {
	base, _ := mutableRefBase(t)
	switch typ := base.(type) {
	case *List, *Map, *Chan, *Receiver, *Sender:
		return false
	case *ForeignType:
		// Named Go map and slice types are descriptors like their unnamed
		// shapes: content mutation flows through the value without a pointer.
		if typ.MapKey != nil && typ.MapValue != nil {
			return false
		}
		if typ.Elem != nil {
			return false
		}
		return !typ.Pointer && !typ.Interface
	default:
		return true
	}
}

func (c *Checker) foreignInterfaceArgUpcast(expected Type, actual Expression) (Expression, bool) {
	iface, ok := expected.(*ForeignType)
	if !ok || !iface.Interface || actual == nil {
		return nil, false
	}
	actualBase, _ := mutableRefBase(actual.Type())
	def, ok := actualBase.(*StructDef)
	if !ok || !c.structImplementsForeignInterface(def, iface) {
		return nil, false
	}
	pointer := c.foreignInterfaceImplRequiresPointer(def, iface)
	if pointer && !c.isAddressableForeignInterfaceUpcast(actual) {
		return nil, false
	}
	return &ForeignInterfaceUpcast{Value: actual, Iface: iface, Pointer: pointer}, true
}

func (c *Checker) isAddressableForeignInterfaceUpcast(expr Expression) bool {
	switch expr.(type) {
	case *Variable, *InstanceProperty:
		return c.isMutable(expr)
	default:
		return false
	}
}

func (c *Checker) foreignInterfaceImplRequiresPointer(def *StructDef, iface *ForeignType) bool {
	if def == nil || iface == nil {
		return false
	}
	methods := c.structMethods(def)
	for goName := range iface.Methods {
		if method := methods[goMethodNameToArdName(goName)]; method != nil && method.Mutates {
			return true
		}
	}
	return false
}

func goMethodNameToArdName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			r = r + ('a' - 'A')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isDeferCallExpression(expr parse.Expression) bool {
	switch expr.(type) {
	case *parse.FunctionCall, *parse.FunctionValueCall, *parse.InstanceMethod, *parse.StaticFunction:
		return true
	default:
		return false
	}
}

func checkedBlockHasWork(block *Block) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if stmt.Expr != nil || stmt.Stmt != nil || stmt.Break {
			return true
		}
	}
	return false
}

func (c *Checker) checkDefer(s *parse.Defer) *Statement {
	if c.scope.getReturnType() == nil && !c.scope.insideScript() {
		legacy := "defer can only be used inside a function, method, closure, or script body"
		c.addDiagnostic(invalidDeferDiagnostic{Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy, Label: "`defer` cannot be used in a module initializer"}.build())
		return nil
	}
	if c.scope.insideUnsafeBlock() {
		legacy := "defer is not allowed inside unsafe blocks; move it outside the unsafe block"
		c.addDiagnostic(invalidDeferDiagnostic{Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy, Label: "move this deferred work outside the unsafe block"}.build())
		return nil
	}

	c.deferredWorkDepth++
	defer func() { c.deferredWorkDepth-- }()

	if s.Expr != nil {
		if !isDeferCallExpression(s.Expr) {
			c.addDiagnostic(invalidDeferDiagnostic{Span: c.sourceSpan(s.Expr.GetLocation()), LegacyMessage: "defer call form requires a call expression", Label: "deferred expression must be a function or method call"}.build())
			return nil
		}
		expr := c.withDiscardExprContext(func() Expression { return c.checkExpr(s.Expr) })
		if expr == nil {
			return nil
		}
		return &Statement{Stmt: &Defer{Expr: expr}}
	}

	diagnosticCount := len(c.diagnostics)
	body := c.checkBlockWithInferredFinalValue(s.Body, func() {
		// Deferred blocks lower as zero-argument closure bodies, so break cannot
		// target an outer loop and nested defers bind to the deferred closure.
		c.scope.expectReturn(Void)
	}, true)
	if len(c.diagnostics) == diagnosticCount && !checkedBlockHasWork(body) {
		c.addDiagnostic(invalidDeferDiagnostic{Span: c.sourceSpan(s.GetLocation()), LegacyMessage: "deferred block has no statements", Label: "add work to this deferred block or remove it"}.build())
		return nil
	}
	if len(c.diagnostics) != diagnosticCount {
		return nil
	}
	return &Statement{Stmt: &Defer{Body: body}}
}

func (c *Checker) checkStmt(stmt *parse.Statement) *Statement {
	if c.halted {
		return nil
	}
	if c.isDuplicateTopLevelTypeDeclaration(*stmt) {
		return nil
	}
	switch s := (*stmt).(type) {
	case *parse.Comment:
		return nil
	case *parse.Break:
		if !c.scope.breakAllowed() {
			// The unsafe pre-scan already reports breaks inside unsafe
			// blocks; avoid stacking a second diagnostic on the same token.
			if !c.scope.insideUnsafeBlock() {
				c.addDiagnostic(invalidBreakDiagnostic{Span: c.sourceSpan(s.GetLocation()), LegacyMessage: "break can only be used inside a loop"}.build())
			}
			return nil
		}
		return &Statement{Break: true}
	case *parse.Defer:
		return c.checkDefer(s)
	case *parse.TraitDefinition:
		{
			trait, ok := c.hoistedTrait(s.Name.Name)
			if !ok {
				return nil
			}
			c.recordDef(s.Name.GetLocation(), TypeKey(c.typeOwnerPath(), s.Name.Name))
			methods := make([]FunctionDef, len(s.Methods))
			for i, method := range s.Methods {
				params := make([]Parameter, len(method.Parameters))
				for j, param := range method.Parameters {
					paramType, paramMutable := c.resolveParameterType(param.Type)
					if paramType == nil {
						c.addUnresolvedReference(unrecognizedType, param.Type.GetName(), param.Type.GetLocation())
						continue
					}
					params[j] = Parameter{Name: param.Name, Type: paramType, Mutable: paramMutable, Loc: param.GetLocation(), declaredAt: c.sourceSpan(param.GetLocation())}
				}

				var returnType Type = Void
				if method.ReturnType != nil {
					returnType = c.resolveType(method.ReturnType)
					if returnType == nil {
						c.addUnresolvedReference(unrecognizedReturnType, method.ReturnType.GetName(), method.ReturnType.GetLocation())
						continue
					}
				}

				methods[i] = FunctionDef{
					Private:    false,
					Name:       method.Name,
					Parameters: params,
					ReturnType: returnType,
				}
			}

			trait.private = s.Private
			trait.Name = s.Name.Name
			trait.methods = methods
			return nil
		}
	case *parse.TraitImplementation:
		{
			var sym Symbol
			switch name := s.Trait.(type) {
			case parse.Identifier:
				if s, ok := c.scope.get(name.Name); ok {
					sym = *s
				}
			case parse.StaticProperty:
				modName := name.Target.(*parse.Identifier).Name
				mod := c.resolveModule(modName)
				if mod != nil {
					if propId, ok := name.Property.(*parse.Identifier); ok {
						sym = mod.Get(propId.Name)
					} else {
						c.addError(fmt.Sprintf("Bad path: %s", name), name.Property.GetLocation())
						return nil
					}
				} else if goPkg := c.program.GoImports[modName]; goPkg != nil {
					if propId, ok := name.Property.(*parse.Identifier); ok {
						if typ := goPkg.Types[propId.Name]; typ != nil {
							sym = Symbol{Name: propId.Name, Type: typ}
						}
					} else {
						c.addError(fmt.Sprintf("Bad path: %s", name), name.Property.GetLocation())
						return nil
					}
				}
			default:
				panic(fmt.Errorf("Unsupported trait node: %s", name))
			}

			if sym.IsZero() {
				c.addUnresolvedReference(undefinedTrait, s.Trait.String(), s.Trait.GetLocation())
				return nil
			}

			trait, ok := sym.Type.(*Trait)
			if !ok {
				if foreign, ok := sym.Type.(*ForeignType); ok && foreign.Interface {
					return c.checkForeignInterfaceImplementation(s, foreign)
				}
				legacy := fmt.Sprintf("%T is not a trait", sym.Type)
				c.addDiagnostic(invalidImplementationTargetDiagnostic{
					Target: fmt.Sprint(s.Trait), ContractKind: "trait", Span: c.sourceSpan(s.Trait.GetLocation()), LegacyMessage: legacy, InvalidContract: true,
				}.build())
				return nil
			}

			// Check that the type exists
			typeSym, ok := c.scope.get(s.ForType.Name)
			if !ok {
				c.addUnresolvedReference(undefinedType, s.ForType.Name, s.ForType.GetLocation())
				return nil
			}

			switch targetType := typeSym.Type.(type) {
			case *StructDef:
				// Verify that all required methods are implemented
				traitMethods := trait.GetMethods()
				implementedMethods := make(map[string]bool)
				invalidImplementedMethods := map[string]bool{}
				receiverGenerics := genericParamsForType(targetType)

				// Check each method in the implementation
				for _, method := range s.Methods {
					if len(method.TypeParams) > 0 {
						c.addMethodIntroducedGeneric("", methodGenericExplicitDeclaration, method.GetLocation())
						invalidImplementedMethods[method.Name] = true
						continue
					}
					implementedMethods[method.Name] = true

					// Find the corresponding trait method
					var traitMethod *FunctionDef
					for _, m := range traitMethods {
						if m.Name == method.Name {
							traitMethod = &m
							break
						}
					}

					if traitMethod == nil {
						c.addDiagnostic(unexpectedImplementationMethodDiagnostic{
							Method: method.Name, Contract: trait.name(), ContractKind: "trait", Span: c.sourceSpan(method.GetLocation()),
						}.build())
						continue
					}

					// Check parameter count
					if len(method.Parameters) != len(traitMethod.Parameters) {
						c.addDiagnostic(implementationParameterCountDiagnostic{
							Method: method.Name, Expected: len(traitMethod.Parameters), Actual: len(method.Parameters), Span: c.sourceSpan(method.GetLocation()),
						}.build())
						continue
					}

					params := make([]Parameter, len(method.Parameters))
					for i, param := range method.Parameters {
						paramType, paramMutable := c.resolveParameterType(param.Type)
						expectedType := traitMethod.Parameters[i].Type
						if !paramType.equal(expectedType) {
							c.addTypeMismatch(expectedType, paramType, param.GetLocation())
						}

						if paramMutable != traitMethod.Parameters[i].Mutable {
							legacy := fmt.Sprintf("Trait method '%s' parameter '%s' mutability mismatch", method.Name, param.Name)
							c.addDiagnostic(implementationParameterMutabilityDiagnostic{
								Method: method.Name, Parameter: param.Name, ExpectedMutable: traitMethod.Parameters[i].Mutable,
								Span: c.sourceSpan(param.GetLocation()), ExpectedSpan: sourceSpanIfPresent(traitMethod.Parameters[i].declaredAt), LegacyMessage: legacy,
							}.build())
						}

						params[i] = Parameter{Name: param.Name, Type: paramType, Mutable: paramMutable, Loc: param.GetLocation(), declaredAt: c.sourceSpan(param.GetLocation())}
					}

					// Check return type
					var returnType Type = Void
					if method.ReturnType != nil {
						returnType = c.resolveType(method.ReturnType)
					}
					if !traitMethod.ReturnType.equal(returnType) {
						location := method.GetLocation()
						if method.ReturnType != nil {
							location = method.ReturnType.GetLocation()
						}
						legacy := fmt.Sprintf("Trait method '%s' has return type of %s", method.Name, traitMethod.ReturnType)
						c.addDiagnostic(implementationReturnTypeDiagnostic{
							Method: method.Name, Expected: traitMethod.ReturnType, Actual: returnType, Span: c.sourceSpan(location), LegacyMessage: legacy,
						}.build())
						continue
					}

					// if we made it this far, it's a valid implementation
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, targetType, method.Mutates)
					}, receiverGenerics...)
					c.popMethodGenericAllowlist()
					if fnDef != nil && !methodUsesOnlyReceiverGenerics(fnDef, receiverGenerics) {
						c.addMethodIntroducedGeneric("", methodGenericSemanticLeak, method.GetLocation())
						invalidImplementedMethods[method.Name] = true
						continue
					}
					fnDef.Receiver = s.Receiver.Name
					fnDef.Mutates = method.Mutates
					// add the method to the struct method table
					c.addStructMethod(targetType, fnDef)
				}

				// Check if all required methods are implemented
				for _, method := range traitMethods {
					if !implementedMethods[method.Name] && !invalidImplementedMethods[method.Name] {
						legacy := fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name())
						c.addDiagnostic(missingImplementationMethodDiagnostic{
							Method: method.Name, Contract: trait.name(), ContractKind: "trait", Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy,
						}.build())
					}
				}

				// Add the trait to the struct type's traits list
				targetType.Traits = append(targetType.Traits, trait)

				// Return the struct so downstream backends can register the new trait methods
				return &Statement{Stmt: targetType}

			case *Enum:
				// Verify that all required methods are implemented (same logic as structs)
				traitMethods := trait.GetMethods()
				implementedMethods := make(map[string]bool)
				invalidImplementedMethods := map[string]bool{}
				receiverGenerics := genericParamsForType(targetType)

				// Check each method in the implementation
				for _, method := range s.Methods {
					if len(method.TypeParams) > 0 {
						c.addMethodIntroducedGeneric("", methodGenericExplicitDeclaration, method.GetLocation())
						invalidImplementedMethods[method.Name] = true
						continue
					}
					implementedMethods[method.Name] = true

					// Find the corresponding trait method
					var traitMethod *FunctionDef
					for _, m := range traitMethods {
						if m.Name == method.Name {
							traitMethod = &m
							break
						}
					}

					if traitMethod == nil {
						c.addDiagnostic(unexpectedImplementationMethodDiagnostic{
							Method: method.Name, Contract: trait.name(), ContractKind: "trait", Span: c.sourceSpan(method.GetLocation()),
						}.build())
						continue
					}

					// Check parameter count
					if len(method.Parameters) != len(traitMethod.Parameters) {
						c.addDiagnostic(implementationParameterCountDiagnostic{
							Method: method.Name, Expected: len(traitMethod.Parameters), Actual: len(method.Parameters), Span: c.sourceSpan(method.GetLocation()),
						}.build())
						continue
					}

					params := make([]Parameter, len(method.Parameters))
					for i, param := range method.Parameters {
						paramType, paramMutable := c.resolveParameterType(param.Type)
						expectedType := traitMethod.Parameters[i].Type
						if !paramType.equal(expectedType) {
							c.addTypeMismatch(expectedType, paramType, param.GetLocation())
						}

						if paramMutable != traitMethod.Parameters[i].Mutable {
							legacy := fmt.Sprintf("Trait method '%s' parameter '%s' mutability mismatch", method.Name, param.Name)
							c.addDiagnostic(implementationParameterMutabilityDiagnostic{
								Method: method.Name, Parameter: param.Name, ExpectedMutable: traitMethod.Parameters[i].Mutable,
								Span: c.sourceSpan(param.GetLocation()), ExpectedSpan: sourceSpanIfPresent(traitMethod.Parameters[i].declaredAt), LegacyMessage: legacy,
							}.build())
						}

						params[i] = Parameter{Name: param.Name, Type: paramType, Mutable: paramMutable, Loc: param.GetLocation(), declaredAt: c.sourceSpan(param.GetLocation())}
					}

					// Check return type
					var returnType Type = Void
					if method.ReturnType != nil {
						returnType = c.resolveType(method.ReturnType)
					}
					if !traitMethod.ReturnType.equal(returnType) {
						location := method.GetLocation()
						if method.ReturnType != nil {
							location = method.ReturnType.GetLocation()
						}
						legacy := fmt.Sprintf("Trait method '%s' has return type of %s", method.Name, traitMethod.ReturnType)
						c.addDiagnostic(implementationReturnTypeDiagnostic{
							Method: method.Name, Expected: traitMethod.ReturnType, Actual: returnType, Span: c.sourceSpan(location), LegacyMessage: legacy,
						}.build())
						continue
					}

					// if we made it this far, it's a valid implementation
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.checkFunction(&method, func() {
						c.scope.add(s.Receiver.Name, targetType, false) // Enums are immutable, so always false
					}, receiverGenerics...)
					c.popMethodGenericAllowlist()
					if fnDef != nil && !methodUsesOnlyReceiverGenerics(fnDef, receiverGenerics) {
						c.addMethodIntroducedGeneric("", methodGenericSemanticLeak, method.GetLocation())
						invalidImplementedMethods[method.Name] = true
						continue
					}
					fnDef.Receiver = s.Receiver.Name
					// Enums cannot have mutating methods
					if method.Mutates {
						c.addDiagnostic(mutatingEnumMethodDiagnostic{Span: c.sourceSpan(method.GetLocation())}.build())
					}
					fnDef.Mutates = false // Enums are always immutable

					// Ensure enum has Methods map initialized
					if targetType.Methods == nil {
						targetType.Methods = make(map[string]*FunctionDef)
					}
					// add the method to the enum
					targetType.Methods[method.Name] = fnDef
				}

				// Check if all required methods are implemented
				for _, method := range traitMethods {
					if !implementedMethods[method.Name] && !invalidImplementedMethods[method.Name] {
						legacy := fmt.Sprintf("Missing method '%s' in trait '%s'", method.Name, trait.name())
						c.addDiagnostic(missingImplementationMethodDiagnostic{
							Method: method.Name, Contract: trait.name(), ContractKind: "trait", Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy,
						}.build())
					}
				}

				// Add the trait to the enum type's traits list
				targetType.Traits = append(targetType.Traits, trait)

				// Return the enum so downstream backends can register the new trait methods
				return &Statement{Stmt: targetType}

			default:
				legacy := fmt.Sprintf("%s cannot implement a Trait", s.ForType.Name)
				c.addDiagnostic(invalidImplementationTargetDiagnostic{
					Target: s.ForType.Name, ContractKind: "trait", Span: c.sourceSpan(s.ForType.GetLocation()), LegacyMessage: legacy,
				}.build())
				return nil
			}
		}
	case *parse.TypeDeclaration:
		{
			// Record before the hoisted-alias early return below, or plain
			// aliases (already in scope) would never get a definition span.
			c.recordDef(s.Name.GetLocation(), TypeKey(c.typeOwnerPath(), s.Name.Name))
			if c.recursiveTopLevelAliases[s.Name.Name] {
				return nil
			}
			if len(s.Type) == 1 {
				if _, exists := c.scope.get(s.Name.Name); exists {
					return nil
				}
			}
			// Handle type declaration (type unions/aliases)
			types := make([]Type, len(s.Type))
			for i, declType := range s.Type {
				resolvedType := c.resolveType(declType)
				if resolvedType == nil {
					c.addUnresolvedReference(unrecognizedType, declType.GetName(), declType.GetLocation())
					return nil
				}
				types[i] = resolvedType
			}

			if len(types) == 1 {
				// It's a type alias
				c.scope.add(s.Name.Name, types[0], false)
				return nil
			}

			unionType, ok := c.hoistedUnion(s.Name.Name)
			if !ok {
				return nil
			}
			unionType.Name = s.Name.Name
			unionType.ModulePath = c.typeOwnerPath()
			unionType.Types = types
			unionType.Private = s.Private
			return &Statement{Stmt: unionType}
		}
	case *parse.VariableDeclaration:
		{
			var val Expression
			c.withValueExprContext(func() {
				if s.Type == nil {
					switch literal := s.Value.(type) {
					case *parse.ListLiteral:
						check := func() {
							if expr := c.checkList(nil, literal); expr != nil {
								val = expr
							}
						}
						if len(literal.Items) == 0 {
							c.withCollectionBinding(s.Name, s.NameLocation, check)
						} else {
							check()
						}
					case *parse.MapLiteral:
						check := func() {
							if expr := c.checkMap(nil, literal); expr != nil {
								val = expr
							}
						}
						if len(literal.Entries) == 0 {
							c.withCollectionBinding(s.Name, s.NameLocation, check)
						} else {
							check()
						}
					default:
						val = c.checkExpr(s.Value)
					}
				} else {
					expected := c.resolveType(s.Type)
					if expected == Void {
						c.addError("Cannot assign a void value", s.Value.GetLocation())
						return
					}

					switch literal := s.Value.(type) {
					case *parse.ListLiteral:
						if expr := c.checkList(expected, literal); expr != nil {
							val = expr
						}
					case *parse.MapLiteral:
						if expr := c.checkMap(expected, literal); expr != nil {
							val = expr
						}
					default:
						if expected != nil {
							expectedSpan := c.sourceSpan(s.Type.GetLocation())
							val = c.checkExprAsWithExpectation(
								s.Value,
								expected,
								&typeExpectation{Span: expectedSpan, Kind: expectationAnnotation},
							)
						}
					}
				}
			})

			if val == nil {
				return nil
			}

			__type := val.Type()

			if s.Type != nil {
				if expected := c.resolveType(s.Type); expected != nil {
					if !c.areCompatible(expected, val.Type()) {
						expectedSpan := c.sourceSpan(s.Type.GetLocation())
						c.addDiagnostic(typeMismatchDiagnostic{
							Expected:    expected,
							Actual:      val.Type(),
							ActualSpan:  c.sourceSpan(s.Value.GetLocation()),
							Expectation: &typeExpectation{Span: expectedSpan, Kind: expectationAnnotation},
						}.build())
						return nil
					}
					__type = expected
				}
			}

			if call, ok := val.(*ForeignFunctionCall); ok && call.PointerResult && s.Mutable {
				c.addDiagnostic(invalidForeignPointerBindingDiagnostic{Span: c.sourceSpan(s.Value.GetLocation())}.build())
				return nil
			}

			v := &VariableDef{
				Mutable: s.Mutable,
				Name:    s.Name,
				Value:   val,
				__type:  __type,
			}
			bound := c.scope.add(v.Name, v.__type, v.Mutable)
			c.recordBindingWithSpan(s.NameLocation, s.GetLocation(), bound)
			if c.spans != nil && c.scope.parent == nil {
				// Module-level values are importable; give them a canonical
				// identity for cross-module references.
				c.spans.add(SpanRecord{
					Loc:   s.GetLocation(),
					Key:   ValueKey(c.typeOwnerPath(), v.Name),
					IsDef: true,
				})
			}
			return &Statement{
				Stmt: v,
			}
		}
	case *parse.VariableAssignment:
		{
			if id, ok := s.Target.(*parse.Identifier); ok {
				target, ok := c.scope.get(id.Name)
				if ok {
					c.recordSymbolUse(id, target, nil)
				}
				if !ok {
					c.addUnresolvedReference(undefinedAssignmentTarget, id.Name, s.Target.GetLocation())
					return nil
				}

				// Assignment through a mutable-reference binding writes the
				// referent; it does not rebind, so the binding's own
				// (im)mutability does not gate it (ADR 0045).
				expectedType := target.Type
				refBase, targetIsRef := mutableRefBase(target.Type)
				if targetIsRef {
					expectedType = refBase
					// Whole-value writes cannot reach descriptor-backed
					// referents: the alias shares element storage, not the
					// binding slot (ADR 0040 / ADR 0045).
					if !mutableParamNeedsGoPointer(refBase) {
						c.addDiagnostic(unreachableReferentAssignmentDiagnostic{
							Name:            target.Name,
							Span:            c.sourceSpan(s.Target.GetLocation()),
							DeclarationSpan: sourceSpanIfPresent(target.declaredAt),
						}.build())
						return nil
					}
				} else if !target.mutable {
					c.addDiagnostic(immutableAssignmentDiagnostic{
						Name:            target.Name,
						AssignmentSpan:  c.sourceSpan(s.Target.GetLocation()),
						DeclarationSpan: target.declaredAt,
					}.build())
					return nil
				}

				var value Expression
				c.withValueExprContext(func() {
					value = c.checkExpr(s.Value)
				})
				if value == nil {
					return nil
				}

				if targetIsRef {
					// A reference binding is not rebindable: writing a new
					// reference through it has no storage to land in.
					if _, valueIsRef := value.Type().(*MutableRef); valueIsRef || isPointerForeign(value.Type()) {
						c.addDiagnostic(referenceRebindingDiagnostic{
							Span:            c.sourceSpan(s.Value.GetLocation()),
							DeclarationSpan: sourceSpanIfPresent(target.declaredAt),
						}.build())
						return nil
					}
				}

				if !c.areCompatible(expectedType, value.Type()) {
					c.addTypeMismatch(expectedType, value.Type(), s.Value.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: &Variable{*target}, Value: value},
				}
			}

			if ip, ok := s.Target.(*parse.InstanceProperty); ok {
				subject := c.checkExpr(ip)
				if subject == nil {
					return nil
				}
				// Check the value with the field type as expected context so
				// literals type contextually, then enforce compatibility for
				// every field target - native and foreign alike. Nullable
				// fields check against the inner type (mirroring call
				// arguments) and wrap below.
				fieldType := subject.Type()
				var value Expression
				c.withValueExprContext(func() {
					if maybeField, isMaybe := fieldType.(*Maybe); isMaybe {
						// Mirror call-argument checking for nullable targets:
						// literals and anonymous functions check against the
						// inner type; other expressions check freely and wrap
						// below when they produce the inner type.
						switch s.Value.(type) {
						case *parse.StrLiteral, *parse.NumLiteral, *parse.BoolLiteral,
							*parse.RuneLiteral, *parse.InterpolatedStr,
							*parse.ListLiteral, *parse.MapLiteral, *parse.AnonymousFunction:
							value = c.checkExprAs(s.Value, maybeField.Of())
						default:
							value = c.checkExpr(s.Value)
						}
						return
					}
					value = c.checkExprAs(s.Value, fieldType)
				})
				if value == nil {
					return nil
				}

				if !c.isMutable(subject) {
					c.addDiagnostic(immutablePropertyAssignmentDiagnostic{
						Property:        ip.String(),
						Span:            c.sourceSpan(s.Target.GetLocation()),
						DeclarationSpan: expressionBindingSpan(subject),
					}.build())
					return nil
				}
				if maybeField, isMaybe := fieldType.(*Maybe); isMaybe && !value.Type().equal(fieldType) {
					// A bare T assigns into a T? field by wrapping, matching
					// struct literal and call-argument behavior.
					if c.areCompatible(maybeField.Of(), value.Type()) {
						value = c.synthesizeMaybeSome(value, fieldType)
					}
				}
				if !c.areCompatible(fieldType, value.Type()) {
					c.addTypeMismatch(fieldType, value.Type(), s.Value.GetLocation())
					return nil
				}

				return &Statement{
					Stmt: &Reassignment{Target: subject, Value: value},
				}
			}

			if sp, ok := s.Target.(*parse.StaticProperty); ok {
				if id, ok := sp.Target.(*parse.Identifier); ok {
					if prop, ok := sp.Property.(*parse.Identifier); ok {
						if goPkg := c.program.GoImports[id.Name]; goPkg != nil {
							if typ := goPkg.Variables[prop.Name]; typ != nil {
								var value Expression
								c.withValueExprContext(func() {
									value = c.checkExpr(s.Value)
								})
								if value == nil {
									return nil
								}
								if !c.areCompatible(typ, value.Type()) {
									c.addTypeMismatch(typ, value.Type(), s.Value.GetLocation())
									return nil
								}
								return &Statement{Stmt: &Reassignment{Target: &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: typ, Assignable: true}, Value: value}}
							}
							if goPkg.Constants[prop.Name] != nil {
								c.addDiagnostic(nonAssignableStaticPropertyDiagnostic{
									Kind:   goConstantAssignment,
									Target: id.Name + "::" + prop.Name,
									Span:   c.sourceSpan(s.Target.GetLocation()),
								}.build())
								return nil
							}
							if reason := goPkg.UnsupportedConstants[prop.Name]; reason != "" {
								qualified := id.Name + "::" + prop.Name
								legacy := fmt.Sprintf("Unsupported Go constant %s: %s", qualified, reason)
								c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "constant", Name: qualified, Reason: reason, Span: c.sourceSpan(prop.GetLocation()), LegacyMessage: legacy}.build())
								return nil
							}
							if reason := goPkg.UnsupportedVariables[prop.Name]; reason != "" {
								qualified := id.Name + "::" + prop.Name
								legacy := fmt.Sprintf("Unsupported Go variable %s: %s", qualified, reason)
								c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "variable", Name: qualified, Reason: reason, Span: c.sourceSpan(prop.GetLocation()), LegacyMessage: legacy}.build())
								return nil
							}
							c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", id.Name, prop.Name), prop.GetLocation())
							return nil
						}
					}
				}
				c.addDiagnostic(nonAssignableStaticPropertyDiagnostic{
					Kind:   staticPropertyAssignment,
					Target: sp.String(),
					Span:   c.sourceSpan(s.Target.GetLocation()),
				}.build())
				return nil
			}

			c.addError(fmt.Sprintf("Unsupported reassignment target: %T", s.Target), s.Target.GetLocation())
			return nil
		}
	case *parse.WhileLoop:
		{
			// Check the condition expression
			var condition Expression
			if s.Condition == nil {
				condition = &BoolLiteral{true}
			} else {
				condition = c.checkExpr(s.Condition)
			}

			if condition == nil {
				return nil
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addDiagnostic(nonBooleanLoopConditionDiagnostic{Loop: "while", Actual: condition.Type(), Span: c.sourceSpan(s.Condition.GetLocation()), LegacyMessage: "While loop condition must be a boolean expression"}.build())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, c.markLoopScope(nil))

			// Create and return the while loop
			loop := &WhileLoop{
				Condition: condition,
				Body:      body,
			}

			return &Statement{Stmt: loop}
		}
	case *parse.ForLoop:
		{
			// Create a new scope for the loop body and initialization
			parent := c.scope
			scope := makeScope(parent)
			c.scope = &scope
			defer func() {
				c.scope = parent
			}()

			// Check the initialization statement - handle it as a variable declaration
			initDeclStmt := parse.Statement(s.Init)
			initStmt := c.checkStmt(&initDeclStmt)
			if initStmt == nil || initStmt.Stmt == nil {
				c.addDiagnostic(invalidForClauseDiagnostic{Clause: "initializer", Span: c.sourceSpan(s.Init.GetLocation()), LegacyMessage: "Invalid for loop initialization", Label: "this initializer could not be checked"}.build())
				return nil
			}
			initVarDef, ok := initStmt.Stmt.(*VariableDef)
			if !ok {
				c.addDiagnostic(invalidForClauseDiagnostic{Clause: "initializer", Span: c.sourceSpan(s.Init.GetLocation()), LegacyMessage: "For loop initialization must be a variable declaration", Label: "for-loop initialization must declare a variable"}.build())
				return nil
			}

			// Check the condition expression
			condition := c.checkExpr(s.Condition)
			if condition == nil {
				return nil
			}

			// Condition must be a boolean expression
			if condition.Type() != Bool {
				c.addDiagnostic(nonBooleanLoopConditionDiagnostic{Loop: "for", Actual: condition.Type(), Span: c.sourceSpan(s.Condition.GetLocation()), LegacyMessage: "For loop condition must be a boolean expression"}.build())
				return nil
			}

			// Check the update statement - handle it as a variable assignment
			incrStmt := parse.Statement(s.Incrementer)
			updateStmt := c.checkStmt(&incrStmt)
			if updateStmt == nil || updateStmt.Stmt == nil {
				c.addDiagnostic(invalidForClauseDiagnostic{Clause: "update", Span: c.sourceSpan(s.Incrementer.GetLocation()), LegacyMessage: "Invalid for loop update expression", Label: "this update expression could not be checked"}.build())
				return nil
			}
			update, ok := updateStmt.Stmt.(*Reassignment)
			if !ok {
				c.addDiagnostic(invalidForClauseDiagnostic{Clause: "update", Span: c.sourceSpan(s.Incrementer.GetLocation()), LegacyMessage: "For loop update must be a reassignment", Label: "for-loop update must reassign a value"}.build())
				return nil
			}

			// Check the body of the loop
			body := c.checkBlock(s.Body, c.markLoopScope(nil))

			// Create and return the for loop
			loop := &ForLoop{
				Init:      initVarDef,
				Condition: condition,
				Update:    update,
				Body:      body,
			}

			return &Statement{Stmt: loop}
		}
	case *parse.RangeLoop:
		{
			start, end := c.checkExpr(s.Start), c.checkExpr(s.End)
			if start == nil || end == nil {
				return nil
			}
			if start.Type() != end.Type() {
				legacy := fmt.Sprintf("Invalid range: %s..%s", start.Type(), end.Type())
				c.addDiagnostic(invalidRangeDiagnostic{StartType: start.Type(), EndType: end.Type(), StartSpan: c.sourceSpan(s.Start.GetLocation()), EndSpan: c.sourceSpan(s.End.GetLocation()), LegacyMessage: legacy}.build())
				return nil
			}

			if start.Type() == Int {
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  start,
					End:    end,
				}
				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(s.Cursor.Name, start.Type(), false))
					if loop.Index != "" {
						c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Index, Int, false))
					}
				}))
				loop.Body = body
				return &Statement{Stmt: loop}
			}

			legacy := fmt.Sprintf("Cannot create range of %s", start.Type())
			c.addDiagnostic(invalidRangeDiagnostic{StartType: start.Type(), EndType: end.Type(), StartSpan: c.sourceSpan(s.Start.GetLocation()), EndSpan: c.sourceSpan(s.End.GetLocation()), LegacyMessage: legacy, Unsupported: true}.build())
			return nil
		}
	case *parse.ForInLoop:
		{
			iterValue := c.checkExpr(s.Iterable)
			if iterValue == nil {
				return nil
			}

			// Handle strings specifically
			if iterValue.Type() == Str {
				loop := &ForInStr{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Value:  iterValue,
				}

				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					// Direct string iteration yields Unicode scalar values.
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(s.Cursor.Name, Rune, false))
					if loop.Index != "" {
						c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Index, Int, false))
					}
				}))

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Handle integer iteration (for i in n - sugar for 0..n)
			if iterValue.Type() == Int {
				// This is syntax sugar for a range from 0 to n
				loop := &ForIntRange{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					Start:  &IntLiteral{0}, // Start from 0
					End:    iterValue,      // End at the specified number
				}

				// Create a new scope for the loop body where the cursor is defined
				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					// Add the cursor variable to the scope
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(s.Cursor.Name, Int, false))
					if loop.Index != "" {
						c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Index, Int, false))
					}
				}))

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if listType, ok := iterValue.Type().(*List); ok {
				// This is syntax sugar for a range from 0 to n
				loop := &ForInList{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					List:   iterValue,
				}
				cursorMutable := c.isMutable(iterValue)

				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					// Add the cursor variable to the scope
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(s.Cursor.Name, listType.of, cursorMutable))
					if loop.Index != "" {
						c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Index, Int, false))
					}
				}))

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if arrayType, ok := iterValue.Type().(*FixedArray); ok {
				loop := &ForInList{
					Cursor: s.Cursor.Name,
					Index:  s.Cursor2.Name,
					List:   iterValue,
				}
				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(s.Cursor.Name, arrayType.of, false))
					if loop.Index != "" {
						c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Index, Int, false))
					}
				}))

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			if mapType, ok := iterValue.Type().(*Map); ok {
				iterable := c.checkExpr(s.Iterable)
				if iterable == nil {
					return nil
				}

				loop := &ForInMap{
					Key: s.Cursor.Name,
					Val: s.Cursor2.Name,
					Map: iterable,
				}

				valueMutable := c.isMutable(iterable)
				body := c.checkBlock(s.Body, c.markLoopScope(func() {
					// Add the cursors to the scope
					c.recordBinding(s.Cursor.GetLocation(), c.scope.add(loop.Key, mapType.Key(), false))
					c.recordBinding(s.Cursor2.GetLocation(), c.scope.add(loop.Val, mapType.Value(), valueMutable))
				}))

				loop.Body = body
				return &Statement{Stmt: loop}
			}

			// Currently we only support string, integer, and List iteration
			legacy := fmt.Sprintf("Cannot iterate over a %s", iterValue.Type())
			c.addDiagnostic(unsupportedIterationDiagnostic{Actual: iterValue.Type(), Span: c.sourceSpan(s.Iterable.GetLocation()), LegacyMessage: legacy}.build())
			return nil
		}
	case *parse.EnumDefinition:
		{
			enum, ok := c.hoistedEnum(s.Name)
			if !ok {
				return nil
			}
			c.recordDef(s.NameLocation, TypeKey(c.typeOwnerPath(), s.Name))
			if len(s.Variants) == 0 {
				c.addDiagnostic(emptyEnumDiagnostic{Name: s.Name, Span: c.sourceSpan(s.GetLocation())}.build())
				return nil
			}

			// Check for duplicate variant names
			seenNames := make(map[string]bool)
			for _, variant := range s.Variants {
				if seenNames[variant.Name] {
					c.addDiagnostic(duplicateEnumVariantDiagnostic{Name: variant.Name, Span: c.sourceSpan(s.GetLocation())}.build())
					return nil
				}
				seenNames[variant.Name] = true
			}

			// Compute discriminant values
			var computedValues []EnumValue
			var nextValue int = 0
			seenValues := make(map[int]string) // Detect duplicate discriminants
			seenValueSpans := make(map[int]*SourceSpan)

			for _, variant := range s.Variants {
				var value int
				var valueSpan *SourceSpan

				if variant.Value != nil {
					// Parse explicit value
					expr := c.checkExpr(variant.Value)
					if expr == nil {
						continue
					}

					// Value must be an integer literal
					intLit, ok := expr.(*IntLiteral)
					if !ok {
						c.addDiagnostic(invalidEnumDiscriminantDiagnostic{Span: c.sourceSpan(variant.Value.GetLocation())}.build())
						continue
					}
					value = intLit.Value
					span := c.sourceSpan(variant.Value.GetLocation())
					valueSpan = &span
					nextValue = value + 1
				} else {
					// Auto-assign
					value = nextValue
					nextValue++
				}

				// Check for duplicate discriminant values
				if existing, found := seenValues[value]; found {
					span := c.sourceSpan(s.GetLocation())
					if valueSpan != nil {
						span = *valueSpan
					}
					c.addDiagnostic(duplicateEnumDiscriminantDiagnostic{
						Value: value, PreviousName: existing, Span: span, PreviousSpan: seenValueSpans[value],
					}.build())
					return nil
				}
				seenValues[value] = variant.Name
				seenValueSpans[value] = valueSpan

				computedValues = append(computedValues, EnumValue{
					Name:  variant.Name,
					Value: value,
				})
			}

			enum.Private = s.Private
			enum.Name = s.Name
			enum.ModulePath = c.typeOwnerPath()
			enum.Values = computedValues
			if enum.Methods == nil {
				enum.Methods = make(map[string]*FunctionDef)
			}
			return nil
		}
	case *parse.StructDefinition:
		{
			def, ok := c.hoistedStruct(s.Name.Name)
			if !ok {
				return nil
			}
			c.recordDef(s.Name.GetLocation(), TypeKey(c.typeOwnerPath(), s.Name.Name))
			c.populateStructDefinition(def, s)
			return &Statement{Stmt: def}
		}
	case *parse.ImplBlock:
		{
			sym, ok := c.scope.get(s.Target.Name)
			if !ok {
				c.addUnresolvedReference(undefinedType, s.Target.String(), s.Target.GetLocation())
				return nil
			}
			if isNominalType(sym.Type) {
				c.recordTypeRef(s.Target.GetLocation(), s.Target.Name)
			}

			switch def := sym.Type.(type) {
			case *StructDef:
				receiverGenerics := genericParamsForType(def)
				signatures := make([]*FunctionDef, len(s.Methods))
				for i := range s.Methods {
					method := &s.Methods[i]
					if len(method.TypeParams) > 0 {
						c.addMethodIntroducedGeneric("", methodGenericExplicitDeclaration, method.GetLocation())
						continue
					}
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.resolveMethodSignature(method)
					c.popMethodGenericAllowlist()
					fnDef.Receiver = s.Receiver.Name
					fnDef.Mutates = method.Mutates
					signatures[i] = fnDef
					c.addStructMethod(def, fnDef)
				}
				for i := range s.Methods {
					method := &s.Methods[i]
					if signatures[i] == nil {
						continue
					}
					if c.spans != nil {
						c.spans.add(SpanRecord{
							Loc:   method.GetLocation(),
							Key:   MemberKey(TargetMethod, def.ModulePath, def.Name, method.Name),
							IsDef: true,
						})
					}
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.checkFunctionWithSignature(method, func() {
						c.scope.add(s.Receiver.Name, def, method.Mutates)
					}, signatures[i], receiverGenerics...)
					c.popMethodGenericAllowlist()
					if !methodUsesOnlyReceiverGenerics(fnDef, receiverGenerics) {
						c.addMethodIntroducedGeneric("", methodGenericSemanticLeak, method.GetLocation())
					}
				}
				return &Statement{Stmt: def}
			case *Enum:
				if def.Methods == nil {
					def.Methods = make(map[string]*FunctionDef)
				}
				receiverGenerics := genericParamsForType(def)
				signatures := make([]*FunctionDef, len(s.Methods))
				for i := range s.Methods {
					method := &s.Methods[i]
					if len(method.TypeParams) > 0 {
						c.addMethodIntroducedGeneric("", methodGenericExplicitDeclaration, method.GetLocation())
						continue
					}
					if method.Mutates {
						c.addDiagnostic(mutatingEnumMethodDiagnostic{Span: c.sourceSpan(method.GetLocation())}.build())
					}
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.resolveMethodSignature(method)
					c.popMethodGenericAllowlist()
					fnDef.Receiver = s.Receiver.Name
					signatures[i] = fnDef
					def.Methods[method.Name] = fnDef
				}
				for i := range s.Methods {
					method := &s.Methods[i]
					if signatures[i] == nil {
						continue
					}
					c.pushMethodGenericAllowlist(receiverGenerics)
					fnDef := c.checkFunctionWithSignature(method, func() {
						c.scope.add(s.Receiver.Name, def, false)
					}, signatures[i], receiverGenerics...)
					c.popMethodGenericAllowlist()
					if !methodUsesOnlyReceiverGenerics(fnDef, receiverGenerics) {
						c.addMethodIntroducedGeneric("", methodGenericSemanticLeak, method.GetLocation())
					}
				}
				return &Statement{Stmt: def}
			default:
				legacy := fmt.Sprintf("Can only implement methods on structs and enums, not %s", sym.Type)
				c.addDiagnostic(invalidImplementationTargetDiagnostic{
					Target: s.Target.Name, ContractKind: "method implementation", Span: c.sourceSpan(s.Target.GetLocation()), LegacyMessage: legacy,
				}.build())
				return nil
			}
		}
	case nil:
		return nil
	default:
		expr := c.withDiscardExprContext(func() Expression {
			return c.checkExpr((parse.Expression)(*stmt))
		})
		if expr == nil {
			return nil
		}
		return &Statement{Expr: expr}
	}
}

type collectionBindingContext struct {
	Name string
	Span SourceSpan
}

func (c *Checker) withCollectionBinding(name string, location parse.Location, check func()) {
	previous := c.emptyCollectionBinding
	c.emptyCollectionBinding = &collectionBindingContext{Name: name, Span: c.sourceSpan(location)}
	defer func() { c.emptyCollectionBinding = previous }()
	check()
}

func (c *Checker) checkList(declaredType Type, expr *parse.ListLiteral) *ListLiteral {
	// A named Go slice or array type accepts an Ard list-like literal: Go assignability
	// allows an unnamed composite value where the named type is expected.
	literalType := declaredType
	if foreign, ok := declaredType.(*ForeignType); ok && !foreign.Pointer && foreign.Elem != nil {
		declaredType = MakeList(foreign.Elem)
	} else if foreign, ok := declaredType.(*ForeignType); ok && !foreign.Pointer {
		if arrayType, ok := foreign.Underlying.(*FixedArray); ok {
			declaredType = arrayType
		}
	}
	if declaredType != nil {
		var expectedElementType Type
		if declaredList, ok := declaredType.(*List); ok {
			expectedElementType = declaredList.of
		} else if declaredArray, ok := declaredType.(*FixedArray); ok {
			expectedElementType = declaredArray.of
			if len(expr.Items) != declaredArray.length {
				c.addDiagnostic(fixedArrayLengthMismatchDiagnostic{
					Expected: declaredArray.length,
					Actual:   len(expr.Items),
					Span:     c.sourceSpan(expr.GetLocation()),
				}.build())
				return nil
			}
		} else {
			c.addDiagnostic(unexpectedListDiagnostic{Expected: declaredType, Span: c.sourceSpan(expr.GetLocation())}.build())
			return nil
		}
		elements := make([]Expression, len(expr.Items))
		hasError := false
		for i := range expr.Items {
			item := expr.Items[i]
			element := c.checkExprAs(item, expectedElementType)
			if element == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedElementType, element.Type()) {
				c.addTypeMismatch(expectedElementType, element.Type(), item.GetLocation())
				hasError = true
				continue
			}
			elements[i] = element
		}
		if hasError {
			return nil
		}

		return &ListLiteral{
			Elements: elements,
			_type:    literalType,
			ListType: literalType,
		}
	}

	if len(expr.Items) == 0 {
		var bindingSpan *SourceSpan
		bindingName := ""
		if c.emptyCollectionBinding != nil {
			span := c.emptyCollectionBinding.Span
			bindingSpan = &span
			bindingName = c.emptyCollectionBinding.Name
		}
		c.addDiagnostic(emptyCollectionNeedsTypeDiagnostic{
			Kind: emptyListCollection, LiteralSpan: c.sourceSpan(expr.GetLocation()), BindingName: bindingName, BindingSpan: bindingSpan,
		}.build())
		c.halted = true
		listType := MakeList(Void)
		return &ListLiteral{_type: listType, ListType: listType, Elements: []Expression{}}
	}

	hasError := false
	var elementType Type
	var elementSpan parse.Location
	elements := make([]Expression, len(expr.Items))
	for i := range expr.Items {
		item := expr.Items[i]
		element := c.checkExpr(item)
		if element == nil {
			hasError = true
			continue
		}

		if elementType == nil {
			elementType = element.Type()
			elementSpan = item.GetLocation()
		} else if !elementType.equal(element.Type()) {
			c.addDiagnostic(homogeneousListMismatchDiagnostic{
				Expected:     elementType,
				Actual:       element.Type(),
				ExpectedSpan: c.sourceSpan(elementSpan),
				ActualSpan:   c.sourceSpan(item.GetLocation()),
			}.build())
			hasError = true
			continue
		}

		elements[i] = element
	}

	if hasError || elementType == nil {
		return nil
	}

	listType := MakeList(elementType)
	return &ListLiteral{
		Elements: elements,
		_type:    listType,
		ListType: listType,
	}
}

func (c *Checker) checkBlock(stmts []parse.Statement, setup func()) *Block {
	return c.checkBlockWithExpected(stmts, setup, nil, false)
}

// markLoopScope wraps a block setup callback so the block's scope is marked
// as a loop body, making break statements valid within it (up to the next
// function boundary).
func (c *Checker) markLoopScope(setup func()) func() {
	return func() {
		c.scope.inLoop = true
		if setup != nil {
			setup()
		}
	}
}

func (c *Checker) checkBlockWithExpected(stmts []parse.Statement, setup func(), expectedFinal Type, onlyMatchFinal bool) *Block {
	if len(stmts) == 0 {
		return &Block{Stmts: []Statement{}}
	}

	parent := c.scope
	newScope := makeScope(parent)
	c.scope = &newScope
	defer func() {
		c.scope = parent
	}()

	if setup != nil {
		setup()
	}

	lastExprIndex := -1
	if expectedFinal != nil {
		for i := len(stmts) - 1; i >= 0; i-- {
			if _, ok := stmts[i].(*parse.Comment); ok {
				continue
			}
			if c.canCheckStatementAsExpectedExpression(stmts[i], expectedFinal, onlyMatchFinal) {
				lastExprIndex = i
			}
			break
		}
	}

	block := &Block{Stmts: make([]Statement, len(stmts)), DiscardFinalValue: expectedFinal == Void}
	for i := range stmts {
		if i == lastExprIndex {
			expr := c.checkExprAs(stmts[i].(parse.Expression), expectedFinal)
			if expr != nil {
				block.Stmts[i] = Statement{Expr: expr}
			}
		} else if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
		if c.halted {
			break
		}
	}
	return block
}

func (c *Checker) canCheckStatementAsExpectedExpression(stmt parse.Statement, expectedFinal Type, onlyMatchFinal bool) bool {
	if onlyMatchFinal && expectedFinal != Void {
		switch stmt.(type) {
		case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.InstanceMethod, *parse.UnsafeBlock:
			return true
		default:
			return false
		}
	}
	if expectedFinal == Void {
		switch stmt.(type) {
		case *parse.StrLiteral, *parse.RuneLiteral, *parse.BoolLiteral, *parse.VoidLiteral, *parse.NumLiteral, *parse.InterpolatedStr,
			*parse.Identifier, *parse.FunctionCall, *parse.FunctionValueCall, *parse.InstanceProperty, *parse.InstanceMethod,
			*parse.UnaryExpression, *parse.BinaryExpression, *parse.ChainedComparison, *parse.StaticFunction,
			*parse.IfStatement, *parse.AnonymousFunction, *parse.ListLiteral, *parse.MapLiteral,
			*parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.StaticProperty,
			*parse.StructInstance, *parse.Try, *parse.BlockExpression, *parse.UnsafeBlock:
			return true
		default:
			return false
		}
	}

	switch stmt.(type) {
	case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.SelectExpression, *parse.IfStatement, *parse.StaticFunction, *parse.FunctionValueCall, *parse.ListLiteral, *parse.MapLiteral, *parse.AnonymousFunction, *parse.UnsafeBlock:
		return true
	default:
		return false
	}
}

func parseStatementsContainBreak(stmts []parse.Statement) bool {
	for _, stmt := range stmts {
		if parseStatementContainsBreak(stmt) {
			return true
		}
	}
	return false
}

func parseStatementContainsBreak(stmt parse.Statement) bool {
	switch s := stmt.(type) {
	case nil:
		return false
	case *parse.Break:
		return true
	case *parse.WhileLoop:
		return parseExpressionContainsBreak(s.Condition) || parseStatementsContainBreak(s.Body)
	case *parse.RangeLoop:
		return parseExpressionContainsBreak(s.Start) || parseExpressionContainsBreak(s.End) || parseStatementsContainBreak(s.Body)
	case *parse.ForInLoop:
		return parseExpressionContainsBreak(s.Iterable) || parseStatementsContainBreak(s.Body)
	case *parse.ForLoop:
		return parseStatementContainsBreak(s.Init) || parseExpressionContainsBreak(s.Condition) || parseStatementContainsBreak(s.Incrementer) || parseStatementsContainBreak(s.Body)
	case *parse.IfStatement:
		return parseExpressionContainsBreak(s.Condition) || parseStatementsContainBreak(s.Body) || parseStatementContainsBreak(s.Else)
	case *parse.MatchExpression, *parse.ConditionalMatchExpression, *parse.Try, *parse.BlockExpression, *parse.UnsafeBlock, *parse.AnonymousFunction:
		return parseExpressionContainsBreak(s.(parse.Expression))
	default:
		if expr, ok := stmt.(parse.Expression); ok {
			return parseExpressionContainsBreak(expr)
		}
		return false
	}
}

func parseExpressionContainsBreak(expr parse.Expression) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *parse.UnaryExpression:
		return parseExpressionContainsBreak(e.Operand)
	case *parse.BinaryExpression:
		return parseExpressionContainsBreak(e.Left) || parseExpressionContainsBreak(e.Right)
	case *parse.ChainedComparison:
		for _, operand := range e.Operands {
			if parseExpressionContainsBreak(operand) {
				return true
			}
		}
	case *parse.RangeExpression:
		return parseExpressionContainsBreak(e.Start) || parseExpressionContainsBreak(e.End)
	case *parse.InterpolatedStr:
		for _, chunk := range e.Chunks {
			if parseExpressionContainsBreak(chunk) {
				return true
			}
		}
	case *parse.FunctionCall:
		for _, arg := range e.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.FunctionValueCall:
		if parseExpressionContainsBreak(e.Callee) {
			return true
		}
		for _, arg := range e.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.InstanceProperty:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Property)
	case *parse.InstanceMethod:
		if parseExpressionContainsBreak(e.Target) {
			return true
		}
		for _, arg := range e.Method.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.StaticProperty:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Property)
	case *parse.StaticFunction:
		if parseExpressionContainsBreak(e.Target) {
			return true
		}
		for _, arg := range e.Function.Args {
			if parseExpressionContainsBreak(arg.Value) {
				return true
			}
		}
	case *parse.ListLiteral:
		for _, item := range e.Items {
			if parseExpressionContainsBreak(item) {
				return true
			}
		}
	case *parse.MapLiteral:
		for _, entry := range e.Entries {
			if parseExpressionContainsBreak(entry.Key) || parseExpressionContainsBreak(entry.Value) {
				return true
			}
		}
	case *parse.StructInstance:
		for _, prop := range e.Properties {
			if parseExpressionContainsBreak(prop.Value) {
				return true
			}
		}
	case *parse.SelectExpression:
		for _, arm := range e.Cases {
			if arm.Op != nil && parseExpressionContainsBreak(arm.Op) {
				return true
			}
			if parseStatementsContainBreak(arm.Body) {
				return true
			}
		}
	case *parse.MatchExpression:
		if parseExpressionContainsBreak(e.Subject) {
			return true
		}
		for _, matchCase := range e.Cases {
			if parseExpressionContainsBreak(matchCase.Pattern) || parseStatementsContainBreak(matchCase.Body) {
				return true
			}
		}
	case *parse.ConditionalMatchExpression:
		for _, matchCase := range e.Cases {
			if parseExpressionContainsBreak(matchCase.Condition) || parseStatementsContainBreak(matchCase.Body) {
				return true
			}
		}
	case *parse.Try:
		return parseExpressionContainsBreak(e.Expression) || parseStatementsContainBreak(e.CatchBlock)
	case *parse.BlockExpression:
		return parseStatementsContainBreak(e.Statements)
	case *parse.UnsafeBlock:
		return parseStatementsContainBreak(e.Statements)
	case *parse.AnonymousFunction:
		return false
	case *parse.VariableAssignment:
		return parseExpressionContainsBreak(e.Target) || parseExpressionContainsBreak(e.Value)
	case *parse.VariableDeclaration:
		return parseExpressionContainsBreak(e.Value)
	}
	return false
}

func unsafeCatchOkValueTypes(block *Block) []Type {
	return unsafeCatchOkValueTypesInBlock(block, nil)
}

func unsafeCatchOkValueTypesInBlock(block *Block, aliases map[string][]Type) []Type {
	if block == nil {
		return nil
	}
	finalExprIndex := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		if block.Stmts[i].Expr != nil {
			finalExprIndex = i
			break
		}
	}
	if finalExprIndex == -1 {
		return nil
	}
	aliases = cloneUnsafeCatchTypeAliases(aliases)
	for _, stmt := range block.Stmts[:finalExprIndex] {
		if stmt.Expr != nil {
			continue
		}
		if stmt.Break {
			aliases = map[string][]Type{}
			continue
		}
		switch s := stmt.Stmt.(type) {
		case nil:
			continue
		case *VariableDef:
			aliases[s.Name] = unsafeCatchOkValueTypesFromExpression(s.Value, aliases)
		case *Reassignment:
			if target, ok := s.Target.(*Variable); ok {
				aliases[target.Name()] = unsafeCatchOkValueTypesFromExpression(s.Value, aliases)
			} else {
				aliases = map[string][]Type{}
			}
		default:
			aliases = map[string][]Type{}
		}
	}
	return unsafeCatchOkValueTypesFromExpression(block.Stmts[finalExprIndex].Expr, aliases)
}

func unsafeCatchOkValueTypesFromExpression(expr Expression, aliases map[string][]Type) []Type {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *Variable:
		if types, ok := aliases[e.Name()]; ok {
			return types
		}
	case *ModuleFunctionCall:
		if (e.Module == "Result" || e.Module == "ard/result") && e.Call != nil {
			switch e.Call.Name {
			case "ok":
				if len(e.Call.Args) == 1 {
					return []Type{e.Call.Args[0].Type()}
				}
				return nil
			case "err":
				return nil
			}
		}
	case *Block:
		return unsafeCatchOkValueTypesInBlock(e, aliases)
	case *If:
		var out []Type
		for _, branch := range e.Branches {
			out = append(out, unsafeCatchOkValueTypesInBlock(branch.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.Else, aliases)...)
		return out
	case *BoolMatch:
		out := unsafeCatchOkValueTypesInBlock(e.True, aliases)
		out = append(out, unsafeCatchOkValueTypesInBlock(e.False, aliases)...)
		return out
	case *IntMatch:
		var out []Type
		for _, block := range e.IntCases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		for _, block := range e.RangeCases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *StrMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *EnumMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *UnionMatch:
		var out []Type
		for _, match := range e.TypeCases {
			if match != nil {
				out = append(out, unsafeCatchOkValueTypesInBlock(match.Body, aliases)...)
			}
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *ConditionalMatch:
		var out []Type
		for _, matchCase := range e.Cases {
			out = append(out, unsafeCatchOkValueTypesInBlock(matchCase.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *OptionMatch:
		var out []Type
		if e.Some != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Some.Body, aliases)...)
		}
		out = append(out, unsafeCatchOkValueTypesInBlock(e.None, aliases)...)
		return out
	case *ResultMatch:
		var out []Type
		if e.Ok != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Ok.Body, aliases)...)
		}
		if e.Err != nil {
			out = append(out, unsafeCatchOkValueTypesInBlock(e.Err.Body, aliases)...)
		}
		return out
	}
	if result, ok := expr.Type().(*Result); ok {
		return []Type{result.val}
	}
	return nil
}

func unsafeCatchErrValueTypes(block *Block) []Type {
	return unsafeCatchErrValueTypesInBlock(block, nil)
}

func unsafeCatchErrValueTypesInBlock(block *Block, aliases map[string][]Type) []Type {
	if block == nil {
		return nil
	}
	finalExprIndex := -1
	for i := len(block.Stmts) - 1; i >= 0; i-- {
		if block.Stmts[i].Expr != nil {
			finalExprIndex = i
			break
		}
	}
	if finalExprIndex == -1 {
		return nil
	}
	aliases = cloneUnsafeCatchTypeAliases(aliases)
	for _, stmt := range block.Stmts[:finalExprIndex] {
		if stmt.Expr != nil {
			continue
		}
		if stmt.Break {
			aliases = map[string][]Type{}
			continue
		}
		switch s := stmt.Stmt.(type) {
		case nil:
			continue
		case *VariableDef:
			aliases[s.Name] = unsafeCatchErrValueTypesFromExpression(s.Value, aliases)
		case *Reassignment:
			if target, ok := s.Target.(*Variable); ok {
				aliases[target.Name()] = unsafeCatchErrValueTypesFromExpression(s.Value, aliases)
			} else {
				aliases = map[string][]Type{}
			}
		default:
			aliases = map[string][]Type{}
		}
	}
	return unsafeCatchErrValueTypesFromExpression(block.Stmts[finalExprIndex].Expr, aliases)
}

func unsafeCatchErrValueTypesFromExpression(expr Expression, aliases map[string][]Type) []Type {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *Variable:
		if types, ok := aliases[e.Name()]; ok {
			return types
		}
	case *ModuleFunctionCall:
		if (e.Module == "Result" || e.Module == "ard/result") && e.Call != nil {
			switch e.Call.Name {
			case "ok":
				return nil
			case "err":
				if len(e.Call.Args) == 1 {
					return []Type{e.Call.Args[0].Type()}
				}
				return nil
			}
		}
	case *Block:
		return unsafeCatchErrValueTypesInBlock(e, aliases)
	case *If:
		var out []Type
		for _, branch := range e.Branches {
			out = append(out, unsafeCatchErrValueTypesInBlock(branch.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.Else, aliases)...)
		return out
	case *BoolMatch:
		out := unsafeCatchErrValueTypesInBlock(e.True, aliases)
		out = append(out, unsafeCatchErrValueTypesInBlock(e.False, aliases)...)
		return out
	case *IntMatch:
		var out []Type
		for _, block := range e.IntCases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		for _, block := range e.RangeCases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *StrMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *EnumMatch:
		var out []Type
		for _, block := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(block, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *UnionMatch:
		var out []Type
		for _, match := range e.TypeCases {
			if match != nil {
				out = append(out, unsafeCatchErrValueTypesInBlock(match.Body, aliases)...)
			}
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *ConditionalMatch:
		var out []Type
		for _, matchCase := range e.Cases {
			out = append(out, unsafeCatchErrValueTypesInBlock(matchCase.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.CatchAll, aliases)...)
		return out
	case *OptionMatch:
		var out []Type
		if e.Some != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Some.Body, aliases)...)
		}
		out = append(out, unsafeCatchErrValueTypesInBlock(e.None, aliases)...)
		return out
	case *ResultMatch:
		var out []Type
		if e.Ok != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Ok.Body, aliases)...)
		}
		if e.Err != nil {
			out = append(out, unsafeCatchErrValueTypesInBlock(e.Err.Body, aliases)...)
		}
		return out
	}
	if result, ok := expr.Type().(*Result); ok {
		return []Type{result.err}
	}
	return nil
}

func cloneUnsafeCatchTypeAliases(aliases map[string][]Type) map[string][]Type {
	if len(aliases) == 0 {
		return map[string][]Type{}
	}
	cloned := make(map[string][]Type, len(aliases))
	for name, types := range aliases {
		cloned[name] = types
	}
	return cloned
}

func unsafeResultOkValueCompatible(expected Type, actual Type) bool {
	expected = deref(expected)
	actual = deref(actual)
	if expectedVar, ok := expected.(*TypeVar); ok {
		if expectedVar.bound && expectedVar.actual != nil {
			return unsafeResultOkValueCompatible(expectedVar.actual, actual)
		}
		actualVar, ok := actual.(*TypeVar)
		if !ok {
			return false
		}
		if actualVar.bound && actualVar.actual != nil {
			return unsafeResultOkValueCompatible(expected, actualVar.actual)
		}
		return expectedVar.name == actualVar.name
	}
	if actualVar, ok := actual.(*TypeVar); ok {
		if actualVar.bound && actualVar.actual != nil {
			return unsafeResultOkValueCompatible(expected, actualVar.actual)
		}
		return false
	}
	if !hasGenericsInType(expected) && !hasGenericsInType(actual) {
		return expected.equal(actual)
	}

	switch expectedType := expected.(type) {
	case *List:
		actualType, ok := actual.(*List)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Chan:
		actualType, ok := actual.(*Chan)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Receiver:
		actualType, ok := actual.(*Receiver)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Sender:
		actualType, ok := actual.(*Sender)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Map:
		actualType, ok := actual.(*Map)
		return ok && unsafeResultOkValueCompatible(expectedType.key, actualType.key) && unsafeResultOkValueCompatible(expectedType.value, actualType.value)
	case *Maybe:
		actualType, ok := actual.(*Maybe)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *Result:
		actualType, ok := actual.(*Result)
		return ok && unsafeResultOkValueCompatible(expectedType.val, actualType.val) && unsafeResultOkValueCompatible(expectedType.err, actualType.err)
	case *MutableRef:
		actualType, ok := actual.(*MutableRef)
		return ok && unsafeResultOkValueCompatible(expectedType.of, actualType.of)
	case *StructDef:
		actualType, ok := actual.(*StructDef)
		if !ok || expectedType.Name != actualType.Name || expectedType.ModulePath != actualType.ModulePath || len(expectedType.TypeArgs) != len(actualType.TypeArgs) {
			return false
		}
		for i := range expectedType.TypeArgs {
			if !unsafeResultOkValueCompatible(expectedType.TypeArgs[i], actualType.TypeArgs[i]) {
				return false
			}
		}
		return true
	case *FunctionDef:
		actualType, ok := actual.(*FunctionDef)
		if !ok || len(expectedType.Parameters) != len(actualType.Parameters) {
			return false
		}
		for i := range expectedType.Parameters {
			eMut, eType := normalizedParamMutability(expectedType.Parameters[i])
			aMut, aType := normalizedParamMutability(actualType.Parameters[i])
			if eMut != aMut || !unsafeResultOkValueCompatible(eType, aType) {
				return false
			}
		}
		return unsafeResultOkValueCompatible(expectedType.ReturnType, actualType.ReturnType)
	}

	if hasGenericsInType(expected) || hasGenericsInType(actual) {
		return false
	}
	return expected.equal(actual)
}

func (c *Checker) validateUnsafeCatchResults(block *Block, resultType *Result, loc parse.Location) {
	if block == nil || resultType == nil {
		return
	}
	for _, stmt := range block.Stmts {
		c.validateUnsafeCatchResultsInStatement(stmt, resultType, loc)
	}
}

func (c *Checker) validateUnsafeCatchResultsInStatement(stmt Statement, resultType *Result, loc parse.Location) {
	if stmt.Expr != nil {
		c.validateUnsafeCatchResultsInExpression(stmt.Expr, resultType, loc)
	}
	if stmt.Stmt == nil {
		return
	}
	switch s := stmt.Stmt.(type) {
	case *VariableDef:
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
	case *Reassignment:
		c.validateUnsafeCatchResultsInExpression(s.Target, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
	case *WhileLoop:
		c.validateUnsafeCatchResultsInExpression(s.Condition, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForIntRange:
		c.validateUnsafeCatchResultsInExpression(s.Start, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(s.End, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInStr:
		c.validateUnsafeCatchResultsInExpression(s.Value, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInList:
		c.validateUnsafeCatchResultsInExpression(s.List, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForInMap:
		c.validateUnsafeCatchResultsInExpression(s.Map, resultType, loc)
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	case *ForLoop:
		if s.Init != nil {
			c.validateUnsafeCatchResultsInExpression(s.Init.Value, resultType, loc)
		}
		c.validateUnsafeCatchResultsInExpression(s.Condition, resultType, loc)
		if s.Update != nil {
			c.validateUnsafeCatchResultsInExpression(s.Update.Target, resultType, loc)
			c.validateUnsafeCatchResultsInExpression(s.Update.Value, resultType, loc)
		}
		c.validateUnsafeCatchResults(s.Body, resultType, loc)
	}
}

func (c *Checker) validateUnsafeCatchResultsInExpression(expr Expression, resultType *Result, loc parse.Location) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *TryOp:
		c.validateUnsafeCatchResultsInExpression(e.Expr(), resultType, loc)
		if e.CatchBlock != nil {
			if catchResult, ok := e.CatchBlock.Type().(*Result); ok && catchResult.err.equal(resultType.err) {
				for _, okType := range unsafeCatchOkValueTypes(e.CatchBlock) {
					if !unsafeResultOkValueCompatible(resultType.val, okType) {
						c.addTypeMismatch(resultType, MakeResult(okType, catchResult.err), loc)
					}
				}
				for _, errType := range unsafeCatchErrValueTypes(e.CatchBlock) {
					if !unsafeResultOkValueCompatible(resultType.err, errType) {
						c.addTypeMismatch(resultType, MakeResult(catchResult.val, errType), loc)
					}
				}
			}
			c.validateUnsafeCatchResults(e.CatchBlock, resultType, loc)
		}
	case *Block:
		c.validateUnsafeCatchResults(e, resultType, loc)
	case *UnsafeBlock:
		if nestedResult, ok := e.Type().(*Result); ok {
			c.validateUnsafeCatchResults(e.Body, nestedResult, loc)
		}
	case *If:
		for _, branch := range e.Branches {
			c.validateUnsafeCatchResultsInExpression(branch.Condition, resultType, loc)
			c.validateUnsafeCatchResults(branch.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.Else, resultType, loc)
	case *BoolMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		c.validateUnsafeCatchResults(e.True, resultType, loc)
		c.validateUnsafeCatchResults(e.False, resultType, loc)
	case *IntMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.IntCases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		for _, block := range e.RangeCases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *StrMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.Cases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *EnumMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, block := range e.Cases {
			c.validateUnsafeCatchResults(block, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *UnionMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, match := range e.TypeCases {
			if match != nil {
				c.validateUnsafeCatchResults(match.Body, resultType, loc)
			}
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *ConditionalMatch:
		for _, matchCase := range e.Cases {
			c.validateUnsafeCatchResultsInExpression(matchCase.Condition, resultType, loc)
			c.validateUnsafeCatchResults(matchCase.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.CatchAll, resultType, loc)
	case *OptionMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Some != nil {
			c.validateUnsafeCatchResults(e.Some.Body, resultType, loc)
		}
		c.validateUnsafeCatchResults(e.None, resultType, loc)
	case *ResultMatch:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Ok != nil {
			c.validateUnsafeCatchResults(e.Ok.Body, resultType, loc)
		}
		if e.Err != nil {
			c.validateUnsafeCatchResults(e.Err.Body, resultType, loc)
		}
	case *Negation:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *Not:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *MutableRefExpr:
		c.validateUnsafeCatchResultsInExpression(e.Operand, resultType, loc)
	case *IntAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntSubtraction:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntMultiplication:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntDivision:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntModulo:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntGreater:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntGreaterEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntLess:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *IntLessEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatSubtraction:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatMultiplication:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatDivision:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatGreater:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatGreaterEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatLess:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *FloatLessEqual:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *StrAddition:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Equality:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Inequality:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *And:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *Or:
		c.validateUnsafeCatchResultsInExpression(e.Left, resultType, loc)
		c.validateUnsafeCatchResultsInExpression(e.Right, resultType, loc)
	case *StrMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ByteMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *RuneMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *IntMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *FloatMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *BoolMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *ListMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *MapMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *MaybeMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ResultMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *ListLiteral:
		for _, item := range e.Elements {
			c.validateUnsafeCatchResultsInExpression(item, resultType, loc)
		}
	case *MapLiteral:
		for _, key := range e.Keys {
			c.validateUnsafeCatchResultsInExpression(key, resultType, loc)
		}
		for _, value := range e.Values {
			c.validateUnsafeCatchResultsInExpression(value, resultType, loc)
		}
	case *StructInstance:
		for _, field := range e.Fields {
			c.validateUnsafeCatchResultsInExpression(field, resultType, loc)
		}
	case *FunctionCall:
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *FunctionValueCall:
		c.validateUnsafeCatchResultsInExpression(e.Callee, resultType, loc)
		for _, arg := range e.Args {
			c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
		}
	case *InstanceProperty:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
	case *InstanceMethod:
		c.validateUnsafeCatchResultsInExpression(e.Subject, resultType, loc)
		if e.Method != nil {
			for _, arg := range e.Method.Args {
				c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
			}
		}
	case *ModuleFunctionCall:
		if e.Call != nil {
			for _, arg := range e.Call.Args {
				c.validateUnsafeCatchResultsInExpression(arg, resultType, loc)
			}
		}
	case *UnsafeCast:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *UnsafeIsNil:
		c.validateUnsafeCatchResultsInExpression(e.Value, resultType, loc)
	case *ModuleStructInstance:
		if e.Property != nil {
			for _, field := range e.Property.Fields {
				c.validateUnsafeCatchResultsInExpression(field, resultType, loc)
			}
		}
	case *TemplateStr:
		for _, chunk := range e.Chunks {
			c.validateUnsafeCatchResultsInExpression(chunk, resultType, loc)
		}
	case *Panic:
		c.validateUnsafeCatchResultsInExpression(e.Message, resultType, loc)
	}
}

// primeGoResolver loads the check's whole Go import closure into the
// resolver's shared session before any import binds (ADR 0044). Priming is
// idempotent — drivers that already primed (frontend, test loader) resolve
// everything from cache — and it makes directly constructed checkers
// (tests, tools) share one go/types universe too. A prime error means the
// pre-scan missed a path, which is a compiler bug; when the miss lives only
// in a transitive module, the sub-module's own check surfaces it at its
// `use` statement instead.
//
// The type assertion is part of the contract: the LSP wraps its resolver
// (lockedGoResolver), intentionally opting out of per-check auto-prime
// because the engine owns session priming with the workspace-wide union.
func (c *Checker) primeGoResolver() {
	resolver, ok := c.options.GoResolver.(*GoPackagesResolver)
	if !ok || resolver == nil {
		return
	}
	paths := CollectGoImportPaths(c.moduleResolver, GoImportScanEntry{Program: c.input, ModulePath: c.modulePath})
	if err := resolver.Prime(paths); err != nil {
		for _, imp := range c.input.Imports {
			if imp.Kind == parse.ImportKindGo {
				c.addError(err.Error(), imp.GetLocation())
				break
			}
		}
	}
}

// mutRefPlaceName renders a place expression for diagnostics without
// falling back to Go struct formatting for complex subjects.
func mutRefPlaceName(operand Expression) string {
	switch e := operand.(type) {
	case *Variable:
		return e.Name()
	case *InstanceProperty:
		return mutRefPlaceName(e.Subject) + "." + e.Property
	case *ForeignFieldAccess:
		return mutRefPlaceName(e.Subject) + "." + e.Symbol
	}
	return "value"
}

// expectedFunctionTypeForClosure returns the function signature a closure
// literal should check against, or nil when the expected type provides none.
// A bare `*FunctionDef` counts only when it is a function *type* (marked with
// the "<function>" name) rather than a specific named function's type. A
// named Go func type (for example http.HandlerFunc) carries its signature as
// the foreign type's underlying function; closures check against that
// signature so parameters infer and a value-producing body is discarded for
// void callbacks, mirroring Go's unnamed-to-named assignability.
func expectedFunctionTypeForClosure(expected Type) *FunctionDef {
	switch expected := expected.(type) {
	case *FunctionDef:
		if expected.Name == "<function>" {
			return expected
		}
	case *ForeignType:
		if expected.Pointer {
			return nil
		}
		if fn, ok := expected.Underlying.(*FunctionDef); ok {
			return fn
		}
	}
	return nil
}

func (c *Checker) checkMatchArmBlock(stmts []parse.Statement, setup func()) *Block {
	expectedType := c.expectedExpr
	discardFinal := c.matchArmDiscardContext || expectedType == Void
	c.expectedExpr = nil
	var block *Block
	if expectedType == nil || expectedType == Void {
		block = c.checkBlockWithInferredFinalValue(stmts, setup, discardFinal)
	} else {
		block = c.checkBlockWithExpected(stmts, setup, expectedType, false)
	}
	c.expectedExpr = expectedType
	return block
}

func (c *Checker) checkBlockWithInferredFinalValue(stmts []parse.Statement, setup func(), finalDiscardContext bool) *Block {
	if len(stmts) == 0 {
		return &Block{Stmts: []Statement{}}
	}

	parent := c.scope
	newScope := makeScope(parent)
	c.scope = &newScope
	defer func() {
		c.scope = parent
	}()

	if setup != nil {
		setup()
	}

	lastExprIndex := -1
	for i := len(stmts) - 1; i >= 0; i-- {
		if _, ok := stmts[i].(*parse.Comment); ok {
			continue
		}
		if c.canCheckStatementAsExpectedExpression(stmts[i], Void, false) {
			lastExprIndex = i
		}
		break
	}

	block := &Block{Stmts: make([]Statement, len(stmts))}
	for i := range stmts {
		if i == lastExprIndex {
			var expr Expression
			if finalDiscardContext {
				expr = c.withDiscardExprContext(func() Expression {
					return c.checkExpr(stmts[i].(parse.Expression))
				})
			} else {
				expr = c.checkValueExpr(stmts[i].(parse.Expression))
			}
			if expr != nil {
				block.Stmts[i] = Statement{Expr: expr}
			}
		} else if stmt := c.checkStmt(&stmts[i]); stmt != nil {
			block.Stmts[i] = *stmt
		}
		if c.halted {
			break
		}
	}
	return block
}

func (c *Checker) withExpectedExpr(expected Type, check func() Expression) Expression {
	previousExpected := c.expectedExpr
	previousDiscard := c.discardExprContext
	c.expectedExpr = expected
	c.discardExprContext = expected == Void
	defer func() {
		c.expectedExpr = previousExpected
		c.discardExprContext = previousDiscard
	}()
	return check()
}

func (c *Checker) withDiscardExprContext(check func() Expression) Expression {
	previous := c.discardExprContext
	c.discardExprContext = true
	defer func() {
		c.discardExprContext = previous
	}()
	return check()
}

func (c *Checker) withValueExprContext(check func()) {
	previous := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previous
	}()
	check()
}

func (c *Checker) checkValueExpr(expr parse.Expression) Expression {
	var checked Expression
	c.withValueExprContext(func() {
		checked = c.checkExpr(expr)
	})
	return checked
}

func (c *Checker) checkMap(declaredType Type, expr *parse.MapLiteral) *MapLiteral {
	// A named Go map type accepts an Ard map literal: Go assignability allows
	// an unnamed map value where the named type is expected.
	if foreign, ok := declaredType.(*ForeignType); ok && !foreign.Pointer && foreign.MapKey != nil && foreign.MapValue != nil {
		declaredType = MakeMap(foreign.MapKey, foreign.MapValue)
	}
	// Handle empty map with declared type
	if len(expr.Entries) == 0 {
		if declaredType != nil {
			mapType, ok := declaredType.(*Map)
			if !ok {
				c.addDiagnostic(expectedMapTypeDiagnostic{Actual: declaredType, Span: c.sourceSpan(expr.GetLocation())}.build())
				return nil
			}

			// Return empty map with the declared type
			return &MapLiteral{
				Keys:      []Expression{},
				Values:    []Expression{},
				_type:     mapType,
				KeyType:   mapType.Key(),
				ValueType: mapType.Value(),
			}
		} else {
			// Empty map without a declared type is an error
			var bindingSpan *SourceSpan
			bindingName := ""
			if c.emptyCollectionBinding != nil {
				span := c.emptyCollectionBinding.Span
				bindingSpan = &span
				bindingName = c.emptyCollectionBinding.Name
			}
			c.addDiagnostic(emptyCollectionNeedsTypeDiagnostic{
				Kind: emptyMapCollection, LiteralSpan: c.sourceSpan(expr.GetLocation()), BindingName: bindingName, BindingSpan: bindingSpan,
			}.build())
			c.halted = true
			mapType := MakeMap(Void, Void)
			return &MapLiteral{
				_type:     mapType,
				Keys:      []Expression{},
				Values:    []Expression{},
				KeyType:   Void,
				ValueType: Void,
			}
		}
	}

	// Handle non-empty map
	if declaredType != nil {
		mapType, ok := declaredType.(*Map)
		if !ok {
			c.addDiagnostic(expectedMapTypeDiagnostic{Actual: declaredType, Span: c.sourceSpan(expr.GetLocation())}.build())
			return nil
		}

		expectedKeyType := mapType.key
		expectedValueType := mapType.value

		keys := make([]Expression, len(expr.Entries))
		values := make([]Expression, len(expr.Entries))

		hasError := false
		for i, entry := range expr.Entries {
			// Type check the key
			key := c.checkExpr(entry.Key)
			if key == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedKeyType, key.Type()) {
				c.addTypeMismatch(expectedKeyType, key.Type(), entry.Key.GetLocation())
				hasError = true
				continue
			}
			keys[i] = key

			// Type check the value
			value := c.checkExpr(entry.Value)
			if value == nil {
				hasError = true
				continue
			}
			if !c.areCompatible(expectedValueType, value.Type()) {
				c.addTypeMismatch(expectedValueType, value.Type(), entry.Value.GetLocation())
				hasError = true
				continue
			}
			values[i] = value
		}

		if hasError {
			return nil
		}

		return &MapLiteral{
			Keys:      keys,
			Values:    values,
			_type:     mapType,
			KeyType:   mapType.Key(),
			ValueType: mapType.Value(),
		}
	}

	// Type inference for non-empty maps without declared type
	keys := make([]Expression, len(expr.Entries))
	values := make([]Expression, len(expr.Entries))

	// Check the first entry to determine key and value types
	firstKey := c.checkExpr(expr.Entries[0].Key)
	firstValue := c.checkExpr(expr.Entries[0].Value)

	if firstKey == nil || firstValue == nil {
		return nil
	}

	keyType := firstKey.Type()
	valueType := firstValue.Type()
	keys[0] = firstKey
	values[0] = firstValue

	// Check that all entries have consistent types
	for i := 1; i < len(expr.Entries); i++ {
		key := c.checkExpr(expr.Entries[i].Key)
		if key == nil {
			keyType = Void
			continue
		}
		if !keyType.equal(key.Type()) {
			legacyMessage := fmt.Sprintf("Map key type mismatch: Expected %s, got %s", keyType, key.Type())
			c.addTypeMismatchWithLegacy(keyType, key.Type(), legacyMessage, expr.Entries[i].Key.GetLocation())
			continue
		}
		keys[i] = key

		value := c.checkExpr(expr.Entries[i].Value)
		if value == nil {
			valueType = Void
			continue
		}
		if !valueType.equal(value.Type()) {
			legacyMessage := fmt.Sprintf("Map value type mismatch: Expected %s, got %s", valueType, value.Type())
			c.addTypeMismatchWithLegacy(valueType, value.Type(), legacyMessage, expr.Entries[i].Value.GetLocation())
			continue
		}
		values[i] = value
	}

	// Create and return the map
	c.validateMapKeyType(keyType, expr.Entries[0].Key.GetLocation())
	mapType := MakeMap(keyType, valueType)
	return &MapLiteral{
		Keys:      keys,
		Values:    values,
		_type:     mapType,
		KeyType:   keyType,
		ValueType: valueType,
	}
}

func (c *Checker) validateForeignStructInstance(foreign *ForeignType, typeArgs []parse.DeclaredType, properties []parse.StructValue, loc parse.Location) *ForeignStructInstance {
	if foreign == nil || foreign.Target != "go" || foreign.Pointer || !foreign.Struct {
		c.addDiagnostic(invalidGoStructLiteralDiagnostic{Span: c.sourceSpan(loc), Message: "Go struct literals require a non-pointer Go struct type"}.build())
		return nil
	}
	foreign = c.instantiateForeignStructForLiteral(foreign, typeArgs, properties, loc)
	if foreign == nil {
		return nil
	}
	if !foreign.FieldsLoaded && foreign.LoadFields != nil {
		foreign.Fields, foreign.UnsupportedFields = foreign.LoadFields()
		foreign.FieldsLoaded = true
	}
	fields := map[string]Expression{}
	seen := map[string]SourceSpan{}
	for _, property := range properties {
		name := property.Name.Name
		fieldType := foreign.Fields[name]
		if fieldType == nil {
			if reason := foreign.UnsupportedFields[name]; reason != "" {
				c.addUnsupportedGoEntity("field", fmt.Sprintf("%s.%s", foreign, name), reason, "Unsupported foreign field", property.GetLocation())
			} else {
				c.addUnresolvedReference(unknownStructField, name, property.GetLocation())
			}
			continue
		}
		if original, duplicate := seen[name]; duplicate {
			c.addDiagnostic(duplicateStructLiteralFieldDiagnostic{
				Name: name, Span: c.sourceSpan(property.Name.GetLocation()), PreviousSpan: original,
			}.build())
			continue
		}
		seen[name] = c.sourceSpan(property.Name.GetLocation())
		value := c.checkExprAs(property.Value, fieldType)
		if value == nil {
			continue
		}
		if !c.areCompatible(fieldType, value.Type()) {
			c.addTypeMismatch(fieldType, value.Type(), property.Value.GetLocation())
			continue
		}
		fields[name] = value
	}
	return &ForeignStructInstance{Target: foreign.Target, Namespace: foreign.Namespace, Qualifier: foreign.Qualifier, Name: foreign.Name, Fields: fields, _type: foreign}
}

func (c *Checker) instantiateForeignStructForLiteral(foreign *ForeignType, typeArgs []parse.DeclaredType, properties []parse.StructValue, loc parse.Location) *ForeignType {
	named, ok := foreign.GoType.(*gotypes.Named)
	if !ok {
		if len(typeArgs) > 0 {
			c.addDiagnostic(invalidGoStructTypeArgumentsDiagnostic{
				Span: c.sourceSpan(declaredTypeLocation(typeArgs[0], loc)), LegacyMessage: "Go struct literal type arguments require a named Go type", Primary: "type arguments require a named Go type",
			}.build())
			return nil
		}
		return foreign
	}
	params := named.TypeParams()
	if params == nil || params.Len() == 0 {
		if len(typeArgs) > 0 {
			legacy := fmt.Sprintf("Go type %s is not generic", foreign)
			c.addDiagnostic(invalidGoStructTypeArgumentsDiagnostic{
				Span: c.sourceSpan(declaredTypeLocation(typeArgs[0], loc)), LegacyMessage: legacy, Primary: fmt.Sprintf("`%s` is not generic", foreign),
			}.build())
			return nil
		}
		return foreign
	}

	args := make([]Type, params.Len())
	goArgs := make([]gotypes.Type, params.Len())
	inferenceSpans := make([]SourceSpan, params.Len())
	if len(typeArgs) > 0 {
		if len(typeArgs) != params.Len() {
			legacy := fmt.Sprintf("Go type %s expects %d type argument(s), got %d", foreign, params.Len(), len(typeArgs))
			span := c.sourceSpan(loc)
			if len(typeArgs) > params.Len() {
				span = c.sourceSpan(declaredTypeLocation(typeArgs[params.Len()], loc))
			}
			c.addDiagnostic(invalidGoStructTypeArgumentsDiagnostic{
				Span: span, LegacyMessage: legacy, Primary: fmt.Sprintf("expected %d type argument(s), but found %d", params.Len(), len(typeArgs)),
			}.build())
			return nil
		}
		for i, typeArg := range typeArgs {
			arg := c.resolveType(typeArg)
			if arg == nil {
				return nil
			}
			goArg, ok := checkerTypeToGoType(arg)
			if !ok {
				legacy := fmt.Sprintf("Type argument %s cannot be used as a Go type argument", arg)
				c.addDiagnostic(invalidGoStructTypeArgumentsDiagnostic{
					Span: c.sourceSpan(declaredTypeLocation(typeArg, loc)), LegacyMessage: legacy, Primary: fmt.Sprintf("`%s` cannot be represented as a Go type argument", arg),
				}.build())
				return nil
			}
			args[i] = arg
			goArgs[i] = goArg
		}
	} else {
		inferredTypes := make([]Type, params.Len())
		inferredGoTypes := make([]gotypes.Type, params.Len())
		structType, ok := named.Underlying().(*gotypes.Struct)
		if !ok {
			return foreign
		}
		for _, property := range properties {
			field := exportedGoStructField(structType, property.Name.Name)
			if field == nil || !goTypeMentionsTypeParam(field.Type(), params) {
				continue
			}
			value := c.checkExprForInference(property.Value)
			if value == nil {
				continue
			}
			goValue, ok := checkerTypeToGoType(value.Type())
			if !ok {
				continue
			}
			if !c.inferGoStructTypeArgs(field.Type(), value.Type(), goValue, params, inferredTypes, inferredGoTypes, inferenceSpans, property.GetLocation()) {
				return nil
			}
		}
		for i := 0; i < params.Len(); i++ {
			if inferredTypes[i] == nil || inferredGoTypes[i] == nil {
				legacy := fmt.Sprintf("Could not infer type argument %s for Go type %s", params.At(i).Obj().Name(), foreign)
				c.addDiagnostic(goTypeInferenceFailureDiagnostic{
					Parameter: params.At(i).Obj().Name(), EntityKind: "type", EntityName: foreign.String(), Span: c.sourceSpan(loc), LegacyMessage: legacy,
				}.build())
				return nil
			}
			args[i] = inferredTypes[i]
			goArgs[i] = inferredGoTypes[i]
		}
	}

	for i, goArg := range goArgs {
		constraint, ok := params.At(i).Constraint().Underlying().(*gotypes.Interface)
		if ok && !gotypes.Satisfies(goArg, constraint) {
			legacy := fmt.Sprintf("Type argument %s does not satisfy Go constraint %s", args[i], params.At(i).Constraint())
			span := c.sourceSpan(loc)
			if len(typeArgs) > i {
				span = c.sourceSpan(declaredTypeLocation(typeArgs[i], loc))
			} else if inferenceSpans[i].FilePath != "" {
				span = inferenceSpans[i]
			}
			c.addDiagnostic(goConstraintDiagnostic{Argument: args[i], Constraint: params.At(i).Constraint().String(), Span: span, LegacyMessage: legacy}.build())
			return nil
		}
	}
	instantiated, err := gotypes.Instantiate(c.goTypesContext, named, goArgs, true)
	if err != nil {
		legacy := fmt.Sprintf("Could not instantiate Go type %s: %s", foreign, err)
		c.addDiagnostic(goTypeInstantiationDiagnostic{Name: foreign.String(), Cause: err.Error(), Span: c.sourceSpan(loc), LegacyMessage: legacy}.build())
		return nil
	}
	instNamed, ok := instantiated.(*gotypes.Named)
	if !ok {
		legacy := fmt.Sprintf("Could not instantiate Go type %s", foreign)
		c.addDiagnostic(goTypeInstantiationDiagnostic{Name: foreign.String(), Span: c.sourceSpan(loc), LegacyMessage: legacy}.build())
		return nil
	}
	inst := foreignNamedTypeFromGo(instNamed, false, true).(*ForeignType)
	inst.TypeArgs = args
	return inst
}

func (c *Checker) checkExprForInference(expr parse.Expression) Expression {
	diagnosticsLen := len(c.diagnostics)
	spansMark := c.spansMark()
	halted := c.halted
	checked := c.checkExpr(expr)
	c.diagnostics = c.diagnostics[:diagnosticsLen]
	c.spansTruncate(spansMark)
	c.halted = halted
	return checked
}

func goTypeMentionsTypeParam(t gotypes.Type, params *gotypes.TypeParamList) bool {
	switch typ := t.(type) {
	case *gotypes.TypeParam:
		for i := 0; i < params.Len(); i++ {
			if params.At(i) == typ {
				return true
			}
		}
	case *gotypes.Slice:
		return goTypeMentionsTypeParam(typ.Elem(), params)
	case *gotypes.Map:
		return goTypeMentionsTypeParam(typ.Key(), params) || goTypeMentionsTypeParam(typ.Elem(), params)
	case *gotypes.Pointer:
		return goTypeMentionsTypeParam(typ.Elem(), params)
	case *gotypes.Array:
		return goTypeMentionsTypeParam(typ.Elem(), params)
	case *gotypes.Signature:
		for i := 0; i < typ.Params().Len(); i++ {
			if goTypeMentionsTypeParam(typ.Params().At(i).Type(), params) {
				return true
			}
		}
		for i := 0; i < typ.Results().Len(); i++ {
			if goTypeMentionsTypeParam(typ.Results().At(i).Type(), params) {
				return true
			}
		}
	case *gotypes.Chan:
		return goTypeMentionsTypeParam(typ.Elem(), params)
	case *gotypes.Named:
		if args := typ.TypeArgs(); args != nil {
			for i := 0; i < args.Len(); i++ {
				if goTypeMentionsTypeParam(args.At(i), params) {
					return true
				}
			}
		}
	}
	return false
}

func exportedGoStructField(strct *gotypes.Struct, name string) *gotypes.Var {
	for i := 0; i < strct.NumFields(); i++ {
		field := strct.Field(i)
		if field.Name() == name && field.Exported() && !field.Embedded() {
			return field
		}
	}
	return nil
}

func (c *Checker) inferGoStructTypeArgs(pattern gotypes.Type, actual Type, goActual gotypes.Type, params *gotypes.TypeParamList, inferred []Type, inferredGo []gotypes.Type, inferredSpans []SourceSpan, loc parse.Location) bool {
	switch pattern := pattern.(type) {
	case *gotypes.TypeParam:
		for i := 0; i < params.Len(); i++ {
			if params.At(i) != pattern {
				continue
			}
			if inferred[i] != nil && !inferred[i].equal(actual) {
				c.addDiagnostic(conflictingGoTypeInferenceDiagnostic{
					Parameter: pattern.Obj().Name(), PreviousType: inferred[i], CurrentType: actual, CurrentSpan: c.sourceSpan(loc), PreviousSpan: sourceSpanIfPresent(inferredSpans[i]),
				}.build())
				return false
			}
			if inferred[i] == nil {
				inferredSpans[i] = c.sourceSpan(loc)
			}
			inferred[i] = actual
			inferredGo[i] = goActual
			return true
		}
	case *gotypes.Slice:
		if list, ok := actual.(*List); ok {
			if goSlice, ok := goActual.Underlying().(*gotypes.Slice); ok {
				return c.inferGoStructTypeArgs(pattern.Elem(), list.Of(), goSlice.Elem(), params, inferred, inferredGo, inferredSpans, loc)
			}
		}
	case *gotypes.Map:
		if m, ok := actual.(*Map); ok {
			goMap, ok := goActual.Underlying().(*gotypes.Map)
			if !ok {
				return true
			}
			if !c.inferGoStructTypeArgs(pattern.Key(), m.Key(), goMap.Key(), params, inferred, inferredGo, inferredSpans, loc) {
				return false
			}
			return c.inferGoStructTypeArgs(pattern.Elem(), m.Value(), goMap.Elem(), params, inferred, inferredGo, inferredSpans, loc)
		}
	}
	return true
}

func checkerTypeToGoType(t Type) (gotypes.Type, bool) {
	switch t {
	case Bool:
		return gotypes.Typ[gotypes.Bool], true
	case Str:
		return gotypes.Typ[gotypes.String], true
	case Int:
		return gotypes.Typ[gotypes.Int], true
	case Int8:
		return gotypes.Typ[gotypes.Int8], true
	case Int16:
		return gotypes.Typ[gotypes.Int16], true
	case Int32:
		return gotypes.Typ[gotypes.Int32], true
	case Int64:
		return gotypes.Typ[gotypes.Int64], true
	case Uint:
		return gotypes.Typ[gotypes.Uint], true
	case Byte:
		return gotypes.Typ[gotypes.Uint8], true
	case Uint16:
		return gotypes.Typ[gotypes.Uint16], true
	case Uint32:
		return gotypes.Typ[gotypes.Uint32], true
	case Uint64:
		return gotypes.Typ[gotypes.Uint64], true
	case Uintptr:
		return gotypes.Typ[gotypes.Uintptr], true
	case Float32:
		return gotypes.Typ[gotypes.Float32], true
	case Float64:
		return gotypes.Typ[gotypes.Float64], true
	case Any:
		return gotypes.NewInterfaceType(nil, nil), true
	}
	switch typ := t.(type) {
	case *ForeignType:
		if typ.GoType != nil {
			return typ.GoType, true
		}
	case *List:
		elem, ok := checkerTypeToGoType(typ.Of())
		if ok {
			return gotypes.NewSlice(elem), true
		}
	case *Map:
		key, ok := checkerTypeToGoType(typ.Key())
		if !ok {
			return nil, false
		}
		value, ok := checkerTypeToGoType(typ.Value())
		if !ok {
			return nil, false
		}
		return gotypes.NewMap(key, value), true
	}
	return nil, false
}

// resolveStructTypeArgs resolves explicit struct literal type arguments
// (`Box<Str>{...}`). Returns ok=false when any argument fails to resolve;
// a diagnostic has been reported in that case.
func (c *Checker) resolveStructTypeArgs(instance *parse.StructInstance) ([]Type, bool) {
	if len(instance.TypeArgs) == 0 {
		return nil, true
	}
	typeArgs := make([]Type, len(instance.TypeArgs))
	for i, arg := range instance.TypeArgs {
		resolved := c.resolveType(arg)
		if resolved == nil {
			c.addUnresolvedReference(unrecognizedType, arg.GetName(), arg.GetLocation())
			return nil, false
		}
		typeArgs[i] = resolved
	}
	return typeArgs, true
}

// validateStructInstance validates struct instantiation and returns the instance or nil if errors
func (c *Checker) validateStructInstance(structType *StructDef, properties []parse.StructValue, structName string, loc parse.Location, typeArgs []Type) *StructInstance {
	instance := &StructInstance{Name: structName, _type: structType}
	if c.spans != nil {
		for _, prop := range properties {
			if _, exists := structType.Fields[prop.Name.Name]; exists {
				c.recordMember(prop.Name.GetLocation(), TargetField, structType, prop.Name.Name, nil)
			}
		}
	}
	fields := make(map[string]Expression)
	fieldTypes := make(map[string]Type)
	providedFields := make(map[string]bool)

	// For generic structs, infer generics from provided field values
	var structDefCopy *StructDef
	var genericScope *SymbolTable
	if structType.hasGenerics() {
		genericParams := append([]string(nil), structType.GenericParams...)
		if len(genericParams) == 0 {
			genericNames := make(map[string]bool)
			for _, fieldType := range structType.Fields {
				extractGenericNames(fieldType, genericNames)
			}
			for name := range genericNames {
				genericParams = append(genericParams, name)
			}
		}

		// Create generic scope and fresh struct copy
		genericScope = c.scope.createGenericScope(genericParams)
		structDefCopy = copyStructWithTypeVarMap(structType, *genericScope.genericContext)

		// Bind explicit type arguments (`Box<Str>{...}`) before checking
		// fields, so unused-in-fields generics resolve and field values are
		// checked against the instantiated types. On any type-argument error,
		// report and fall back to inference so checking can continue.
		if len(typeArgs) > 0 {
			switch {
			case !structType.DeclaredGenerics && len(genericParams) > 1:
				legacy := fmt.Sprintf("Struct %s must declare its generic parameters to take explicit type arguments", structName)
				c.addDiagnostic(invalidStructTypeArgumentsDiagnostic{
					Struct: structName, Actual: len(typeArgs), Reason: "undeclared_order", Span: c.sourceSpan(loc), LegacyMessage: legacy,
				}.build())
			case len(typeArgs) != len(genericParams):
				legacy := fmt.Sprintf("Expected %d type argument(s), got %d", len(genericParams), len(typeArgs))
				c.addDiagnostic(invalidStructTypeArgumentsDiagnostic{
					Struct: structName, Expected: len(genericParams), Actual: len(typeArgs), Reason: "count", Span: c.sourceSpan(loc), LegacyMessage: legacy,
				}.build())
			default:
				for i, actual := range typeArgs {
					if err := genericScope.bindGeneric(genericParams[i], actual); err != nil {
						c.addError(err.Error(), loc)
						break
					}
				}
			}
		}
	} else {
		if len(typeArgs) > 0 {
			legacy := fmt.Sprintf("Struct %s does not take type arguments", structName)
			c.addDiagnostic(invalidStructTypeArgumentsDiagnostic{
				Struct: structName, Actual: len(typeArgs), Reason: "non_generic", Span: c.sourceSpan(loc), LegacyMessage: legacy,
			}.build())
		}
		structDefCopy = structType
	}

	// Check all provided properties
	for _, property := range properties {
		fieldName := property.Name.Name
		var field Type
		var ok bool

		// Get field from the copy (which has fresh TypeVar instances for generics)
		if genericScope != nil {
			field, ok = structDefCopy.Fields[fieldName]
		} else {
			field, ok = structType.Fields[fieldName]
		}

		if !ok {
			c.addUnresolvedReference(unknownStructField, fieldName, property.GetLocation())
		} else {
			providedFields[fieldName] = true

			// A `mut T` (MutableRef) field accepts an existing `mut T` reference value
			// directly (a stored handle), in addition to borrowing a mutable base-type
			// lvalue below (ADR 0031). Try the reference value first.
			if _, ok := mutableRefBase(field); ok {
				diagCount := len(c.diagnostics)
				spansMark := c.spansMark()
				if checked := c.checkExprAs(property.Value, field); checked != nil && checked.Type().equal(field) {
					fields[fieldName] = checked
					fieldTypes[fieldName] = field
					continue
				}
				c.diagnostics = c.diagnostics[:diagCount]
				c.spansTruncate(spansMark)
			}

			fieldExpected := field
			fieldIsMutableRef := false
			if base, ok := mutableRefBase(field); ok {
				fieldExpected = base
				fieldIsMutableRef = true
			}

			// For generic structs, unify types to resolve generics
			if genericScope != nil {
				// Check expression without type context first (let it infer if possible)
				checkVal := c.checkExpr(property.Value)
				if checkVal == nil {
					continue
				}

				// Implicit Maybe wrapping: if field is Maybe<T> and value is T, wrap in Maybe::new()
				if maybeField, isMaybe := fieldExpected.(*Maybe); isMaybe {
					if valType := checkVal.Type(); !valType.equal(fieldExpected) {
						if c.areCompatible(maybeField.Of(), valType) {
							checkVal = c.synthesizeMaybeSome(checkVal, fieldExpected)
						}
					}
				}

				if fieldIsMutableRef && !c.isMutable(checkVal) {
					c.addDiagnostic(typeMismatchDiagnostic{
						Expected:      fieldExpected,
						Actual:        checkVal.Type(),
						ActualSpan:    c.sourceSpan(property.GetLocation()),
						LegacyMessage: fmt.Sprintf("Type mismatch: Expected a mutable %s", fieldExpected.String()),
					}.build())
					continue
				}

				if err := c.unifyTypes(fieldExpected, checkVal.Type(), genericScope); err != nil {
					c.addError(err.Error(), property.GetLocation())
					continue
				}
				// After unification, dereference to get the actual type
				fieldExpected = derefType(fieldExpected)
				if fieldIsMutableRef {
					field = MakeMutableRef(fieldExpected)
				} else {
					field = fieldExpected
				}

				fields[fieldName] = checkVal
				fieldTypes[fieldName] = field
			} else {
				// For non-generic structs, handle nullable fields with implicit wrapping
				var val Expression
				if maybeField, isMaybe := fieldExpected.(*Maybe); isMaybe {
					// Preserve full Maybe<T> type context for expressions like Maybe::new(...)
					// and Maybe::new(), but use the inner type for literals and anonymous
					// functions so they can still infer their element/parameter types.
					switch property.Value.(type) {
					case *parse.ListLiteral, *parse.MapLiteral, *parse.AnonymousFunction:
						val = c.checkExprAs(property.Value, maybeField.Of())
						if val != nil {
							val = c.synthesizeMaybeSome(val, field)
						}
					default:
						diagnosticCount := len(c.diagnostics)
						spansMark := c.spansMark()
						val = c.checkExprAs(property.Value, fieldExpected)
						if val == nil {
							c.diagnostics = c.diagnostics[:diagnosticCount]
							c.spansTruncate(spansMark)
							val = c.checkExpr(property.Value)
							if val != nil && !val.Type().equal(fieldExpected) {
								if c.areCompatible(maybeField.Of(), val.Type()) {
									val = c.synthesizeMaybeSome(val, fieldExpected)
								} else {
									c.addTypeMismatch(fieldExpected, val.Type(), property.GetLocation())
									val = nil
								}
							}
						}
					}
				} else {
					// Non-nullable fields use checkExprAs which provides type context
					diagCount := len(c.diagnostics)
					spansMark := c.spansMark()
					val = c.checkExprAs(property.Value, fieldExpected)
					if val == nil {
						// A mutable-reference lvalue (e.g. a `mut T` field read that
						// deref's to its value type) auto-borrows back into `mut T` so a
						// stored Go pointer handle can satisfy an interface/pointer field
						// (ADR 0031).
						if borrowed := c.checkExpr(property.Value); borrowed != nil {
							if refType := referenceArgType(borrowed); !refType.equal(borrowed.Type()) && c.areCompatible(fieldExpected, refType) {
								c.diagnostics = c.diagnostics[:diagCount]
								c.spansTruncate(spansMark)
								val = borrowed
							}
						}
					}
				}
				if val != nil {
					if fieldIsMutableRef && !c.isMutable(val) {
						c.addDiagnostic(typeMismatchDiagnostic{
							Expected:      fieldExpected,
							Actual:        val.Type(),
							ActualSpan:    c.sourceSpan(property.GetLocation()),
							LegacyMessage: fmt.Sprintf("Type mismatch: Expected a mutable %s", fieldExpected.String()),
						}.build())
						continue
					}
					fields[fieldName] = val
					fieldTypes[fieldName] = field
				}
			}
		}
	}

	// Check for missing required fields
	missing := []string{}
	checkFieldsMap := structDefCopy.Fields
	if structDefCopy == structType {
		checkFieldsMap = structType.Fields
	}

	for name, t := range checkFieldsMap {
		if _, exists := fields[name]; !exists {
			if _, isMaybe := t.(*Maybe); !isMaybe {
				if !providedFields[name] {
					missing = append(missing, name)
				}
			} else {
				// For optional fields, include their type
				fieldTypes[name] = t
			}
		} else if _, exists := fieldTypes[name]; !exists {
			// Pre-compute all field types, not just provided ones
			fieldTypes[name] = t
		}
	}
	if len(missing) > 0 {
		c.addDiagnostic(missingStructFieldsDiagnostic{Fields: missing, Span: c.sourceSpan(loc)}.build())
	}

	instance.Fields = fields
	instance.FieldTypes = fieldTypes
	// Store the refined struct definition (with resolved generics) as the instance's type
	resolvedTypeArgs := make([]Type, len(structDefCopy.TypeArgs))
	for i, typeArg := range structDefCopy.TypeArgs {
		resolvedTypeArgs[i] = derefType(typeArg)
	}
	genericParams := append([]string(nil), structDefCopy.GenericParams...)
	if len(genericParams) == 0 {
		genericParams = nil
	}
	instance._type = &StructDef{
		Name:             structDefCopy.Name,
		ModulePath:       structDefCopy.ModulePath,
		Fields:           fieldTypes,
		Self:             structDefCopy.Self,
		Traits:           structDefCopy.Traits,
		GenericParams:    genericParams,
		DeclaredGenerics: structDefCopy.DeclaredGenerics,
		TypeArgs:         resolvedTypeArgs,
		Private:          structDefCopy.Private,
	}
	instance.StructType = instance._type
	return instance
}

// createPrimitiveMethodNode creates type-specific method nodes for primitives and collections
// Falls back to generic InstanceMethod for user-defined types (structs, enums)
func (c *Checker) createPrimitiveMethodNode(subject Expression, methodName string, args []Expression, fnDef *FunctionDef, typeArgs []Type, loc parse.Location) Expression {
	// Determine subject type - emit specialized nodes for all built-in types
	switch subject.Type() {
	case Str:
		return c.createStrMethod(subject, methodName, args)
	case Int:
		return c.createIntMethod(subject, methodName)
	case Byte:
		return c.createByteMethod(subject, methodName)
	case Rune:
		return c.createRuneMethod(subject, methodName)
	case Float64:
		return c.createFloatMethod(subject, methodName)
	case Bool:
		return c.createBoolMethod(subject, methodName)
	}

	// Check for collection types
	if _, isList := subject.Type().(*List); isList {
		return c.createListMethod(subject, methodName, args, fnDef)
	}
	if _, isFixedArray := subject.Type().(*FixedArray); isFixedArray {
		return c.createListMethod(subject, methodName, args, fnDef)
	}
	if _, isMap := subject.Type().(*Map); isMap {
		return c.createMapMethod(subject, methodName, args, fnDef)
	}
	if foreign, isForeign := subject.Type().(*ForeignType); isForeign && foreign.MapKey != nil && foreign.MapValue != nil {
		return c.createMapMethod(subject, methodName, args, fnDef)
	}
	if foreign, isForeign := subject.Type().(*ForeignType); isForeign && foreign.Elem != nil {
		return c.createListMethod(subject, methodName, args, fnDef)
	}
	if foreign, isForeign := subject.Type().(*ForeignType); isForeign {
		if _, ok := foreign.Underlying.(*FixedArray); ok {
			return c.createListMethod(subject, methodName, args, fnDef)
		}
	}
	if _, isMaybe := subject.Type().(*Maybe); isMaybe {
		return c.createMaybeMethod(subject, methodName, args, fnDef, loc)
	}
	if _, isResult := subject.Type().(*Result); isResult {
		return c.createResultMethod(subject, methodName, args, fnDef)
	}

	// For user-defined types (structs, enums), use generic InstanceMethod
	receiverKind := ReceiverUnknown
	var structType *StructDef
	var enumType *Enum
	var traitType *Trait
	switch receiver := subject.Type().(type) {
	case *StructDef:
		receiverKind = ReceiverStruct
		structType = receiver
	case *Enum:
		receiverKind = ReceiverEnum
		enumType = receiver
	case *Trait:
		receiverKind = ReceiverTrait
		traitType = receiver
	}
	return &InstanceMethod{
		Subject: subject,
		Method: &FunctionCall{
			Name:       methodName,
			Args:       args,
			TypeArgs:   typeArgs,
			fn:         fnDef,
			ReturnType: fnDef.ReturnType,
		},
		ReceiverKind: receiverKind,
		StructType:   structType,
		EnumType:     enumType,
		TraitType:    traitType,
	}
}

// checkStrStatic resolves built-in static functions on the Str type, such as
// Str::from (build a Str from a [Byte] or [Rune] view). It returns
// handled=false when the name is not a known Str static so the caller can
// continue normal resolution. (#283)
func (c *Checker) checkStrStatic(s *parse.StaticFunction) (Expression, bool) {
	switch s.Function.Name {
	case "from":
		if len(s.Function.TypeArgs) > 0 {
			c.addInvalidFunctionTypeArguments("Str::from", 0, len(s.Function.TypeArgs), false, s.GetLocation(), "Str::from does not take type arguments")
			return nil, true
		}
		if len(s.Function.Args) != 1 {
			c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
			return nil, true
		}
		// Str::from(bytes) and Str::from(runes) both build a Str, mirroring Go's
		// string([]byte) / string([]rune). The byte form is unchecked (invalid
		// UTF-8 is carried through, like Go); validate with unicode/utf8 in
		// userland when the boundary can produce invalid bytes.
		argNode := s.Function.Args[0].Value
		var arg Expression
		if lit, ok := argNode.(*parse.ListLiteral); ok && len(lit.Items) == 0 {
			// An empty list literal has no element type to infer; default to bytes.
			arg = c.checkExprAs(argNode, MakeList(Byte))
		} else {
			arg = c.checkExpr(argNode)
		}
		if arg == nil {
			return nil, true
		}
		if list, ok := arg.Type().(*List); ok && (list.Of().equal(Byte) || list.Of().equal(Rune)) {
			return &ScalarFrom{Value: arg, Target: Str}, true
		}
		c.addError(fmt.Sprintf("Str::from expects [Byte] or [Rune], got %s", arg.Type().String()), s.Function.Args[0].GetLocation())
		return nil, true
	}
	return nil, false
}

// checkScalarFrom checks a `T::from(value)` conversion into the sized/named
// scalar `target`. The conversion is truncating like Go's `T(x)`: integer
// targets accept an integer-like value, float targets accept a numeric value,
// and the result is `target` (never optional). (#284)
func (c *Checker) checkScalarFrom(s *parse.StaticFunction, target Type) Expression {
	if len(s.Function.TypeArgs) > 0 {
		name := target.String() + "::from"
		c.addInvalidFunctionTypeArguments(name, 0, len(s.Function.TypeArgs), false, s.GetLocation(), name+" does not take type arguments")
		return nil
	}
	if len(s.Function.Args) != 1 {
		c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
		return nil
	}
	// The value type is the target itself for a bare scalar, or the foreign
	// type's underlying sized primitive. Typing the argument against it makes
	// numeric literals adopt and range-check against the target (a constant
	// that overflows is an error, matching Go's constant conversion), while
	// runtime values pass through to be truncated at the Go boundary.
	valueType := target
	if prim := foreignScalarPrimitive(target); prim != nil {
		valueType = prim
	}
	// Defensive: only numeric targets convert. Foreign named types over Str or
	// Bool underlyings are not numeric and must not reach here.
	if !isNumericScalar(valueType) {
		c.addError(fmt.Sprintf("%s::from requires a numeric type", target.String()), s.GetLocation())
		return nil
	}
	floatTarget := isFloatScalar(valueType)
	argNode := s.Function.Args[0].Value
	var arg Expression
	if isNumericLiteralNode(argNode) {
		// A numeric literal adopts and range-checks against the target, so a
		// constant that overflows is reported here (matching Go's constant
		// conversion) instead of leaking a generated-Go compile error.
		arg = c.checkExprAs(argNode, valueType)
	} else {
		// A runtime value keeps its own type and is truncated at the Go
		// boundary, matching Go's `T(x)` for non-constant x.
		arg = c.checkExpr(argNode)
	}
	if arg == nil {
		return nil
	}
	argType := arg.Type()
	ok := isRelationalIntegerLike(argType)
	if floatTarget {
		ok = ok || isRelationalFloatLike(argType)
	}
	if !ok {
		c.addError(fmt.Sprintf("%s::from expects a numeric value, got %s", target.String(), argType.String()), s.Function.Args[0].GetLocation())
		return nil
	}
	return &ScalarFrom{Value: arg, Target: target}
}

func isFloatScalar(t Type) bool {
	return t == Float64 || t == Float32
}

// isNumericScalar reports whether t is an integer or float scalar (excluding
// Str/Bool-underlying types), i.e. a valid `T::from` conversion target.
func isNumericScalar(t Type) bool {
	return isIntegerScalar(t) || isFloatScalar(t)
}

func isNumericLiteralNode(e parse.Expression) bool {
	switch n := e.(type) {
	case *parse.NumLiteral:
		return true
	case *parse.UnaryExpression:
		return n.Operator == parse.Minus && isNumericLiteralNode(n.Operand)
	}
	return false
}

func (c *Checker) createStrMethod(subject Expression, methodName string, args []Expression) Expression {
	var kind StrMethodKind
	switch methodName {
	case "at":
		kind = StrAt
	case "size":
		kind = StrSize
	case "bytes":
		kind = StrBytes
	case "runes":
		kind = StrRunes
	case "is_empty":
		kind = StrIsEmpty
	case "contains":
		kind = StrContains
	case "replace":
		kind = StrReplace
	case "replace_all":
		kind = StrReplaceAll
	case "starts_with":
		kind = StrStartsWith
	case "ends_with":
		kind = StrEndsWith
	case "to_str":
		kind = StrToStr
	case "trim":
		kind = StrTrim
	default:
		// Fallback for unknown methods
		panic(fmt.Sprintf("Unknown Str method: %s", methodName))
	}
	return &StrMethod{
		Subject: subject,
		Kind:    kind,
		Args:    args,
	}
}

func (c *Checker) createByteMethod(subject Expression, methodName string) Expression {
	var kind ByteMethodKind
	switch methodName {
	case "to_int":
		kind = ByteToInt
	case "to_str":
		kind = ByteToStr
	default:
		panic(fmt.Sprintf("Unknown Byte method: %s", methodName))
	}
	return &ByteMethod{Subject: subject, Kind: kind}
}

func (c *Checker) createRuneMethod(subject Expression, methodName string) Expression {
	var kind RuneMethodKind
	switch methodName {
	case "to_int":
		kind = RuneToInt
	case "to_str":
		kind = RuneToStr
	default:
		panic(fmt.Sprintf("Unknown Rune method: %s", methodName))
	}
	return &RuneMethod{Subject: subject, Kind: kind}
}

func (c *Checker) createIntMethod(subject Expression, methodName string) Expression {
	var kind IntMethodKind
	switch methodName {
	case "to_str":
		kind = IntToStr
	case "to_f64":
		kind = IntToF64
	default:
		panic(fmt.Sprintf("Unknown Int method: %s", methodName))
	}
	return &IntMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createFloatMethod(subject Expression, methodName string) Expression {
	var kind FloatMethodKind
	switch methodName {
	case "to_str":
		kind = FloatToStr
	case "to_int":
		kind = FloatToInt
	default:
		panic(fmt.Sprintf("Unknown Float64 method: %s", methodName))
	}
	return &FloatMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createBoolMethod(subject Expression, methodName string) Expression {
	var kind BoolMethodKind
	switch methodName {
	case "to_str":
		kind = BoolToStr
	default:
		panic(fmt.Sprintf("Unknown Bool method: %s", methodName))
	}
	return &BoolMethod{
		Subject: subject,
		Kind:    kind,
	}
}

func (c *Checker) createListMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	var elemType Type
	if listType, ok := subject.Type().(*List); ok {
		elemType = listType.of
	} else if arrayType, ok := subject.Type().(*FixedArray); ok {
		elemType = arrayType.of
	} else if foreign, ok := subject.Type().(*ForeignType); ok && foreign.Elem != nil {
		elemType = foreign.Elem
	} else if foreign, ok := subject.Type().(*ForeignType); ok {
		if arrayType, ok := foreign.Underlying.(*FixedArray); ok {
			elemType = arrayType.of
		} else {
			panic(fmt.Sprintf("List method on non-list type: %s", subject.Type()))
		}
	} else {
		panic(fmt.Sprintf("List method on non-list type: %s", subject.Type()))
	}
	var kind ListMethodKind
	switch methodName {
	case "at":
		kind = ListAt
	case "prepend":
		kind = ListPrepend
	case "push":
		kind = ListPush
	case "set":
		kind = ListSet
	case "size":
		kind = ListSize
	case "sort":
		kind = ListSort
	case "swap":
		kind = ListSwap
	default:
		panic(fmt.Sprintf("Unknown List method: %s", methodName))
	}
	return &ListMethod{
		Subject:     subject,
		Kind:        kind,
		Args:        args,
		ElementType: elemType,
		fn:          fnDef,
	}
}

func isListMethodName(name string) bool {
	switch name {
	case "at", "prepend", "push", "set", "size", "sort", "swap":
		return true
	default:
		return false
	}
}

func isMapMethodName(name string) bool {
	switch name {
	case "keys", "size", "get", "set", "delete", "has":
		return true
	default:
		return false
	}
}

func (c *Checker) createMapMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	var keyType Type
	var valueType Type
	if mapType, ok := subject.Type().(*Map); ok {
		keyType = mapType.Key()
		valueType = mapType.Value()
	} else if foreign, ok := subject.Type().(*ForeignType); ok && foreign.MapKey != nil && foreign.MapValue != nil {
		keyType = foreign.MapKey
		valueType = foreign.MapValue
	} else {
		panic(fmt.Sprintf("Map method on non-map type: %s", subject.Type()))
	}
	var kind MapMethodKind
	switch methodName {
	case "keys":
		kind = MapKeys
	case "size":
		kind = MapSize
	case "get":
		kind = MapGet
	case "set":
		kind = MapSet
	case "delete":
		kind = MapDelete
	case "has":
		kind = MapHas
	default:
		panic(fmt.Sprintf("Unknown Map method: %s", methodName))
	}
	return &MapMethod{
		Subject:   subject,
		Kind:      kind,
		Args:      args,
		KeyType:   keyType,
		ValueType: valueType,
		fn:        fnDef,
	}
}

func (c *Checker) createMaybeMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef, loc parse.Location) Expression {
	maybeType := subject.Type().(*Maybe)
	var kind MaybeMethodKind
	switch methodName {
	case "expect":
		kind = MaybeExpect
	case "is_none":
		kind = MaybeIsNone
	case "is_some":
		kind = MaybeIsSome
	case "or":
		kind = MaybeOr
	case "map":
		kind = MaybeMap
	case "and_then":
		kind = MaybeAndThen
	case "set":
		kind = MaybeSet
	case "clear":
		kind = MaybeClear
	default:
		panic(fmt.Sprintf("Unknown Maybe method: %s", methodName))
	}
	if (kind == MaybeSet || kind == MaybeClear) && !c.isMutable(subject) {
		c.addDiagnostic(immutableReceiverDiagnostic{
			Kind:            immutableMaybeReceiver,
			Receiver:        "Maybe",
			Method:          methodName,
			Span:            c.sourceSpan(loc),
			DeclarationSpan: expressionBindingSpan(subject),
		}.build())
		return nil
	}
	return &MaybeMethod{
		Subject:    subject,
		Kind:       kind,
		Args:       args,
		InnerType:  maybeType.Of(),
		fn:         fnDef,
		ReturnType: fnDef.ReturnType,
	}
}

func (c *Checker) createResultMethod(subject Expression, methodName string, args []Expression, fnDef *FunctionDef) Expression {
	resultType := subject.Type().(*Result)
	var kind ResultMethodKind
	switch methodName {
	case "expect":
		kind = ResultExpect
	case "or":
		kind = ResultOr
	case "is_ok":
		kind = ResultIsOk
	case "is_err":
		kind = ResultIsErr
	case "map":
		kind = ResultMap
	case "map_err":
		kind = ResultMapErr
	case "and_then":
		kind = ResultAndThen
	default:
		panic(fmt.Sprintf("Unknown Result method: %s", methodName))
	}
	return &ResultMethod{
		Subject:    subject,
		Kind:       kind,
		Args:       args,
		OkType:     resultType.Val(),
		ErrType:    resultType.Err(),
		fn:         fnDef,
		ReturnType: fnDef.ReturnType,
	}
}

// extractGenericBindingsFromSpecializedStruct builds a mapping from generic parameter names to their resolved types
// by comparing the original struct definition with a specialized version of it.
// For example, if the original struct is `Box { item: $T }` and the specialized has `item: Int`,
// this returns `{$T: Int}`.
func (c *Checker) extractGenericBindingsFromSpecializedStruct(originalDef, specializedDef *StructDef) map[string]Type {
	bindings := make(map[string]Type)

	if originalDef == nil || specializedDef == nil {
		return bindings
	}

	for i, param := range originalDef.GenericParams {
		if i < len(specializedDef.TypeArgs) {
			bindings[param] = derefType(specializedDef.TypeArgs[i])
		}
	}

	// Now compare original field types with specialized field types
	// For each field that contains a generic in the original, extract its resolved type
	for fieldName, originalFieldType := range originalDef.Fields {
		specializedFieldType, ok := specializedDef.Fields[fieldName]
		if !ok {
			continue
		}

		bindGenericTypes(originalFieldType, specializedFieldType, bindings)
	}

	return bindings
}

func bindGenericTypes(original Type, specialized Type, bindings map[string]Type) {
	original = deref(original)
	specialized = deref(specialized)
	switch orig := original.(type) {
	case *TypeVar:
		if orig.name != "" {
			if _, alreadyBound := bindings[orig.name]; !alreadyBound {
				bindings[orig.name] = specialized
			}
		}
	case *List:
		if spec, ok := specialized.(*List); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Chan:
		if spec, ok := specialized.(*Chan); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Receiver:
		if spec, ok := specialized.(*Receiver); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Sender:
		if spec, ok := specialized.(*Sender); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Map:
		if spec, ok := specialized.(*Map); ok {
			bindGenericTypes(orig.key, spec.key, bindings)
			bindGenericTypes(orig.value, spec.value, bindings)
		}
	case *Maybe:
		if spec, ok := specialized.(*Maybe); ok {
			bindGenericTypes(orig.of, spec.of, bindings)
		}
	case *Result:
		if spec, ok := specialized.(*Result); ok {
			bindGenericTypes(orig.val, spec.val, bindings)
			bindGenericTypes(orig.err, spec.err, bindings)
		}
	case *StructDef:
		if spec, ok := specialized.(*StructDef); ok {
			for i, originalArg := range orig.TypeArgs {
				if i < len(spec.TypeArgs) {
					bindGenericTypes(originalArg, spec.TypeArgs[i], bindings)
				}
			}
			for name, originalField := range orig.Fields {
				if specializedField, ok := spec.Fields[name]; ok {
					bindGenericTypes(originalField, specializedField, bindings)
				}
			}
		}
	case *FunctionDef:
		if spec, ok := specialized.(*FunctionDef); ok {
			for i := range orig.Parameters {
				if i >= len(spec.Parameters) {
					break
				}
				bindGenericTypes(orig.Parameters[i].Type, spec.Parameters[i].Type, bindings)
			}
			bindGenericTypes(orig.ReturnType, spec.ReturnType, bindings)
		}
	}
}

// ifChainHasElse reports whether an if chain ends in a final else branch.
// A chain ending in `else if` has no final else.
func ifChainHasElse(s *parse.IfStatement) bool {
	current := s
	for current != nil {
		if current.Else == nil {
			return false
		}
		next, chained := current.Else.(*parse.IfStatement)
		if !chained {
			return true
		}
		current = next
	}
	return false
}

func (c *Checker) checkIfChain(s *parse.IfStatement) Expression {
	if s == nil || s.Condition == nil {
		return nil
	}
	branches := []IfBranch{}
	var elseBlock *Block
	var referenceType Type
	var referenceSpan parse.Location
	current := s
	for current != nil {
		if current.Condition == nil {
			expectedType := c.expectedExpr
			c.expectedExpr = nil
			block := c.checkBlockWithExpected(current.Body, nil, expectedType, false)
			c.expectedExpr = expectedType
			if referenceType != nil && !block.Type().equal(referenceType) {
				if referenceType == Void || block.Type() == Void {
					if referenceType != Void {
						for i := range branches {
							if branches[i].Body != nil && branches[i].Body.Type() != Void {
								branches[i].Body.DiscardFinalValue = true
							}
						}
					}
					if block.Type() != Void {
						block.DiscardFinalValue = true
					}
					referenceType = Void
				} else {
					c.addDiagnostic(branchTypeMismatchDiagnostic{
						Expected:      referenceType,
						Actual:        block.Type(),
						ExpectedSpan:  c.sourceSpanPtr(referenceSpan),
						ActualSpan:    c.sourceSpan(bodyResultLocation(current.Body, current.GetLocation())),
						LegacyMessage: "All branches must have the same result type",
						Title:         "Incompatible if branch types",
					}.build())
					return nil
				}
			}
			elseBlock = block
			break
		}
		condition := c.checkExpr(current.Condition)
		if condition == nil {
			return nil
		}
		if condition.Type() != Bool {
			c.addDiagnostic(nonBooleanIfConditionDiagnostic{Actual: condition.Type(), Span: c.sourceSpan(current.Condition.GetLocation())}.build())
			return nil
		}
		expectedType := c.expectedExpr
		c.expectedExpr = nil
		body := c.checkBlockWithExpected(current.Body, nil, expectedType, false)
		c.expectedExpr = expectedType
		if referenceType == nil {
			referenceType = body.Type()
			referenceSpan = bodyResultLocation(current.Body, current.GetLocation())
		} else if !body.Type().equal(referenceType) {
			if referenceType == Void || body.Type() == Void {
				if referenceType != Void {
					for i := range branches {
						if branches[i].Body != nil && branches[i].Body.Type() != Void {
							branches[i].Body.DiscardFinalValue = true
						}
					}
				}
				if body.Type() != Void {
					body.DiscardFinalValue = true
				}
				referenceType = Void
			} else {
				c.addDiagnostic(branchTypeMismatchDiagnostic{
					Expected:      referenceType,
					Actual:        body.Type(),
					ExpectedSpan:  c.sourceSpanPtr(referenceSpan),
					ActualSpan:    c.sourceSpan(bodyResultLocation(current.Body, current.GetLocation())),
					LegacyMessage: "All branches must have the same result type",
					Title:         "Incompatible if branch types",
				}.build())
				return nil
			}
		}
		branches = append(branches, IfBranch{Condition: condition, Body: body})
		next, ok := current.Else.(*parse.IfStatement)
		if !ok {
			break
		}
		current = next
	}
	return &If{Branches: branches, Else: elseBlock}
}

func functionDefForCallableType(typ Type) (*FunctionDef, bool) {
	typ = derefType(typ)
	switch fn := typ.(type) {
	case *FunctionDef:
		return fn, true
	case *ForeignType:
		// A named Go func type is callable with its underlying signature,
		// matching Go's own call rules for named func values.
		if !fn.Pointer {
			if underlying, ok := fn.Underlying.(*FunctionDef); ok {
				return underlying, true
			}
		}
		return nil, false
	default:
		return nil, false
	}
}

func (c *Checker) checkFunctionValueCall(callee Expression, callArgs []parse.Argument, typeArgs []parse.DeclaredType, location parse.Location, displayName string) Expression {
	fnDef, ok := functionDefForCallableType(callee.Type())
	if !ok {
		c.addNonCallable(displayName, location, expressionBindingSpan(callee), nonCallableSuffix)
		return nil
	}

	callTypeArgs := c.resolveCallTypeArgs(typeArgs)
	resolvedExprs, err := c.resolveArguments(callArgs, fnDef.Parameters)
	if err != nil {
		c.addArgumentBindingError(err, location)
		return nil
	}

	numOmittedArgs := 0
	if len(resolvedExprs) < len(fnDef.Parameters) {
		for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
			if !parameterOmittable(fnDef.Parameters[i]) {
				c.addMissingArgument(fnDef.Parameters[i], location)
				return nil
			}
		}
		numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
	} else if len(resolvedExprs) > len(fnDef.Parameters) {
		c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(resolvedExprs), location, "")
		resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
	}

	fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)
	args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
	if args == nil {
		return nil
	}
	if len(callTypeArgs) > 0 {
		specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, location)
		if err != nil {
			c.addGenericFunctionResolutionError(err, location)
			return nil
		}
		fnToUse = specialized
	}

	return &FunctionValueCall{
		Callee:       callee,
		Args:         args,
		FunctionType: fnToUse,
		ReturnType:   fnToUse.ReturnType,
	}
}

func (c *Checker) checkFunctionFieldCall(subject Expression, method parse.FunctionCall, location parse.Location) (Expression, bool) {
	if subject == nil || subject.Type() == nil {
		return nil, false
	}
	fieldType := subject.Type().get(method.Name)
	if fieldType == nil {
		return nil, false
	}
	field := &InstanceProperty{
		Subject:  subject,
		Property: method.Name,
		_type:    fieldType,
	}
	if _, ok := subject.Type().(*StructDef); ok {
		field.Kind = StructSubject
	}
	return c.checkFunctionValueCall(field, method.Args, method.TypeArgs, location, fmt.Sprintf("%s.%s", subject, method.Name)), true
}

func comparisonOperatorText(operator parse.Operator) string {
	switch operator {
	case parse.GreaterThan:
		return ">"
	case parse.GreaterThanOrEqual:
		return ">="
	case parse.LessThan:
		return "<"
	case parse.LessThanOrEqual:
		return "<="
	case parse.Equal:
		return "=="
	case parse.NotEqual:
		return "!="
	default:
		return fmt.Sprint(operator)
	}
}

func (c *Checker) addInvalidArithmetic(operator string, left, right Expression, leftLoc, rightLoc parse.Location, legacy string, unsupported bool) {
	c.addDiagnostic(invalidArithmeticDiagnostic{
		Operator: operator, LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(leftLoc), RightSpan: c.sourceSpan(rightLoc), LegacyMessage: legacy, Unsupported: unsupported,
	}.build())
}

func (c *Checker) addInvalidRelational(operator string, left, right Expression, leftLoc, rightLoc parse.Location, legacy string) {
	c.addDiagnostic(invalidRelationalDiagnostic{
		Operator: operator, LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(leftLoc), RightSpan: c.sourceSpan(rightLoc), LegacyMessage: legacy, Unsupported: left.Type().equal(right.Type()),
	}.build())
}

func (c *Checker) addInvalidEquality(operator string, left, right Expression, leftLoc, rightLoc parse.Location, legacy string, unsupported bool) {
	c.addDiagnostic(invalidEqualityDiagnostic{
		Operator: operator, LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(leftLoc), RightSpan: c.sourceSpan(rightLoc), LegacyMessage: legacy, Unsupported: unsupported,
	}.build())
}

func (c *Checker) checkExpr(expr parse.Expression) Expression {
	result := c.checkExprInner(expr)
	if result != nil {
		c.recordExprSpan(expr, result)
	}
	return result
}

func (c *Checker) checkExprInner(expr parse.Expression) Expression {
	if c.halted {
		return nil
	}
	discardThisExpr := c.discardExprContext
	previousDiscard := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previousDiscard
	}()

	switch s := (expr).(type) {
	case *parse.StrLiteral:
		return &StrLiteral{s.Value}
	case *parse.RuneLiteral:
		runes := []rune(s.Value)
		if len(runes) != 1 || !utf8.ValidRune(runes[0]) {
			c.addError("Rune literal must contain exactly one Unicode scalar value", s.GetLocation())
			return &RuneLiteral{Value: 0}
		}
		return &RuneLiteral{Value: runes[0]}
	case *parse.BoolLiteral:
		return &BoolLiteral{s.Value}
	case *parse.VoidLiteral:
		return &VoidLiteral{}
	case *parse.NumLiteral:
		{
			stripped := strings.ReplaceAll(s.Value, "_", "")
			if strings.Contains(stripped, ".") {
				value, err := strconv.ParseFloat(stripped, 64)
				if err != nil {
					c.addError(fmt.Sprintf("Invalid float: %s", s.Value), s.GetLocation())
					return &FloatLiteral{Value: 0.0}
				}
				return &FloatLiteral{Value: value}
			}
			value64, err := strconv.ParseInt(stripped, 0, 64)
			if err != nil {
				c.addError(fmt.Sprintf("Invalid int: %s", s.Value), s.GetLocation())
				return &IntLiteral{0}
			}
			if !c.intLiteralFitsType(value64, Int) {
				c.addError(fmt.Sprintf("Integer literal %s overflows Int", s.Value), s.GetLocation())
			}
			return &IntLiteral{int(value64)}
		}
	case *parse.InterpolatedStr:
		{
			chunks := make([]Expression, len(s.Chunks))
			for i := range s.Chunks {
				cx := c.checkExpr(s.Chunks[i])
				if cx == nil {
					// skip bad expressions
					chunks[i] = &StrLiteral{}
					continue
				}

				// A foreign named scalar stringifies as its underlying primitive
				// (e.g. term::EventTitle interpolates as its Str value).
				if prim := foreignScalarPrimitive(cx.Type()); prim != nil {
					cx = &ForeignScalarConvert{Value: cx, Target: prim}
				}

				// If chunk is a string, use it directly
				if cx.Type() == Str {
					chunks[i] = cx
					continue
				}

				if toStr, ok := cx.Type().get("to_str").(*FunctionDef); ok && toStr.ReturnType == Str && len(toStr.Parameters) == 0 {
					chunks[i] = c.createPrimitiveMethodNode(cx, toStr.Name, []Expression{}, toStr, nil, parse.Location{})
					continue
				}

				if strMod := c.findModuleByPath("ard/string"); strMod != nil {
					toStringTrait := strMod.Get("ToString").Type.(*Trait)
					if !cx.Type().hasTrait(toStringTrait) {
						c.addTypeMismatch(toStringTrait, cx.Type(), s.Chunks[i].GetLocation())
						// a non-stringable chunk stays empty
						chunks[i] = &StrLiteral{}
						continue
					}

					// For non-string types that satisfy ToString trait, wrap with to_str() call
					toStrMethod := toStringTrait.methods[0]
					methodNode := c.createPrimitiveMethodNode(cx, toStrMethod.Name, []Expression{}, &toStrMethod, nil, parse.Location{})
					chunks[i] = methodNode
					continue
				}

				c.addDiagnostic(stringInterpolationMismatchDiagnostic{
					Actual: cx.Type(),
					Span:   c.sourceSpan(s.Chunks[i].GetLocation()),
				}.build())
				chunks[i] = &StrLiteral{}
			}
			return &TemplateStr{chunks}
		}
	case *parse.Identifier:
		if sym, ok := c.scope.get(s.Name); ok {
			c.recordSymbolUse(s, sym, nil)
			return &Variable{*sym}
		}
		c.addDiagnostic(undefinedNameDiagnostic{
			Kind: undefinedVariable,
			Name: s.Name,
			Span: c.sourceSpan(s.GetLocation()),
		}.build())
		c.halted = true
		return nil
	case *parse.FunctionValueCall:
		{
			callee := c.checkExpr(s.Callee)
			if callee == nil {
				return nil
			}
			return c.checkFunctionValueCall(callee, s.Args, s.TypeArgs, s.GetLocation(), s.Callee.String())
		}
	case *parse.FunctionCall:
		{
			if s.Name == "panic" {
				if len(s.TypeArgs) > 0 {
					c.addInvalidFunctionTypeArguments("panic", 0, len(s.TypeArgs), false, s.GetLocation(), "")
					return nil
				}
				if len(s.Args) != 1 {
					c.addArgumentCount("1", len(s.Args), s.GetLocation(), "Incorrect number of arguments: 'panic' requires a message")
					return nil
				}
				message := c.checkExpr(s.Args[0].Value)
				if message == nil {
					return nil
				}

				return &Panic{
					Message: message,
					node:    s,
				}
			}

			// Find the function in the scope
			fnSym, got := c.scope.get(s.Name)
			if !got {
				c.addDiagnostic(undefinedNameDiagnostic{
					Kind: undefinedFunction,
					Name: s.Name,
					Span: c.sourceSpan(s.GetLocation()),
				}.build())
				return nil
			}

			// Cast to FunctionDef
			var fnDef *FunctionDef
			var ok bool

			// Try different types for the function symbol, including named Go
			// func values, which call through their underlying signature.
			fnDef, ok = functionDefForCallableType(fnSym.Type)
			if !ok {
				c.addNonCallable(s.Name, s.GetLocation(), sourceSpanIfPresent(fnSym.declaredAt), nonCallablePrefix)
				return nil
			}
			c.recordCallAttempt(s, s.Name, fnDef)

			callTypeArgs := c.resolveCallTypeArgs(s.TypeArgs)

			// Resolve named arguments to positional arguments (for expressions only)
			resolvedExprs, err := c.resolveArguments(s.Args, fnDef.Parameters)
			if err != nil {
				c.addArgumentBindingError(err, s.GetLocation())
				return nil
			}

			// Check argument count after resolving
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					if !parameterOmittable(fnDef.Parameters[i]) {
						c.addMissingArgument(fnDef.Parameters[i], s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) {
				c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(resolvedExprs), s.GetLocation(), "")
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if len(callTypeArgs) > 0 {
				specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
				if err != nil {
					c.addGenericFunctionResolutionError(err, s.GetLocation())
					return nil
				}
				fnToUse = specialized
			}

			call := &FunctionCall{
				Name:       s.Name,
				Args:       args,
				TypeArgs:   callTypeArgs,
				fn:         fnToUse,
				ReturnType: fnToUse.ReturnType,
			}
			return call
		}
	case *parse.InstanceProperty:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				// panic(fmt.Errorf("Cannot access %s on nil", s.Property))
				return nil
			}

			if foreign, ok := subj.Type().(*ForeignType); ok {
				if !foreign.FieldsLoaded && foreign.LoadFields != nil {
					foreign.Fields, foreign.UnsupportedFields = foreign.LoadFields()
					foreign.FieldsLoaded = true
				}
				if reason := foreign.UnsupportedFields[s.Property.Name]; reason != "" {
					c.addUnsupportedGoEntity("field", fmt.Sprintf("%s.%s", foreign, s.Property.Name), reason, "Unsupported foreign field", s.Property.GetLocation())
					return nil
				}
				if fieldType := foreign.Fields[s.Property.Name]; fieldType != nil {
					return &ForeignFieldAccess{Subject: subj, Target: foreign.Target, Symbol: s.Property.Name, _type: fieldType}
				}
			}

			propType := subj.Type().get(s.Property.Name)
			foreignPointerReceiver := false
			if propType == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok && !foreign.Pointer {
					pointerForeign := *foreign
					pointerForeign.Pointer = true
					pointerForeign.Methods = foreign.PointerMethods
					pointerForeign.UnsupportedMethods = foreign.UnsupportedPointerMethods
					pointerForeign.MethodsLoaded = pointerForeign.Methods != nil || pointerForeign.UnsupportedMethods != nil
					if pointerSig := pointerForeign.get(s.Property.Name); pointerSig != nil {
						if !c.isMutable(subj) {
							c.addDiagnostic(immutableReceiverDiagnostic{
								Kind:            immutablePointerMethodAccess,
								Receiver:        foreign.String(),
								Method:          s.Property.Name,
								Span:            c.sourceSpan(s.Property.GetLocation()),
								DeclarationSpan: expressionBindingSpan(subj),
							}.build())
							return nil
						}
						propType = pointerSig
						foreignPointerReceiver = true
					} else if reason := pointerForeign.UnsupportedMethods[s.Property.Name]; reason != "" {
						c.addUnsupportedGoEntity("method", fmt.Sprintf("%s.%s", foreign, s.Property.Name), reason, "Unsupported foreign method", s.Property.GetLocation())
						return nil
					}
				}
			}
			if propType == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					if reason := foreign.UnsupportedMethods[s.Property.Name]; reason != "" {
						c.addUnsupportedGoEntity("method", fmt.Sprintf("%s.%s", foreign, s.Property.Name), reason, "Unsupported foreign method", s.Property.GetLocation())
						return nil
					}
				}
				c.addDiagnostic(undefinedMemberDiagnostic{
					Kind:     undefinedField,
					Receiver: fmt.Sprint(subj),
					Member:   s.Property.Name,
					Span:     c.sourceSpan(s.Property.GetLocation()),
				}.build())
				return nil
			}

			if fnDef, ok := propType.(*FunctionDef); ok {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					pointer := foreign.Pointer || foreignPointerReceiver
					return &ForeignMethodValue{Subject: subj, Target: foreign.Target, Namespace: foreign.Namespace, Qualifier: foreign.Qualifier, Receiver: foreign.Name, Pointer: pointer, Symbol: s.Property.Name, _type: fnDef}
				}
			}

			prop := &InstanceProperty{
				Subject:  subj,
				Property: s.Property.Name,
				_type:    propType,
			}
			c.recordMember(s.Property.GetLocation(), TargetField, subj.Type(), s.Property.Name, prop)

			// Pre-compute which kind of property this is based on subject type
			switch subjType := subj.Type().(type) {
			case *StructDef:
				prop.Kind = StructSubject
			case *MutableRef:
				if _, ok := subjType.Of().(*StructDef); ok {
					prop.Kind = StructSubject
				} else {
					c.addError(fmt.Sprintf("Cannot access property on type %s", subj.Type()), s.Property.GetLocation())
				}
			default:
				// unreachable
				c.addError(fmt.Sprintf("Cannot access property on type %s", subj.Type()), s.Property.GetLocation())
			}

			return prop
		}
	case *parse.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}

			if subj.Type() == nil {
				panic(fmt.Errorf("Cannot access %+v on nil: %s", subj.(*Variable).sym, s.Target))
			}
			var sig Type
			if structDef, ok := subj.Type().(*StructDef); ok {
				if method, ok := c.structMethod(structDef, s.Method.Name); ok {
					sig = method
				}
			} else {
				sig = subj.Type().get(s.Method.Name)
			}
			foreignPointerReceiver := false
			if sig == nil {
				if foreign, ok := subj.Type().(*ForeignType); ok {
					if reason := foreign.UnsupportedMethods[s.Method.Name]; reason != "" {
						c.addUnsupportedGoEntity("method", fmt.Sprintf("%s.%s", foreign, s.Method.Name), reason, "Unsupported foreign method", s.Method.GetLocation())
						return nil
					}
					if !foreign.Pointer {
						pointerForeign := *foreign
						pointerForeign.Pointer = true
						pointerForeign.Methods = foreign.PointerMethods
						pointerForeign.UnsupportedMethods = foreign.UnsupportedPointerMethods
						pointerForeign.MethodsLoaded = pointerForeign.Methods != nil || pointerForeign.UnsupportedMethods != nil
						if pointerSig := pointerForeign.get(s.Method.Name); pointerSig != nil {
							if !c.isMutable(subj) {
								c.addDiagnostic(immutableReceiverDiagnostic{
									Kind:            immutablePointerMethodCall,
									Receiver:        foreign.String(),
									Method:          s.Method.Name,
									Span:            c.sourceSpan(s.Method.GetLocation()),
									DeclarationSpan: expressionBindingSpan(subj),
								}.build())
								return nil
							}
							sig = pointerSig
							foreignPointerReceiver = true
						} else if reason := pointerForeign.UnsupportedMethods[s.Method.Name]; reason != "" {
							c.addUnsupportedGoEntity("method", fmt.Sprintf("%s.%s", foreign, s.Method.Name), reason, "Unsupported foreign method", s.Method.GetLocation())
							return nil
						}
					}
				}
			}
			if sig == nil {
				if call, ok := c.checkFunctionFieldCall(subj, s.Method, s.GetLocation()); ok {
					return call
				}
				// A foreign named scalar with no Go method of this name falls back
				// to its underlying primitive's methods (e.g. EventTitle.to_str()).
				// Real Go methods on the named type still win above.
				if prim := foreignScalarPrimitive(subj.Type()); prim != nil {
					if primSig := prim.get(s.Method.Name); primSig != nil {
						subj = &ForeignScalarConvert{Value: subj, Target: prim}
						sig = primSig
					}
				}
				if sig == nil {
					c.addDiagnostic(undefinedMemberDiagnostic{
						Kind:     undefinedMethod,
						Receiver: fmt.Sprint(subj),
						Member:   s.Method.Name,
						Span:     c.sourceSpan(s.Method.GetLocation()),
					}.build())
					return nil
				}
			}

			fnDef, ok := sig.(*FunctionDef)
			if !ok {
				c.addNonCallable(fmt.Sprintf("%s.%s", subj, s.Method.Name), s.Method.GetLocation(), nil, nonCallableSuffix)
				return nil
			}

			if fnDef.Mutates && !c.isMutable(subj) {
				c.addDiagnostic(immutableReceiverDiagnostic{
					Kind:            immutableArdReceiver,
					Receiver:        fmt.Sprint(subj),
					Method:          s.Method.Name,
					Span:            c.sourceSpan(s.Method.GetLocation()),
					DeclarationSpan: expressionBindingSpan(subj),
				}.build())
				return nil
			}

			// Resolve named and positional arguments to match parameters
			resolvedExprs, err := c.resolveArguments(s.Method.Args, fnDef.Parameters)
			if err != nil {
				c.addArgumentBindingError(err, s.GetLocation())
				return nil
			}

			// Check argument count and validate omitted arguments
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					if !parameterOmittable(fnDef.Parameters[i]) {
						c.addMissingArgument(fnDef.Parameters[i], s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) && !(len(fnDef.Parameters) > 0 && fnDef.Parameters[len(fnDef.Parameters)-1].Variadic) {
				c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(resolvedExprs), s.GetLocation(), "")
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// For methods on generic struct instances, bind struct generics to method parameters.
			// When calling a method on a generic struct instance, the method's generic parameters
			// should use the types that were resolved during struct instantiation.
			// Example: For Box{ item: 42 }, calling put(x: $T) should require Int for $T.
			var genericScope *SymbolTable
			var fnDefCopy *FunctionDef

			// Check if the subject is a struct and if the original struct definition has generics
			structType, isStruct := subj.Type().(*StructDef)
			if isStruct {
				// Look up the original named struct definition by module/name to check if it's generic.
				originalDef := c.structDefinition(structType)

				// If the original definition has generics, the current structType might be
				// a specialized version with concrete field types (e.g., item: Int instead of item: $T)
				if originalDef != nil && originalDef.hasGenerics() {
					genericParams := genericParamsForFunction(fnDef)
					genericParams = appendUniqueStrings(genericParams, originalDef.GenericParams...)
					if len(genericParams) > 0 {
						// Create generic scope with fresh TypeVar instances
						genericScope = c.scope.createGenericScope(genericParams)

						// Build mapping from generic names to their resolved types.
						// Compare original struct fields (which contain generics like $T)
						// with current struct fields (which have concrete types like Int).
						// This tells us what each generic parameter should be bound to.
						genericBindings := c.extractGenericBindingsFromSpecializedStruct(originalDef, structType)

						// Bind the method's generic parameters based on the struct instance's specialization
						for paramName, resolvedType := range genericBindings {
							if gv, exists := (*genericScope.genericContext)[paramName]; exists {
								gv.actual = resolvedType
								gv.bound = true
							}
						}

						// Copy the function with the struct's generic bindings applied
						fnDefCopy = copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)
						fnDefCopy.GenericBindings = cloneTypeMap(genericBindings)
					} else {
						fnDefCopy = fnDef
					}
				} else {
					fnDefCopy, genericScope = c.setupFunctionGenerics(fnDef)
				}
			} else {
				// Not a struct - setup generics if the method itself has them
				fnDefCopy, genericScope = c.setupFunctionGenerics(fnDef)
			}

			callTypeArgs := c.resolveCallTypeArgs(s.Method.TypeArgs)
			if len(callTypeArgs) > 0 {
				methodGenericParams := c.explicitMethodGenericParams(fnDef, subj.Type())
				if len(methodGenericParams) == 0 {
					c.addInvalidFunctionTypeArguments(fnDef.Name, 0, len(callTypeArgs), false, s.GetLocation(), "")
					return nil
				}
				if len(callTypeArgs) != len(methodGenericParams) {
					c.addInvalidFunctionTypeArguments(fnDef.Name, len(methodGenericParams), len(callTypeArgs), true, s.GetLocation(), "")
					return nil
				}
				if genericScope == nil {
					genericScope = c.scope.createGenericScope(genericParamsForFunction(fnDef))
					fnDefCopy = copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)
				}
				for i, actual := range callTypeArgs {
					if err := genericScope.bindGeneric(methodGenericParams[i], actual); err != nil {
						c.addError(err.Error(), s.GetLocation())
						return nil
					}
				}
			}

			fnDef = expandFunctionDefForRepeatedVariadic(fnDef, len(resolvedExprs))
			fnDefCopy = expandFunctionDefForRepeatedVariadic(fnDefCopy, len(resolvedExprs))

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if foreign, ok := subj.Type().(*ForeignType); ok {
				if foreign.MapKey != nil && foreign.MapValue != nil && isMapMethodName(s.Method.Name) {
					return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse, callTypeArgs, s.Method.GetLocation())
				}
				if foreign.Elem != nil && isListMethodName(s.Method.Name) {
					return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse, callTypeArgs, s.Method.GetLocation())
				}
				if _, ok := foreign.Underlying.(*FixedArray); ok && isListMethodName(s.Method.Name) {
					return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse, callTypeArgs, s.Method.GetLocation())
				}
				for _, arg := range s.Method.Args {
					if arg.Name != "" {
						c.addNamedArgumentsUnsupported("Foreign method", arg.GetLocation())
						return nil
					}
				}
				pointer := foreign.Pointer || foreignPointerReceiver
				return &ForeignMethodCall{Subject: subj, Target: foreign.Target, Namespace: foreign.Namespace, Qualifier: foreign.Qualifier, Receiver: foreign.Name, Pointer: pointer, Symbol: s.Method.Name, Call: &FunctionCall{Name: s.Method.Name, Args: args, fn: fnToUse, ReturnType: fnToUse.ReturnType}}
			}
			// Create function call
			return c.createPrimitiveMethodNode(subj, s.Method.Name, args, fnToUse, callTypeArgs, s.Method.GetLocation())
		}
	case *parse.MutRef:
		return c.checkMutRef(s)
	case *parse.UnaryExpression:
		{
			value := c.checkExpr(s.Operand)
			if value == nil {
				return nil
			}
			if s.Operator == parse.Minus {
				if !isSignedArithmeticLike(value.Type()) {
					c.addDiagnostic(invalidUnaryOperatorDiagnostic{Operator: "-", Operand: value.Type(), Span: c.sourceSpan(s.Operand.GetLocation()), LegacyMessage: "Only signed numbers can be negated with '-'"}.build())
					return nil
				}
				return &Negation{value}
			}

			if value.Type() != Bool {
				c.addDiagnostic(invalidUnaryOperatorDiagnostic{Operator: "not", Operand: value.Type(), Span: c.sourceSpan(s.Operand.GetLocation()), LegacyMessage: "Only booleans can be negated with 'not'"}.build())
				return nil
			}
			return &Not{value}
		}
	case *parse.BinaryExpression:
		{
			switch s.Operator {
			case parse.Plus:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addInvalidArithmetic("+", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot add different types", false)
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntAddition{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatAddition{left, right}
					}
					if left.Type() == Str {
						return &StrAddition{left, right}
					}
					c.addInvalidArithmetic("+", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "The '-' operator can only be used for Int or Float64", true)
					return nil
				}
			case parse.Minus:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addInvalidArithmetic("-", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot subtract different types", false)
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntSubtraction{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatSubtraction{left, right}
					}
					c.addInvalidArithmetic("-", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "The '+' operator can only be used for Int or Float64", true)
					return nil
				}
			case parse.Multiply:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addInvalidArithmetic("*", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot multiply different types", false)
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntMultiplication{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatMultiplication{left, right}
					}
					c.addInvalidArithmetic("*", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "The '*' operator can only be used for Int or Float64", true)
					return nil
				}
			case parse.Divide:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addInvalidArithmetic("/", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot divide different types", false)
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntDivision{left, right}
					}
					if isArithmeticFloatLike(left.Type()) {
						return &FloatDivision{left, right}
					}
					c.addInvalidArithmetic("/", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "The '/' operator can only be used for Int or Float64", true)
					return nil
				}
			case parse.Modulo:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if !left.Type().equal(right.Type()) {
						c.addInvalidArithmetic("%", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot modulo different types", false)
						return nil
					}
					if isArithmeticIntegerLike(left.Type()) {
						return &IntModulo{left, right}
					}
					c.addInvalidArithmetic("%", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "The '%' operator can only be used for integer scalars", true)
					return nil
				}
			case parse.GreaterThan:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntGreater{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatGreater{left, right}
						}
					}
					c.addInvalidRelational(">", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot compare different types")
					return nil
				}
			case parse.GreaterThanOrEqual:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntGreaterEqual{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatGreaterEqual{left, right}
						}
					}
					c.addInvalidRelational(">=", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot compare different types")
					return nil
				}
			case parse.LessThan:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntLess{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatLess{left, right}
						}
					}
					c.addInvalidRelational("<", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot compare different types")
					return nil
				}
			case parse.LessThanOrEqual:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					// Allow Enum vs Int comparisons
					if c.areTypesComparable(left.Type(), right.Type()) {
						if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
							return &IntLessEqual{left, right}
						}
						if isRelationalFloatLike(left.Type()) {
							return &FloatLessEqual{left, right}
						}
					}
					c.addInvalidRelational("<=", left, right, s.Left.GetLocation(), s.Right.GetLocation(), "Cannot compare different types")
					return nil
				}
			case parse.Equal, parse.NotEqual:
				{
					operator := "=="
					if s.Operator == parse.NotEqual {
						operator = "!="
					}

					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					leftMaybe, leftIsMaybe := left.Type().(*Maybe)
					rightMaybe, rightIsMaybe := right.Type().(*Maybe)
					if leftIsMaybe && rightIsMaybe {
						leftInner := leftMaybe.Of()
						rightInner := rightMaybe.Of()
						maybeEqualityCompatible := func(expected Type, actual Type) bool {
							// Equality lowering has no coercion point, so the foreign
							// scalar coercions do not apply here.
							return c.areCompatible(expected, actual) && !foreignScalarNarrows(expected, actual) && !foreignScalarWidens(expected, actual)
						}
						if leftInner != Void && rightInner != Void && !maybeEqualityCompatible(leftInner, rightInner) && !maybeEqualityCompatible(rightInner, leftInner) {
							legacy := fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type())
							c.addInvalidEquality(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), legacy, false)
							return nil
						}
						// Equality is only supported on nullable primitives. The
						// inner type (the non-Void side when one is `none`) must be a
						// comparable value type.
						inner := leftInner
						if inner == Void {
							inner = rightInner
						}
						if inner != Void && !isComparableValueType(inner) {
							legacy := fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type())
							c.addInvalidEquality(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), legacy, true)
							return nil
						}
						if s.Operator == parse.NotEqual {
							return &Inequality{left, right}
						}
						return &Equality{left, right}
					}

					// Allow Enum vs Int and Int vs Enum comparisons
					if !c.areTypesComparable(left.Type(), right.Type()) {
						legacy := fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type())
						c.addInvalidEquality(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), legacy, false)
						return nil
					}

					if !isComparableValueType(left.Type()) || !isComparableValueType(right.Type()) {
						legacy := fmt.Sprintf("Invalid: %s %s %s", left.Type(), operator, right.Type())
						c.addInvalidEquality(operator, left, right, s.Left.GetLocation(), s.Right.GetLocation(), legacy, true)
						return nil
					}
					if s.Operator == parse.NotEqual {
						return &Inequality{left, right}
					}
					return &Equality{left, right}
				}
			case parse.And:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addDiagnostic(invalidBooleanOperationDiagnostic{Operator: "and", LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(s.Left.GetLocation()), RightSpan: c.sourceSpan(s.Right.GetLocation()), LegacyMessage: "The 'and' operator can only be used between Bools"}.build())
						return nil
					}

					return &And{left, right}
				}
			case parse.Or:
				{
					left, right := c.checkScalarOperands(s.Left, s.Right)
					if left == nil || right == nil {
						return nil
					}

					if left.Type() != Bool || right.Type() != Bool {
						c.addDiagnostic(invalidBooleanOperationDiagnostic{Operator: "or", LeftType: left.Type(), RightType: right.Type(), LeftSpan: c.sourceSpan(s.Left.GetLocation()), RightSpan: c.sourceSpan(s.Right.GetLocation()), LegacyMessage: "The 'or' operator can only be used with Boolean values"}.build())
						return nil
					}

					return &Or{left, right}
				}
			default:
				panic(fmt.Errorf("Unexpected operator: %v", s.Operator))
			}
		}

	case *parse.ChainedComparison:
		{
			// Validate that only relative operators are used (not == or !=)
			for _, op := range s.Operators {
				if op == parse.Equal || op == parse.NotEqual {
					c.addDiagnostic(invalidChainedComparisonDiagnostic{Span: c.sourceSpan(s.GetLocation())}.build())
					return nil
				}
			}

			// Transform chained comparison into nested And expressions
			// a op1 b op2 c => (a op1 b) && (b op2 c)
			var result Expression

			// Build the first comparison: operands[0] op operators[0] operands[1]
			firstComparison := c.buildComparison(s.Operands[0], s.Operators[0], s.Operands[1])
			if firstComparison == nil {
				return nil
			}
			result = firstComparison

			// Build remaining comparisons and AND them together
			for i := 1; i < len(s.Operators); i++ {
				nextComparison := c.buildComparison(s.Operands[i], s.Operators[i], s.Operands[i+1])
				if nextComparison == nil {
					return nil
				}

				// AND the previous result with the next comparison
				result = &And{result, nextComparison}
			}

			return result
		}

	// [refactor] a lot of the function call checking can be extracted
	// - validate args and resolve generics
	case *parse.StaticFunction:
		{
			// Built-in statics on primitive types, e.g. Str::from. These
			// take precedence over module/Go-package lookup since `Str` is not a
			// user-defined symbol. (#283)
			if targetIdent, ok := s.Target.(*parse.Identifier); ok && targetIdent.Name == "Str" {
				if expr, handled := c.checkStrStatic(s); handled {
					return expr
				}
			}

			// `Int64::from(x)`, `Uint32::from(x)`, ... truncating conversion into a
			// bare sized scalar. (#284)
			if targetIdent, ok := s.Target.(*parse.Identifier); ok && s.Function.Name == "from" {
				if scalar := scalarTypeByName(targetIdent.Name); scalar != nil {
					return c.checkScalarFrom(s, scalar)
				}
			}

			// Handle local functions
			absolutePath := s.Target.String() + "::" + s.Function.Name
			if sym, ok := c.scope.get(absolutePath); ok {
				if c.spans != nil {
					if targetIdent, isIdent := s.Target.(*parse.Identifier); isIdent {
						if typeSym, found := c.scope.get(targetIdent.Name); found && isNominalType(typeSym.Type) {
							c.recordTypeRef(targetIdent.GetLocation(), targetIdent.Name)
						}
					}
				}
				fnDef := sym.Type.(*FunctionDef)
				callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)

				// Resolve named and positional arguments to match parameters
				resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
				if err != nil {
					c.addArgumentBindingError(err, s.GetLocation())
					return nil
				}

				numOmittedArgs := 0
				if len(resolvedExprs) < len(fnDef.Parameters) {
					for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
						if !parameterOmittable(fnDef.Parameters[i]) {
							c.addMissingArgument(fnDef.Parameters[i], s.GetLocation())
							return nil
						}
					}
					numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
				} else if len(resolvedExprs) > len(fnDef.Parameters) {
					c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(resolvedExprs), s.GetLocation(), "")
					resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
				}

				fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)
				args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
				if args == nil {
					return nil
				}
				if len(callTypeArgs) > 0 {
					specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
					if err != nil {
						c.addGenericFunctionResolutionError(err, s.GetLocation())
						return nil
					}
					fnToUse = specialized
				}

				return &FunctionCall{
					Name:       absolutePath,
					Args:       args,
					TypeArgs:   callTypeArgs,
					fn:         fnToUse,
					ReturnType: fnToUse.ReturnType,
				}
			}

			// find the function in a module or Go package namespace
			modName, name := c.destructurePath(s)
			if mod := c.resolveModule(modName); mod != nil && mod.Path() == "ard/unsafe" {
				switch name {
				case "cast":
					return c.checkUnsafeCast(s)
				case "is_nil":
					return c.checkUnsafeIsNil(s)
				}
			}
			if goPkg := c.program.GoImports[modName]; goPkg != nil {
				// `pkg::T::from(x)` truncating conversion into a foreign named
				// scalar type, e.g. time::Duration::from(ms). (#284)
				if typeName, isFrom := strings.CutSuffix(name, "::from"); isFrom {
					if named, ok := goPkg.Types[typeName]; ok && isNumericScalar(foreignScalarPrimitive(named)) {
						return c.checkScalarFrom(s, named)
					}
				}
				fnDef := goPkg.Functions[name]
				var callTypeArgs []Type
				pointerResult := false
				if goFn := goPkg.Generics[name]; goFn != nil {
					fnDef, callTypeArgs, pointerResult = c.instantiateGoFunctionCall(modName, name, goFn, s)
					if fnDef == nil {
						return nil
					}
				} else if fnDef != nil && len(s.Function.TypeArgs) > 0 {
					qualified := modName + "::" + name
					legacy := fmt.Sprintf("Go function %s is not generic", qualified)
					c.addDiagnostic(invalidGoFunctionTypeArgumentsDiagnostic{
						Name: qualified, Actual: len(s.Function.TypeArgs), Span: c.sourceSpan(declaredTypeLocation(s.Function.TypeArgs[0], s.GetLocation())), LegacyMessage: legacy, NonGeneric: true,
					}.build())
					return nil
				}
				if fnDef == nil {
					// `pkg::T(x)` where T is a foreign named scalar type is an
					// explicit conversion to the named type, e.g. ui::IntentType(s).
					// Once T is known to be such a type, commit to the conversion so
					// a mismatched argument reports a clear error.
					if named, ok := goPkg.Types[name]; ok && len(s.Function.TypeArgs) == 0 {
						if prim := foreignScalarPrimitive(named); prim != nil {
							if len(s.Function.Args) != 1 {
								c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
								return nil
							}
							arg := c.checkExprAs(s.Function.Args[0].Value, prim)
							if arg == nil {
								return nil
							}
							// Identity conversion is a no-op, matching Go.
							if named.equal(arg.Type()) {
								return arg
							}
							if prim.equal(arg.Type()) {
								return &ForeignScalarConvert{Value: arg, Target: named}
							}
							// Another foreign scalar with the same underlying converts
							// through the primitive, lowering to T(string(x)).
							if foreignScalarNarrows(prim, arg.Type()) {
								return &ForeignScalarConvert{Value: &ForeignScalarConvert{Value: arg, Target: prim}, Target: named}
							}
							c.addTypeMismatch(prim, arg.Type(), s.Function.Args[0].GetLocation())
							return nil
						}
					}
					if reason, ok := goPkg.UnsupportedFunctions[name]; ok {
						qualified := modName + "::" + name
						legacy := fmt.Sprintf("Unsupported Go function %s: %s", qualified, reason)
						c.addDiagnostic(unsupportedGoEntityDiagnostic{Kind: "function", Name: qualified, Reason: reason, Span: c.sourceSpan(s.GetLocation()), LegacyMessage: legacy}.build())
					} else {
						c.addUnresolvedReference(undefinedGoFunction, fmt.Sprintf("%s::%s", modName, name), s.GetLocation())
					}
					return nil
				}
				for _, arg := range s.Function.Args {
					if arg.Name != "" {
						c.addNamedArgumentsUnsupported("Go function", arg.GetLocation())
						return nil
					}
				}
				resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
				if err != nil {
					c.addArgumentBindingError(err, s.GetLocation())
					return nil
				}
				effectiveFnDef := expandFunctionDefForRepeatedVariadic(fnDef, len(resolvedExprs))
				if len(resolvedExprs) != len(effectiveFnDef.Parameters) {
					// A trailing Go variadic argument may be omitted.
					omittedVariadic := len(resolvedExprs) == len(fnDef.Parameters)-1 && fnDef.Parameters[len(fnDef.Parameters)-1].Variadic
					if !omittedVariadic {
						c.addArgumentCount(fmt.Sprint(variadicExpectedArgumentCount(fnDef)), len(resolvedExprs), s.GetLocation(), "")
						return nil
					}
				}
				args := make([]Expression, len(resolvedExprs))
				for i, expr := range resolvedExprs {
					checkedArg := c.checkExprAsArgument(expr, effectiveFnDef.Parameters[i].Type, effectiveFnDef.Parameters[i])
					if checkedArg == nil {
						return nil
					}
					if !c.areCompatible(effectiveFnDef.Parameters[i].Type, checkedArg.Type()) {
						upcast, ok := c.foreignInterfaceArgUpcast(effectiveFnDef.Parameters[i].Type, checkedArg)
						if !ok {
							legacyMessage := typeMismatch(effectiveFnDef.Parameters[i].Type, checkedArg.Type())
							c.addIncorrectArgumentType(legacyMessage, effectiveFnDef.Parameters[i].Type, checkedArg.Type(), expr.GetLocation(), effectiveFnDef.Parameters[i], false)
							return nil
						}
						checkedArg = upcast
					}
					if effectiveFnDef.Parameters[i].Mutable && !c.isMutable(checkedArg) && !freshContainerSatisfiesMutable(effectiveFnDef.Parameters[i].Type, checkedArg) {
						legacyMessage := fmt.Sprintf("Type mismatch: Expected a mutable %s", effectiveFnDef.Parameters[i].Type.String())
						c.addIncorrectArgumentType(legacyMessage, effectiveFnDef.Parameters[i].Type, checkedArg.Type(), expr.GetLocation(), effectiveFnDef.Parameters[i], true)
						return nil
					}
					args[i] = checkedArg
				}
				return &ForeignFunctionCall{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: name, TypeArgs: callTypeArgs, PointerResult: pointerResult, Call: &FunctionCall{Name: name, Args: args, fn: effectiveFnDef, ReturnType: effectiveFnDef.ReturnType}}
			}

			var fnDef *FunctionDef
			mod := c.resolveModule(modName)
			if mod == nil {
				c.addUnresolvedReference(undefinedModule, modName, s.Target.GetLocation())
				return nil
			}
			if mod.Path() == "builtin/Maybe" && name == "new" {
				return c.checkMaybeNewStatic(s, mod)
			}

			sym := mod.Get(name)
			if sym.IsZero() {
				targetName := s.Target.String()
				c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", targetName, s.Function.Name), s.GetLocation())
				return nil
			}

			// Handle both regular functions and external functions
			var ok bool
			switch fn := sym.Type.(type) {
			case *FunctionDef:
				fnDef = fn
				ok = true
			default:
				ok = false
			}

			if !ok {
				targetName := s.Target.String()
				c.addNonCallable(fmt.Sprintf("%s::%s", targetName, s.Function.Name), s.GetLocation(), nil, nonCallableSuffix)
				return nil
			}
			callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)

			// Resolve named and positional arguments to match parameters
			resolvedExprs, err := c.resolveArguments(s.Function.Args, fnDef.Parameters)
			if err != nil {
				c.addArgumentBindingError(err, s.GetLocation())
				return nil
			}

			// Check argument count and validate omitted arguments
			numOmittedArgs := 0
			if len(resolvedExprs) < len(fnDef.Parameters) {
				// Find first non-nullable parameter that's missing
				for i := len(resolvedExprs); i < len(fnDef.Parameters); i++ {
					if !parameterOmittable(fnDef.Parameters[i]) {
						c.addMissingArgument(fnDef.Parameters[i], s.GetLocation())
						return nil
					}
				}
				numOmittedArgs = len(fnDef.Parameters) - len(resolvedExprs)
			} else if len(resolvedExprs) > len(fnDef.Parameters) {
				c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(resolvedExprs), s.GetLocation(), "")
				resolvedExprs = resolvedExprs[:len(fnDef.Parameters)]
			}

			// Setup generics if function has them
			fnDefCopy, genericScope := c.setupFunctionGenerics(fnDef)

			// Check and process arguments (handles both generics and mutability)
			args, fnToUse := c.checkAndProcessArguments(fnDef, resolvedExprs, fnDefCopy, genericScope, numOmittedArgs)
			if args == nil {
				return nil
			}
			if len(callTypeArgs) > 0 {
				specialized, err := c.resolveGenericFunction(fnDef, args, callTypeArgs, s.GetLocation())
				if err != nil {
					c.addGenericFunctionResolutionError(err, s.GetLocation())
					return nil
				}
				fnToUse = specialized
			}

			// Create function call. Keep the source member name so lowering can
			// distinguish real module functions from function-typed module variables.
			callName := name
			call := &FunctionCall{
				Name:       callName,
				Args:       args,
				TypeArgs:   callTypeArgs,
				fn:         fnToUse,
				ReturnType: fnToUse.ReturnType,
			}
			c.recordTarget(s, call, SpanTarget{Kind: TargetFunction, Module: mod.Path(), Symbol: name})
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call:   call,
			}
		}
	case *parse.IfStatement:
		return c.checkIfChain(s)
	case *parse.FunctionDeclaration:
		return c.checkFunction(s, nil)
	case *parse.AnonymousFunction:
		{
			// Resolve parameters and return type (no type context for inference)
			params := c.resolveParametersWithContext(s.Parameters, nil)
			returnType := c.resolveReturnTypeWithContext(s.ReturnType, nil)

			// Create function definition
			uniqueName := fmt.Sprintf("anon_func_%p", s)
			fn := &FunctionDef{
				Name:                    uniqueName,
				Parameters:              params,
				ReturnType:              returnType,
				InferReturnTypeFromBody: s.ReturnType == nil,
				Body:                    nil,
			}

			// Check body
			setup := func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.recordBinding(param.Loc, c.scope.add(param.Name, param.Type, param.Mutable))
				}
			}
			c.pushFunctionGenericContext(fn)
			previousDeferredWorkDepth := c.deferredWorkDepth
			c.deferredWorkDepth = 0
			var body *Block
			if fn.InferReturnTypeFromBody {
				// Without a return annotation, the closure adopts its body's
				// final expression type (issue #266). The final expression is
				// checked as a value so constructs like match keep their arm
				// consistency validation. Expected-type contexts (combinator
				// generics, Go callbacks, annotated fn types) flow through
				// checkExprAs instead of this path, so the expected type still
				// governs there — a value-producing body keeps discarding for
				// void callbacks.
				body = c.checkBlockWithInferredFinalValue(s.Body, setup, false)
				fn.ReturnType = body.Type()
			} else {
				body = c.checkBlockWithExpected(s.Body, setup, returnType, true)
			}
			c.deferredWorkDepth = previousDeferredWorkDepth
			c.popFunctionGenericContext()

			// Add function to scope after body is checked (for generic resolution support)
			c.scope.add(uniqueName, fn, false)

			// Validate return type
			if !fn.InferReturnTypeFromBody && returnType != Void && !c.areCompatible(returnType, body.Type()) {
				c.addBodyReturnMismatch(s.Body, returnType, body.Type(), s.GetLocation(), s.ReturnType)
				return nil
			}

			fn.Body = body
			return fn
		}
	case *parse.StaticFunctionDeclaration:
		if c.spans != nil {
			if target, ok := s.Path.Target.(*parse.Identifier); ok {
				if sym, found := c.scope.get(target.Name); found && isNominalType(sym.Type) {
					c.recordTypeRef(target.GetLocation(), target.Name)
				}
			}
		}
		fn := c.checkFunction(&s.FunctionDeclaration, nil)
		if fn != nil {
			fn.Name = s.Path.String()
			c.scope.add(fn.Name, fn, false)
		}

		return fn
	case *parse.ListLiteral:
		// checkList returns a typed-nil *ListLiteral on failure; normalize to an
		// interface nil so callers' `== nil` checks hold and never deref it.
		if result := c.checkList(nil, s); result != nil {
			return result
		}
		return nil
	case *parse.MapLiteral:
		if result := c.checkMap(nil, s); result != nil {
			return result
		}
		return nil
	case *parse.SelectExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()

		sel := &Select{}
		var resultType Type
		var defaultSpan *SourceSpan

		for _, arm := range s.Cases {
			// Default arm: the head is the `_` identifier.
			if id, ok := arm.Op.(*parse.Identifier); ok && id.Name == "_" {
				if arm.Binding != nil {
					c.addInvalidSelectArm("The default select arm cannot bind a value", arm.Binding.GetLocation(), "the default arm cannot bind a value")
				}
				if defaultSpan != nil {
					c.addDuplicateMatchArm(Error, "Duplicate default (_) arm in select", arm.Op.GetLocation(), defaultSpan)
				} else {
					span := c.sourceSpan(arm.Op.GetLocation())
					defaultSpan = &span
				}
				body := c.checkMatchArmBlock(arm.Body, nil)
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmDefault, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), arm.Op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
				continue
			}

			op, ok := arm.Op.(*parse.InstanceMethod)
			if !ok {
				c.addInvalidSelectArm("A select arm must be a channel recv() or send() operation", arm.Op.GetLocation(), "expected a channel `recv()` or `send()` operation")
				continue
			}

			channel := c.checkExpr(op.Target)
			if channel == nil {
				continue
			}
			elem, ok := channelElementType(channel.Type())
			if !ok {
				legacy := fmt.Sprintf("A select arm operates on a channel, but got %s", channel.Type().String())
				c.addInvalidSelectArm(legacy, op.Target.GetLocation(), fmt.Sprintf("expected a channel, but found `%s`", channel.Type()))
				continue
			}

			switch op.Method.Name {
			case "recv":
				if !channelCanRecv(channel.Type()) {
					legacy := fmt.Sprintf("recv() is not available on %s", channel.Type().String())
					c.addInvalidSelectArm(legacy, op.GetLocation(), "this channel does not support receiving")
					continue
				}
				if len(op.Method.Args) != 0 {
					c.addInvalidSelectArm("recv() in a select arm takes no arguments", op.GetLocation(), "remove the arguments from this `recv()` operation")
				}
				var binding *Identifier
				if arm.Binding != nil {
					binding = &Identifier{Name: arm.Binding.Name}
				}
				body := c.checkMatchArmBlock(arm.Body, func() {
					if arm.Binding != nil {
						c.scope.add(arm.Binding.Name, &Maybe{elem}, false)
					}
				})
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmRecv, Channel: channel, Binding: binding, ElemType: elem, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
			case "send":
				if !channelCanSend(channel.Type()) {
					legacy := fmt.Sprintf("send() is not available on %s", channel.Type().String())
					c.addInvalidSelectArm(legacy, op.GetLocation(), "this channel does not support sending")
					continue
				}
				if arm.Binding != nil {
					c.addInvalidSelectArm("A select send arm cannot bind a value", arm.Binding.GetLocation(), "a send arm cannot bind a received value")
				}
				if len(op.Method.Args) != 1 {
					c.addInvalidSelectArm("send() in a select arm takes exactly one argument", op.GetLocation(), "provide exactly one value to send")
					continue
				}
				value := c.checkExprAs(op.Method.Args[0].Value, elem)
				body := c.checkMatchArmBlock(arm.Body, nil)
				sel.Arms = append(sel.Arms, SelectArm{Kind: SelectArmSend, Channel: channel, ElemType: elem, Value: value, Body: body})
				if merged, ok := mergeMatchResultType(c, resultType, body.Type(), op.GetLocation(), allowMixedVoid); ok {
					resultType = merged
				}
			default:
				legacy := fmt.Sprintf("A select arm must use recv() or send(), got %s()", op.Method.Name)
				c.addInvalidSelectArm(legacy, op.GetLocation(), "expected `recv()` or `send()` here")
			}
		}

		sel.ResultType = resultType
		return sel

	case *parse.MatchExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()

		// Check the subject
		subject := c.checkExpr(s.Subject)
		if subject == nil {
			return nil
		}

		// Dynamic type tests over Any or foreign-interface subjects (ADR 0042)
		if isDynamicMatchSubject(subject.Type()) {
			return c.checkForeignTypeMatch(s, subject, allowMixedVoid)
		}

		// For Maybe types, generate an OptionMatch
		if maybeType, ok := subject.Type().(*Maybe); ok {
			var patternIdent *Identifier
			var someBody *Block
			var noneBody *Block
			bindingMutable := c.isMutable(subject)

			// Process the cases
			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// This is the None case
						noneBody = c.checkMatchArmBlock(matchCase.Body, nil)
					} else {
						// This is the Some case with a variable binding
						// Create a new scope for the body with the pattern bound to the unwrapped value
						someBody = c.checkMatchArmBlock(matchCase.Body, func() {
							// Add the pattern name as a variable in the scope with the inner type
							// For example, if the Maybe is Str?, the pattern should be a Str
							c.scope.add(id.Name, maybeType.of, bindingMutable)
						})

						// Create an identifier to use in the Match struct
						patternIdent = &Identifier{Name: id.Name}
					}
				} else {
					c.addInvalidMatchPattern("Pattern in Maybe match must be an identifier", matchCase.Pattern.GetLocation(), "expected a binding identifier or `_`")
					return nil
				}
			}

			// Ensure we have both some and none cases
			if someBody == nil {
				c.addNonExhaustiveMatch("Match on a Maybe type must include a binding case", s.GetLocation(), "add a binding case for the present value")
				return nil
			}

			if noneBody == nil {
				c.addNonExhaustiveMatch("Match on a Maybe type must include a wildcard (_) case", s.GetLocation(), "add a wildcard `_` case for the absent value")
				return nil
			}

			resultType, ok := mergeMatchResultType(c, someBody.Type(), noneBody.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}

			// Create the OptionMatch
			return &OptionMatch{
				Subject:   subject,
				InnerType: maybeType.of,
				Some: &Match{
					Pattern: patternIdent,
					Body:    someBody,
				},
				None:       noneBody,
				ResultType: resultType,
			}
		}

		// For Enum types, generate an EnumMatch
		if enumType, ok := subject.Type().(*Enum); ok {
			// Map to track which discriminant values we've seen. Imported Go enum-like
			// constants may have multiple exported aliases for the same value.
			seenDiscriminants := make(map[int]struct {
				Name string
				Span SourceSpan
			})
			// Track whether we've seen a catch-all case
			var catchAllSpan *SourceSpan
			// Cases in the match statement mapped to enum variants
			cases := make([]*Block, len(enumType.Values))
			var catchAllBody *Block

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// This is a catch-all case
						if catchAllSpan != nil {
							c.addDuplicateMatchArm(Error, "Duplicate catch-all case", matchCase.Pattern.GetLocation(), catchAllSpan)
							return nil
						}

						span := c.sourceSpan(matchCase.Pattern.GetLocation())
						catchAllSpan = &span
						catchAllBody = c.checkMatchArmBlock(matchCase.Body, nil)
						continue
					}
				}

				// Handle enum variant case - the pattern should be a static property reference like Enum::Variant
				if staticProp, ok := matchCase.Pattern.(*parse.StaticProperty); ok {
					// Resolve the pattern using existing expression resolution logic
					patternExpr := c.checkExpr(staticProp)
					if patternExpr == nil {
						continue // Error already reported by checkExpr
					}

					// Check if the pattern resolves to an enum variant
					enumVariant, ok := patternExpr.(*EnumVariant)
					if !ok {
						c.addInvalidMatchPattern("Pattern in enum match must be an enum variant", staticProp.GetLocation(), "this does not resolve to an enum variant")
						continue
					}

					// Verify that the variant's enum matches the subject's enum
					if !enumVariant.enum.equal(enumType) {
						legacy := fmt.Sprintf("Cannot match %s variant against %s enum", enumVariant.enum.Name, enumType.Name)
						c.addInvalidMatchPattern(legacy, staticProp.GetLocation(), fmt.Sprintf("this variant belongs to `%s`, not `%s`", enumVariant.enum.Name, enumType.Name))
						continue
					}

					// Get the variant name and index
					variantName := enumType.Values[enumVariant.Variant].Name
					variantIndex := int(enumVariant.Variant)

					// Check for duplicate cases by value, not just by name. This lets Go
					// enum-like constants import aliases while preserving closed enum
					// exhaustiveness over distinct values.
					discriminant := enumType.Values[enumVariant.Variant].Value
					current := fmt.Sprintf("%s::%s", enumType.Name, variantName)
					if previous, found := seenDiscriminants[discriminant]; found {
						legacy := fmt.Sprintf("Duplicate case: %s", current)
						if previous.Name != current {
							legacy = fmt.Sprintf("Duplicate case: %s has same value as %s", current, previous.Name)
						}
						c.addDuplicateMatchArm(Error, legacy, staticProp.GetLocation(), &previous.Span)
						continue
					}
					seenDiscriminants[discriminant] = struct {
						Name string
						Span SourceSpan
					}{Name: current, Span: c.sourceSpan(staticProp.GetLocation())}

					// Check the body for this case
					body := c.checkMatchArmBlock(matchCase.Body, nil)
					cases[variantIndex] = body
				} else {
					c.addInvalidMatchPattern("Pattern in enum match must be an enum variant or wildcard", matchCase.Pattern.GetLocation(), "expected an enum variant or `_`")
					return nil
				}
			}

			// Check if the match is exhaustive over distinct values. Aliases do not
			// require separate arms. Imported open Go enum-like types always require
			// a wildcard because Go may produce values outside exported constants.
			if catchAllSpan == nil {
				if enumType.Open {
					legacy := fmt.Sprintf("Open enum-like Go type %s requires a catch-all (_) match case", enumType.Name)
					c.addNonExhaustiveMatch(legacy, s.GetLocation(), "add a catch-all `_` case for this open enum-like type")
				} else {
					missingValues := map[int]bool{}
					for i, value := range enumType.Values {
						if _, covered := seenDiscriminants[value.Value]; covered {
							continue
						}
						if cases[i] == nil && !missingValues[value.Value] {
							legacy := fmt.Sprintf("Incomplete match: missing case for '%s::%s'", enumType.Name, value.Name)
							c.addNonExhaustiveMatch(legacy, s.GetLocation(), fmt.Sprintf("add a case for `%s::%s`", enumType.Name, value.Name))
							missingValues[value.Value] = true
						}
					}
				}
			}

			// Ensure all cases return compatible types
			var enumResultType Type
			for _, caseBody := range cases {
				if caseBody == nil {
					continue
				}
				var ok bool
				enumResultType, ok = mergeMatchResultType(c, enumResultType, caseBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}
			if catchAllBody != nil {
				var ok bool
				enumResultType, ok = mergeMatchResultType(c, enumResultType, catchAllBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}

			// Create the EnumMatch
			discriminantToIndex := make(map[int]int, len(enumType.Values))
			for i, value := range enumType.Values {
				if _, ok := discriminantToIndex[value.Value]; !ok {
					discriminantToIndex[value.Value] = i
				}
			}
			enumMatch := &EnumMatch{
				Subject:             subject,
				Cases:               cases,
				CatchAll:            catchAllBody,
				DiscriminantToIndex: discriminantToIndex,
				ResultType:          enumResultType,
			}

			return enumMatch
		}

		// For Bool types, generate a BoolMatch
		if subject.Type() == Bool {
			var trueBody, falseBody *Block
			// Track which cases we've seen
			var trueSpan, falseSpan *SourceSpan

			// Process the cases
			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok {
					if id.Name == "_" {
						// Catch-all cases aren't allowed for boolean matches
						c.addInvalidMatchPattern("Catch-all case is not allowed for boolean matches", matchCase.Pattern.GetLocation(), "use explicit `true` and `false` cases")
						return nil
					}
				}

				// Handle boolean literal case
				if boolLit, ok := matchCase.Pattern.(*parse.BoolLiteral); ok {
					// Check for duplicates
					if boolLit.Value && trueSpan != nil {
						c.addDuplicateMatchArm(Error, "Duplicate case: 'true'", matchCase.Pattern.GetLocation(), trueSpan)
						return nil
					}
					if !boolLit.Value && falseSpan != nil {
						c.addDuplicateMatchArm(Error, "Duplicate case: 'false'", matchCase.Pattern.GetLocation(), falseSpan)
						return nil
					}

					// Process the body
					body := c.checkMatchArmBlock(matchCase.Body, nil)

					// Store the body in the appropriate field
					if boolLit.Value {
						span := c.sourceSpan(matchCase.Pattern.GetLocation())
						trueSpan = &span
						trueBody = body
					} else {
						span := c.sourceSpan(matchCase.Pattern.GetLocation())
						falseSpan = &span
						falseBody = body
					}
				} else {
					c.addInvalidMatchPattern("Pattern in boolean match must be a boolean literal (true or false)", matchCase.Pattern.GetLocation(), "expected `true` or `false`")
					return nil
				}
			}

			// Check exhaustiveness
			if trueSpan == nil || falseSpan == nil {
				if trueSpan == nil {
					c.addNonExhaustiveMatch("Incomplete match: Missing case for 'true'", s.GetLocation(), "add a case for `true`")
				} else {
					c.addNonExhaustiveMatch("Incomplete match: Missing case for 'false'", s.GetLocation(), "add a case for `false`")
				}
				return nil
			}

			// Ensure both branches return compatible types. This allows one branch to
			// return a trait object while the other returns a concrete implementation.
			resultType, ok := mergeMatchResultType(c, trueBody.Type(), falseBody.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}

			// Create and return the BoolMatch
			return &BoolMatch{
				Subject:    subject,
				True:       trueBody,
				False:      falseBody,
				ResultType: resultType,
			}
		}

		// For Union types, generate a UnionMatch
		if unionType, ok := subject.Type().(*Union); ok {
			bindingMutable := c.isMutable(subject)
			// Track which union types we've seen and their corresponding bodies
			typeCases := make(map[string]*Match)
			typeCaseSpans := make(map[string]SourceSpan)
			typeCasesByType := make(map[Type]*Match)
			var catchAllBody *Block
			var catchAllSpan *SourceSpan

			// Record all types in the union
			unionTypeSet := make(map[string]Type)
			for _, t := range unionType.Types {
				unionTypeSet[t.String()] = t
			}

			// Process the cases
			for _, matchCase := range s.Cases {
				switch p := matchCase.Pattern.(type) {
				case *parse.Identifier:
					if p.Name == "_" {
						if catchAllBody != nil {
							c.addDuplicateMatchArm(Warn, "Duplicate catch-all case", matchCase.Pattern.GetLocation(), catchAllSpan)
						} else {
							span := c.sourceSpan(matchCase.Pattern.GetLocation())
							catchAllSpan = &span
							catchAllBody = c.checkMatchArmBlock(matchCase.Body, nil)
						}
						break
					}
					// Allow union type name as implicit binding to "it"
					matchedType, found := unionTypeSet[p.Name]
					if !found {
						c.addInvalidMatchPattern("Catch-all case should be matched with '_'", matchCase.Pattern.GetLocation(), "use `_` for a catch-all case")
						break
					}
					if _, exists := typeCases[p.Name]; exists {
						original := typeCaseSpans[p.Name]
						c.addDuplicateMatchArm(Warn, fmt.Sprintf("Duplicate case: %s", p.Name), matchCase.Pattern.GetLocation(), &original)
						break
					}
					body := c.checkMatchArmBlock(matchCase.Body, func() {
						c.scope.add("it", matchedType, bindingMutable)
					})
					matchNode := &Match{
						Pattern: &Identifier{Name: "it"},
						Body:    body,
					}
					typeCases[p.Name] = matchNode
					typeCaseSpans[p.Name] = c.sourceSpan(matchCase.Pattern.GetLocation())
					typeCasesByType[matchedType] = matchNode
				case *parse.FunctionCall:
					varName := p.Args[0].Value.(*parse.Identifier).Name
					typeName := p.Name

					// Check if the type exists in the union
					_, found := unionTypeSet[typeName]
					if !found {
						legacy := fmt.Sprintf("Type %s is not part of union %s", typeName, unionType)
						c.addInvalidMatchPattern(legacy, matchCase.Pattern.GetLocation(), fmt.Sprintf("`%s` is not a member of `%s`", typeName, unionType))
					}

					// Check for duplicates
					if _, exists := typeCases[typeName]; exists {
						original := typeCaseSpans[typeName]
						c.addDuplicateMatchArm(Warn, fmt.Sprintf("Duplicate case: %s", typeName), matchCase.Pattern.GetLocation(), &original)
					} else {

						// Get the actual type object
						matchedType := unionTypeSet[typeName]

						// Process the body with the matched type binding
						body := c.checkMatchArmBlock(matchCase.Body, func() {
							c.scope.add(varName, matchedType, bindingMutable)
						})
						matchNode := &Match{
							Pattern: &Identifier{Name: varName},
							Body:    body,
						}
						typeCases[typeName] = matchNode
						typeCaseSpans[typeName] = c.sourceSpan(matchCase.Pattern.GetLocation())
						typeCasesByType[matchedType] = matchNode
					}
				}
			}

			// Check exhaustiveness if no catch-all is provided
			if catchAllBody == nil {
				for typeName := range unionTypeSet {
					if _, covered := typeCases[typeName]; !covered {
						legacy := fmt.Sprintf("Incomplete match: missing case for '%s'", typeName)
						c.addNonExhaustiveMatch(legacy, s.GetLocation(), fmt.Sprintf("add a case for `%s`", typeName))
					}
				}
			}

			// Ensure all cases return compatible types
			var unionResultType Type
			for _, caseBody := range typeCases {
				if caseBody == nil {
					continue
				}
				var ok bool
				unionResultType, ok = mergeMatchResultType(c, unionResultType, caseBody.Body.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}
			if catchAllBody != nil {
				var ok bool
				unionResultType, ok = mergeMatchResultType(c, unionResultType, catchAllBody.Type(), s.GetLocation(), allowMixedVoid)
				if !ok {
					return nil
				}
			}

			// Create and return the UnionMatch
			return &UnionMatch{
				Subject:         subject,
				TypeCases:       typeCases,
				TypeCasesByType: typeCasesByType,
				CatchAll:        catchAllBody,
				ResultType:      unionResultType,
			}
		}

		if resultType, ok := subject.Type().(*Result); ok {
			bindingMutable := c.isMutable(subject)
			if len(s.Cases) > 2 {
				c.addInvalidMatchPattern("Too many cases in match", s.GetLocation(), "a `Result` match accepts only `ok` and `err` cases")
				return nil
			}

			var okCase *Match
			var errCase *Match
			for _, node := range s.Cases {
				switch p := node.Pattern.(type) {
				case *parse.Identifier:
					{
						switch p.Name {
						case "ok":
							okCase = &Match{
								Pattern: &Identifier{Name: "ok"},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add("ok", resultType.Val(), bindingMutable)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: "err"},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add("err", resultType.Err(), bindingMutable)
								}),
							}
						default:
							c.addDiagnostic(ignoredMatchPatternDiagnostic{Span: c.sourceSpan(p.GetLocation())}.build())
						}
					}
				case *parse.FunctionCall: // use FunctionCall node as aliasing variable
					{
						varName := p.Args[0].Value.(*parse.Identifier).Name
						switch p.Name {
						case "ok":
							varName := p.Args[0].Value.(*parse.Identifier).Name
							okCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add(varName, resultType.Val(), bindingMutable)
								}),
							}
						case "err":
							errCase = &Match{
								Pattern: &Identifier{Name: varName},
								Body: c.checkMatchArmBlock(node.Body, func() {
									c.scope.add(varName, resultType.Err(), bindingMutable)
								}),
							}
						default:
							c.addDiagnostic(ignoredMatchPatternDiagnostic{Span: c.sourceSpan(p.GetLocation())}.build())
						}
					}
				}
			}

			if okCase == nil {
				c.addNonExhaustiveMatch("Missing ok case", s.GetLocation(), "add an `ok` case")
				return nil
			}
			if errCase == nil {
				c.addNonExhaustiveMatch("Missing err case", s.GetLocation(), "add an `err` case")
				return nil
			}

			matchResultType, ok := mergeMatchResultType(c, okCase.Body.Type(), errCase.Body.Type(), s.GetLocation(), allowMixedVoid)
			if !ok {
				return nil
			}
			return &ResultMatch{
				Subject:    subject,
				Ok:         okCase,
				Err:        errCase,
				OkType:     resultType.Val(),
				ErrType:    resultType.Err(),
				ResultType: matchResultType,
			}
		}

		if subject.Type() == Str {
			strCases := make(map[string]*Block)
			strCaseSpans := make(map[string]SourceSpan)
			var catchAll *Block
			var catchAllSpan *SourceSpan
			var strResultType Type

			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					if catchAll != nil {
						c.addDuplicateMatchArm(Error, "Duplicate catch-all case", matchCase.Pattern.GetLocation(), catchAllSpan)
						return nil
					}
					span := c.sourceSpan(matchCase.Pattern.GetLocation())
					catchAllSpan = &span
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					strResultType, ok = mergeMatchResultType(c, strResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
					continue
				}

				literal, ok := matchCase.Pattern.(*parse.StrLiteral)
				if !ok {
					c.addInvalidMatchPattern("Pattern in Str match must be a string literal or '_'", matchCase.Pattern.GetLocation(), "expected a string literal or `_`")
					return nil
				}
				if _, exists := strCases[literal.Value]; exists {
					original := strCaseSpans[literal.Value]
					c.addDuplicateMatchArm(Error, fmt.Sprintf("Duplicate case: %q", literal.Value), matchCase.Pattern.GetLocation(), &original)
					return nil
				}
				caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
				strCases[literal.Value] = caseBlock
				strCaseSpans[literal.Value] = c.sourceSpan(matchCase.Pattern.GetLocation())
				var mergeOK bool
				strResultType, mergeOK = mergeMatchResultType(c, strResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
				if !mergeOK {
					return nil
				}
			}

			if catchAll == nil {
				c.addNonExhaustiveMatch("Incomplete match: missing catch-all case for Str match", s.GetLocation(), "add a catch-all `_` case")
				return nil
			}

			return &StrMatch{Subject: subject, Cases: strCases, CatchAll: catchAll, ResultType: strResultType}
		}

		if subject.Type() == Rune {
			runeCases := make(map[int]*Block)
			runeCaseSpans := make(map[int]SourceSpan)
			var catchAll *Block
			var catchAllSpan *SourceSpan
			var runeResultType Type

			for _, matchCase := range s.Cases {
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					if catchAll != nil {
						c.addDuplicateMatchArm(Error, "Duplicate catch-all case", matchCase.Pattern.GetLocation(), catchAllSpan)
						return nil
					}
					span := c.sourceSpan(matchCase.Pattern.GetLocation())
					catchAllSpan = &span
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					runeResultType, ok = mergeMatchResultType(c, runeResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
					continue
				}

				literal, ok := matchCase.Pattern.(*parse.RuneLiteral)
				if !ok {
					c.addInvalidMatchPattern("Pattern in Rune match must be a rune literal or '_'", matchCase.Pattern.GetLocation(), "expected a rune literal or `_`")
					return nil
				}
				value, valid := c.parseRuneLiteralValue(literal)
				if !valid {
					return nil
				}
				intValue := int(value)
				if _, exists := runeCases[intValue]; exists {
					original := runeCaseSpans[intValue]
					c.addDuplicateMatchArm(Error, fmt.Sprintf("Duplicate case: %s", strconv.QuoteRune(value)), matchCase.Pattern.GetLocation(), &original)
					return nil
				}
				caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
				runeCases[intValue] = caseBlock
				runeCaseSpans[intValue] = c.sourceSpan(matchCase.Pattern.GetLocation())
				var mergeOK bool
				runeResultType, mergeOK = mergeMatchResultType(c, runeResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
				if !mergeOK {
					return nil
				}
			}

			if catchAll == nil {
				c.addNonExhaustiveMatch("Incomplete match: missing catch-all case for Rune match", s.GetLocation(), "add a catch-all `_` case")
			}

			return &IntMatch{
				Subject:    subject,
				IntCases:   runeCases,
				RangeCases: map[IntRange]*Block{},
				CatchAll:   catchAll,
				ResultType: runeResultType,
			}
		}

		// Check for Int matching
		if subject.Type() == Int {
			intCases := make(map[int]*Block)
			rangeCases := make(map[IntRange]*Block)
			var catchAll *Block
			var intResultType Type

			for _, matchCase := range s.Cases {
				// Check if it's the default case (_)
				if id, ok := matchCase.Pattern.(*parse.Identifier); ok && id.Name == "_" {
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, catchAll.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if literal, ok := matchCase.Pattern.(*parse.NumLiteral); ok {
					// Convert string to int
					value, err := strconv.Atoi(literal.Value)
					if err != nil {
						legacy := fmt.Sprintf("Invalid integer literal: %s", literal.Value)
						c.addInvalidMatchPattern(legacy, matchCase.Pattern.GetLocation(), "this is not a valid integer pattern")
						return nil
					}
					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if unaryExpr, ok := matchCase.Pattern.(*parse.UnaryExpression); ok && unaryExpr.Operator == parse.Minus {
					// Handle negative numbers like -1, -5, etc.
					if literal, ok := unaryExpr.Operand.(*parse.NumLiteral); ok {
						// Convert string to int and negate
						value, err := strconv.Atoi(literal.Value)
						if err != nil {
							legacy := fmt.Sprintf("Invalid integer literal: %s", literal.Value)
							c.addInvalidMatchPattern(legacy, literal.GetLocation(), "this is not a valid integer pattern")
							return nil
						}
						negativeValue := -value
						caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
						intCases[negativeValue] = caseBlock
						var ok bool
						intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
						if !ok {
							return nil
						}
					} else {
						legacy := fmt.Sprintf("Invalid pattern for Int match: %T", matchCase.Pattern)
						c.addInvalidMatchPattern(legacy, matchCase.Pattern.GetLocation(), "expected an integer literal, range, enum variant, or `_`")
						return nil
					}
				} else if rangeExpr, ok := matchCase.Pattern.(*parse.RangeExpression); ok {
					// Handle range pattern like 1..10 or -10..5
					startValue, startErr := c.extractIntFromPattern(rangeExpr.Start)
					if startErr != nil {
						legacy := fmt.Sprintf("Invalid start value in range: %s", startErr.Error())
						c.addInvalidMatchPattern(legacy, rangeExpr.Start.GetLocation(), "range start must be an integer pattern")
						return nil
					}

					endValue, endErr := c.extractIntFromPattern(rangeExpr.End)
					if endErr != nil {
						legacy := fmt.Sprintf("Invalid end value in range: %s", endErr.Error())
						c.addInvalidMatchPattern(legacy, rangeExpr.End.GetLocation(), "range end must be an integer pattern")
						return nil
					}

					if startValue > endValue {
						c.addInvalidMatchPattern("Range start must be less than or equal to end", matchCase.Pattern.GetLocation(), "range start must not exceed its end")
						return nil
					}

					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					rangeCases[IntRange{Start: startValue, End: endValue}] = caseBlock
					var ok bool
					intResultType, ok = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				} else if staticProp, ok := matchCase.Pattern.(*parse.StaticProperty); ok {
					// Handle enum variant pattern like Status::active
					patternExpr := c.checkExpr(staticProp)
					if patternExpr == nil {
						continue // Error already reported by checkExpr
					}

					// Check if the pattern resolves to an enum variant
					enumVariant, ok := patternExpr.(*EnumVariant)
					if !ok {
						c.addInvalidMatchPattern("Pattern in Int match must be an integer literal, range, or enum variant", staticProp.GetLocation(), "this does not resolve to an enum variant")
						continue
					}

					// Extract the integer value from the enum variant's actual value (supports custom enum values)
					value := enumVariant.enum.Values[enumVariant.Variant].Value
					caseBlock := c.checkMatchArmBlock(matchCase.Body, nil)
					intCases[value] = caseBlock
					var mergeOK bool
					intResultType, mergeOK = mergeMatchResultType(c, intResultType, caseBlock.Type(), matchCase.Pattern.GetLocation(), allowMixedVoid)
					if !mergeOK {
						return nil
					}
				} else {
					legacy := fmt.Sprintf("Invalid pattern for Int match: %T", matchCase.Pattern)
					c.addInvalidMatchPattern(legacy, matchCase.Pattern.GetLocation(), "expected an integer literal, range, enum variant, or `_`")
					return nil
				}
			}

			// Validate that there is a catch-all case for Int match
			if catchAll == nil {
				c.addNonExhaustiveMatch("Incomplete match: missing catch-all case for Int match", s.GetLocation(), "add a catch-all `_` case")
			}

			return &IntMatch{
				Subject:    subject,
				IntCases:   intCases,
				RangeCases: rangeCases,
				CatchAll:   catchAll,
				ResultType: intResultType,
			}
		}

		legacy := fmt.Sprintf("Cannot match on %s", subject.Type())
		c.addDiagnostic(invalidMatchSubjectDiagnostic{Actual: subject.Type(), Span: c.sourceSpan(s.Subject.GetLocation()), LegacyMessage: legacy}.build())
		return nil
	case *parse.ConditionalMatchExpression:
		allowMixedVoid := discardThisExpr || c.expectedExpr == Void
		previousArmDiscard := c.matchArmDiscardContext
		c.matchArmDiscardContext = allowMixedVoid
		defer func() {
			c.matchArmDiscardContext = previousArmDiscard
		}()
		var cases []ConditionalCase
		var catchAll *Block
		var catchAllSpan *SourceSpan
		var conditionalResultType Type

		for _, matchCase := range s.Cases {
			if matchCase.Condition == nil {
				// This is a catch-all case (_)
				if catchAll != nil {
					c.addDuplicateMatchArm(Error, "Duplicate catch-all case", matchCase.GetLocation(), catchAllSpan)
				} else {
					span := c.sourceSpan(matchCase.GetLocation())
					catchAllSpan = &span
					catchAll = c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					conditionalResultType, ok = mergeMatchResultType(c, conditionalResultType, catchAll.Type(), matchCase.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}
				}
			} else {
				// Regular condition case
				if condition := c.checkExpr(matchCase.Condition); condition != nil {
					// Ensure condition is boolean
					if condition.Type() != Bool {
						c.addDiagnostic(nonBooleanMatchConditionDiagnostic{Actual: condition.Type(), Span: c.sourceSpan(matchCase.Condition.GetLocation())}.build())
					}

					body := c.checkMatchArmBlock(matchCase.Body, nil)
					var ok bool
					conditionalResultType, ok = mergeMatchResultType(c, conditionalResultType, body.Type(), matchCase.GetLocation(), allowMixedVoid)
					if !ok {
						return nil
					}

					cases = append(cases, ConditionalCase{
						Condition: condition,
						Body:      body,
					})
				}
			}
		}

		// Require catch-all case for conditional match to guarantee a return value
		if catchAll == nil {
			c.addNonExhaustiveMatch("Conditional match must include a catch-all (_) case", s.GetLocation(), "add a catch-all `_` case")
		}

		return &ConditionalMatch{
			Cases:      cases,
			CatchAll:   catchAll,
			ResultType: conditionalResultType,
		}
	case *parse.StaticProperty:
		{
			if id, ok := s.Target.(*parse.Identifier); ok {
				if goPkg := c.program.GoImports[id.Name]; goPkg != nil {
					switch prop := s.Property.(type) {
					case *parse.Identifier:
						if typ := goPkg.Constants[prop.Name]; typ != nil {
							return &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: typ}
						}
						if typ := goPkg.Variables[prop.Name]; typ != nil {
							return &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: typ, Assignable: true}
						}
						// An imported Go function is a first-class value with its
						// Ard-facing signature. Unadapted shapes reference the Go
						// function directly; adapted shapes (variadic, error and
						// comma-ok results) get a compiler-synthesized boundary
						// adapter so the value behaves exactly like a call.
						if def := goPkg.Functions[prop.Name]; def != nil {
							if reason := goPkg.AdaptedFunctions[prop.Name]; reason != "" {
								valueType, variadic, ok := adaptedGoFunctionValueType(def)
								if !ok {
									qualified := id.Name + "::" + prop.Name
									legacy := fmt.Sprintf("Go function %s cannot be referenced as a value: %s; wrap it in a closure", qualified, reason)
									c.addDiagnostic(invalidGoFunctionValueDiagnostic{Name: qualified, Detail: reason, Span: c.sourceSpan(prop.GetLocation()), LegacyMessage: legacy}.build())
									return nil
								}
								return &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: valueType, AdaptedFunction: true, VariadicAdapter: variadic}
							}
							return &ForeignValue{Target: "go", Namespace: goPkg.Path, Qualifier: goPkg.TypesName, Symbol: prop.Name, ValueType: def}
						}
						if _, isGeneric := goPkg.Generics[prop.Name]; isGeneric {
							qualified := id.Name + "::" + prop.Name
							legacy := fmt.Sprintf("Generic Go function %s cannot be referenced as a value; wrap it in a closure so its type parameters are fixed", qualified)
							c.addDiagnostic(invalidGoFunctionValueDiagnostic{Name: qualified, Span: c.sourceSpan(prop.GetLocation()), LegacyMessage: legacy, Generic: true}.build())
							return nil
						}
						if reason := goPkg.UnsupportedFunctions[prop.Name]; reason != "" {
							c.addUnsupportedGoEntity("function", id.Name+"::"+prop.Name, reason, "Unsupported Go function", prop.GetLocation())
							return nil
						}
						if reason := goPkg.UnsupportedConstants[prop.Name]; reason != "" {
							c.addUnsupportedGoEntity("constant", id.Name+"::"+prop.Name, reason, "Unsupported Go constant", prop.GetLocation())
							return nil
						}
						if reason := goPkg.UnsupportedVariables[prop.Name]; reason != "" {
							c.addUnsupportedGoEntity("variable", id.Name+"::"+prop.Name, reason, "Unsupported Go variable", prop.GetLocation())
							return nil
						}
						c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", id.Name, prop.Name), prop.GetLocation())
						return nil
					case *parse.StructInstance:
						typ := goPkg.Types[prop.Name.Name]
						foreign, ok := typ.(*ForeignType)
						if !ok {
							c.addUnresolvedReference(undefinedGoType, fmt.Sprintf("%s::%s", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}
						instance := c.validateForeignStructInstance(foreign, prop.TypeArgs, prop.Properties, prop.GetLocation())
						if instance == nil {
							return nil
						}
						return instance
					default:
						c.addError(fmt.Sprintf("Unsupported property type in %s::%s", id.Name, s.Property), s.Property.GetLocation())
						return nil
					}
				}

				// Check if this is accessing a module
				if mod := c.resolveModule(id.Name); mod != nil {
					switch prop := s.Property.(type) {
					case *parse.StructInstance:
						typeArgs, ok := c.resolveStructTypeArgs(prop)
						if !ok {
							return nil
						}
						// Look up the struct symbol directly from the module
						sym := mod.Get(prop.Name.Name)
						if sym.IsZero() {
							c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						structType, ok := sym.Type.(*StructDef)
						if !ok {
							c.addError(fmt.Sprintf("%s::%s is not a struct", id.Name, prop.Name.Name), prop.Name.GetLocation())
							return nil
						}

						// Use helper function for validation
						instance := c.validateStructInstance(structType, prop.Properties, prop.Name.Name, prop.GetLocation(), typeArgs)
						if instance == nil {
							return nil
						}

						// Pre-compute field types for the module instance
						fieldTypes := make(map[string]Type)
						maps.Copy(fieldTypes, structType.Fields)

						return &ModuleStructInstance{
							Module:     mod.Path(),
							Property:   instance,
							FieldTypes: fieldTypes,
							StructType: instance._type,
						}
					case *parse.Identifier:
						sym := mod.Get(prop.Name)
						if sym.IsZero() {
							c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", id.Name, prop.Name), prop.GetLocation())
							return nil
						}
						node := &ModuleSymbol{Module: mod.Path(), Symbol: Symbol{Name: prop.Name, Type: sym.Type}}
						c.recordTarget(prop, node, SpanTarget{Kind: TargetValue, Module: mod.Path(), Symbol: prop.Name})
						return node
					default:
						c.addError(fmt.Sprintf("Unsupported property type in %s::%s", id.Name, prop), s.Property.GetLocation())
						return nil
					}
				}

				// Handle local enum variants or static functions (not from modules)
				sym, ok := c.scope.get(id.Name)
				if !ok {
					c.addUnresolvedReference(undefinedStaticRoot, id.Name, id.GetLocation())
					return nil
				}

				// Check if it's accessing a static function (e.g., Fixture::from_api_entry)
				if propIdent, ok := s.Property.(*parse.Identifier); ok {
					staticFnName := id.Name + "::" + propIdent.Name
					if fnSym, ok := c.scope.get(staticFnName); ok {
						// Found a static function, return it as a function variable
						if fnDef, ok := fnSym.Type.(*FunctionDef); ok {
							return &Variable{Symbol{Name: staticFnName, Type: fnDef}}
						}
					}
				}

				// Check if it's an enum variant
				enum, ok := sym.Type.(*Enum)
				if !ok {
					c.addUnresolvedReference(invalidStaticMember, fmt.Sprintf("%s::%s", sym.Name, s.Property), id.GetLocation())
					return nil
				}

				variant := -1
				for i := range enum.Values {
					if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
						variant = i
						break
					}
				}
				if variant == -1 {
					c.addUnresolvedReference(undefinedEnumVariant, fmt.Sprintf("%s::%s", sym.Name, s.Property.(*parse.Identifier).Name), id.GetLocation())
					return nil
				}

				return &EnumVariant{
					enum:         enum,
					Variant:      variant,
					EnumType:     enum,
					Discriminant: enum.Values[variant].Value,
				}
			}
			// Handle nested static properties like http::Method::Get
			if _, ok := s.Target.(*parse.StaticProperty); ok {
				// First resolve the nested static property (e.g., http::Method)
				nestedSym := c.checkExpr(s.Target)
				if nestedSym == nil {
					return nil
				}

				// Check if it's an enum type
				if enum, ok := nestedSym.Type().(*Enum); ok {
					// Find the variant
					variant := -1
					for i := range enum.Values {
						if enum.Values[i].Name == s.Property.(*parse.Identifier).Name {
							variant = i
							break
						}
					}
					if variant == -1 {
						c.addUnresolvedReference(undefinedEnumVariant, fmt.Sprintf("%s::%s", enum.Name, s.Property.(*parse.Identifier).Name), s.Property.GetLocation())
						return nil
					}

					return &EnumVariant{
						enum:         enum,
						Variant:      variant,
						EnumType:     enum,
						Discriminant: enum.Values[variant].Value,
					}
				}

				c.addError(fmt.Sprintf("Cannot access property on %T", nestedSym.Type()), s.Property.GetLocation())
				return nil
			}
			panic(fmt.Errorf("Unexpected static property target: %T", s.Target))
		}
	case *parse.StructInstance:
		typeArgs, argsOk := c.resolveStructTypeArgs(s)
		if !argsOk {
			return nil
		}
		name := s.Name.Name
		sym, ok := c.scope.get(name)
		if !ok {
			c.addUnresolvedReference(undefinedStructType, name, s.GetLocation())
			return nil
		}

		structType, ok := sym.Type.(*StructDef)
		if !ok {
			c.addUnresolvedReference(notAStruct, name, s.GetLocation())
			return nil
		}
		if !strings.Contains(name, "::") {
			c.recordTypeRef(s.Name.GetLocation(), name)
		}

		// Use helper function for validation
		return c.validateStructInstance(structType, s.Properties, name, s.GetLocation(), typeArgs)
	case *parse.Try:
		{
			if c.deferredWorkDepth > 0 {
				c.addDiagnostic(invalidTryDiagnostic{LegacyMessage: "try is not allowed inside deferred work; handle the Result or Maybe explicitly", Span: c.sourceSpan(s.GetLocation()), Label: "`try` cannot propagate out of deferred work"}.build())
				return nil
			}
			// Check if this is a property/method accessor chain that might need cascading Maybe handling
			expr := c.tryCheckAccessorChain(s.Expression)
			if expr == nil {
				return nil
			}

			if c.scope.getReturnType() == nil {
				c.addDiagnostic(invalidTryDiagnostic{LegacyMessage: "The `try` keyword can only be used in a function body", Span: c.sourceSpan(s.GetLocation()), Label: "`try` requires an enclosing function return context"}.build())
				return nil
			}

			var catchBlock []Statement

			switch _type := expr.Type().(type) {
			case *Result:
				// Handle catch clause if present
				if s.CatchVar != nil && s.CatchBlock != nil {
					// Create new scope for catch block with error variable
					prevScope := c.scope
					newScope := makeScope(prevScope)
					c.scope = &newScope

					// Add error variable to scope with the error type
					c.recordBinding(s.CatchVar.GetLocation(), c.scope.add(s.CatchVar.Name, _type.err, false))

					// Check catch block statements
					for _, stmt := range s.CatchBlock {
						checkedStmt := c.checkStmt(&stmt)
						if checkedStmt != nil {
							catchBlock = append(catchBlock, *checkedStmt)
						}
					}

					// Restore previous scope
					c.scope = prevScope

					// Create the block
					block := &Block{Stmts: catchBlock}
					blockType := block.Type()
					returnType := c.scope.getReturnType()
					catchLocation := bodyResultLocation(s.CatchBlock, s.GetLocation())

					// Validate catch block type compatibility
					// If both are Results, only error types need to match (value types can differ, including generic $Val)
					var typeOk bool
					if fnReturnResult, ok := returnType.(*Result); ok {
						if blockResultType, ok := blockType.(*Result); ok {
							typeOk = fnReturnResult.err.equal(blockResultType.err)
							if !typeOk {
								legacyMessage := fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), blockResultType.err.String())
								c.addTypeMismatchWithLegacy(fnReturnResult.err, blockResultType.err, legacyMessage, catchLocation)
							}
						} else {
							// Catch block returns non-Result but function expects Result
							typeOk = false
							c.addTypeMismatch(returnType, blockType, catchLocation)
						}
					} else {
						// Function return type is not a Result
						typeOk = returnType.equal(blockType)
						if !typeOk {
							c.addTypeMismatch(returnType, blockType, catchLocation)
						}
					}

					// With catch clause, try returns the unwrapped value type on success
					// On error, it early returns with the catch block result
					return &TryOp{
						expr:       expr,
						ok:         _type.val, // Returns unwrapped value for continued execution
						OkType:     _type.val,
						ErrType:    _type.err,
						CatchBlock: block,
						CatchVar:   s.CatchVar.Name,
						Kind:       TryResult,
					}
				} else {
					// No catch clause: function must return a compatible Result type
					fnReturnResult, ok := c.scope.getReturnType().(*Result)
					if !ok {
						c.addDiagnostic(invalidTryDiagnostic{LegacyMessage: "try without catch clause requires function to return a Result type", Span: c.sourceSpan(s.GetLocation()), Label: "uncaught Result errors require the function to return a Result"}.build())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:    expr,
							ok:      _type.val,
							OkType:  _type.val,
							ErrType: _type.err,
							Kind:    TryResult,
						}
					}

					// Error types must match for direct propagation
					if !_type.err.equal(fnReturnResult.err) {
						legacyMessage := fmt.Sprintf("Error type mismatch: Expected %s, got %s", fnReturnResult.err.String(), _type.err.String())
						c.addTypeMismatchWithLegacy(fnReturnResult.err, _type.err, legacyMessage, s.Expression.GetLocation())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:    expr,
							ok:      _type.val,
							OkType:  _type.val,
							ErrType: _type.err,
							Kind:    TryResult,
						}
					}

					// Success: returns the unwrapped value
					// Error: early returns the error wrapped in the function's Result type
					return &TryOp{
						expr:    expr,
						ok:      _type.val,
						OkType:  _type.val,
						ErrType: _type.err,
						Kind:    TryResult,
					}
				}
			case *Maybe:
				// Handle catch clause if present
				if s.CatchVar != nil && s.CatchBlock != nil {
					// Create new scope for catch block - no variable binding for Maybe catch
					prevScope := c.scope
					newScope := makeScope(prevScope)
					c.scope = &newScope

					// Check catch block statements
					for _, stmt := range s.CatchBlock {
						checkedStmt := c.checkStmt(&stmt)
						if checkedStmt != nil {
							catchBlock = append(catchBlock, *checkedStmt)
						}
					}

					// Restore previous scope
					c.scope = prevScope

					// Create the block
					block := &Block{Stmts: catchBlock}
					blockType := block.Type()
					returnType := c.scope.getReturnType()
					catchLocation := bodyResultLocation(s.CatchBlock, s.GetLocation())

					// Validate catch block type compatibility
					// For Maybe catch blocks, inner types must match (or both have unresolved generics)
					var typeOk bool
					if fnReturnMaybe, ok := returnType.(*Maybe); ok {
						if blockMaybeType, ok := blockType.(*Maybe); ok {
							// Both are Maybe types - inner types should match
							typeOk = fnReturnMaybe.of.equal(blockMaybeType.of)
							if !typeOk {
								c.addTypeMismatch(returnType, blockType, catchLocation)
							}
						} else {
							typeOk = false
							c.addTypeMismatch(returnType, blockType, catchLocation)
						}
					} else {
						// Function return type is not a Maybe
						typeOk = returnType.equal(blockType)
						if !typeOk {
							c.addTypeMismatch(returnType, blockType, catchLocation)
						}
					}

					// With catch clause, try returns the unwrapped value type on success
					// On none, it early returns with the catch block result
					return &TryOp{
						expr:       expr,
						ok:         _type.of, // Returns unwrapped value for continued execution
						OkType:     _type.of,
						CatchBlock: block,
						CatchVar:   "", // No variable binding for Maybe catch
						Kind:       TryMaybe,
					}
				} else {
					// No catch clause: function must return a compatible Maybe type
					fnReturnMaybe, ok := c.scope.getReturnType().(*Maybe)
					if !ok {
						c.addDiagnostic(invalidTryDiagnostic{LegacyMessage: "try without catch clause on Maybe requires function to return a Maybe type", Span: c.sourceSpan(s.GetLocation()), Label: "uncaught absence requires the function to return a Maybe"}.build())
						// Return a try op with the unwrapped type to avoid cascading errors
						return &TryOp{
							expr:   expr,
							ok:     _type.of,
							OkType: _type.of,
							Kind:   TryMaybe,
						}
					}

					// When try fails on Maybe, it early returns none wrapped in the function's Maybe type
					// The inner types don't need to match since we're not directly returning the unwrapped value
					// The unwrapped value (on success) can be any type and will be used in subsequent computations
					_ = fnReturnMaybe // We just need to verify the function returns a Maybe type

					// Success: returns the unwrapped value
					// None: early returns none wrapped in the function's Maybe type
					return &TryOp{
						expr:   expr,
						ok:     _type.of,
						OkType: _type.of,
						Kind:   TryMaybe,
					}
				}
			default:
				legacy := "try can only be used on Result or Maybe types, got: " + expr.Type().String()
				c.addDiagnostic(invalidTryDiagnostic{LegacyMessage: legacy, Span: c.sourceSpan(s.Expression.GetLocation()), Label: fmt.Sprintf("this expression has type `%s`, not Result or Maybe", expr.Type())}.build())
				// Return a try op with the expr type to avoid cascading errors
				return &TryOp{
					expr:    expr,
					ok:      expr.Type(),
					OkType:  expr.Type(),
					ErrType: Void,
					Kind:    TryResult, // Default to Result, though this is an error path
				}
			}
		}
	case *parse.UnsafeBlock:
		{
			if parseStatementsContainBreak(s.Statements) {
				c.addDiagnostic(invalidBreakDiagnostic{Span: c.sourceSpan(s.GetLocation()), LegacyMessage: "break is not allowed inside unsafe blocks", Unsafe: true}.build())
			}
			var expectedValue Type
			if expectedResult, ok := c.expectedExpr.(*Result); ok {
				expectedValue = expectedResult.val
			}
			unsafeReturnType := MakeResult(&TypeVar{name: "Unsafe"}, Str)
			var block *Block
			if expectedValue != nil {
				block = c.checkBlockWithExpected(s.Statements, func() {
					c.scope.expectReturn(unsafeReturnType)
					c.scope.inUnsafe = true
				}, expectedValue, false)
			} else {
				block = c.checkBlockWithInferredFinalValue(s.Statements, func() {
					c.scope.expectReturn(unsafeReturnType)
					c.scope.inUnsafe = true
				}, discardThisExpr || c.expectedExpr == Void)
			}
			valueType := block.Type()
			resultType := MakeResult(valueType, Str)
			c.validateUnsafeCatchResults(block, resultType, s.GetLocation())
			return &UnsafeBlock{Body: block, ValueType: valueType, ResultType: resultType}
		}
	case *parse.BlockExpression:
		{
			block := c.checkBlockWithInferredFinalValue(s.Statements, nil, discardThisExpr || c.expectedExpr == Void)
			return block
		}
	default:
		panic(fmt.Errorf("Unexpected expression: %s", reflect.TypeOf(s)))
	}
}

func (c *Checker) parseRuneLiteralValue(literal *parse.RuneLiteral) (rune, bool) {
	runes := []rune(literal.Value)
	if len(runes) != 1 || !utf8.ValidRune(runes[0]) {
		c.addError("Rune literal must contain exactly one Unicode scalar value", literal.GetLocation())
		return 0, false
	}
	return runes[0], true
}

// extractIntFromPattern extracts an integer value from a pattern that can be either
// a NumLiteral or a UnaryExpression with minus operator applied to a NumLiteral
func (c *Checker) extractIntFromPattern(expr parse.Expression) (int, error) {
	switch e := expr.(type) {
	case *parse.NumLiteral:
		return strconv.Atoi(e.Value)
	case *parse.UnaryExpression:
		if e.Operator == parse.Minus {
			if literal, ok := e.Operand.(*parse.NumLiteral); ok {
				value, err := strconv.Atoi(literal.Value)
				if err != nil {
					return 0, err
				}
				return -value, nil
			}
		}
		return 0, fmt.Errorf("unsupported unary expression in pattern")
	default:
		return 0, fmt.Errorf("pattern must be an integer literal or negative integer")
	}
}

// isEnum checks if a type is an Enum
func (c *Checker) isEnum(t Type) bool {
	_, ok := t.(*Enum)
	return ok
}

// areTypesComparable checks if two types can be compared together
// This allows Enum vs Int and Int vs Enum comparisons
func (c *Checker) areTypesComparable(left, right Type) bool {
	// Same type is always comparable
	if left.equal(right) {
		return true
	}
	// Allow Enum vs Int comparisons
	leftIsEnum := c.isEnum(left)
	rightIsEnum := c.isEnum(right)

	if (leftIsEnum && right == Int) || (left == Int && rightIsEnum) {
		return true
	}

	return false
}

// use this when we know what the expr's Type should be
func bindInferredTypeVars(expected Type, actual Type) {
	if expected == nil || actual == nil {
		return
	}

	expected = derefType(expected)
	actual = derefType(actual)

	switch exp := expected.(type) {
	case *TypeVar:
		if exp.actual == nil {
			exp.actual = actual
			exp.bound = true
			return
		}
		bindInferredTypeVars(exp.actual, actual)
	case *Maybe:
		if act, ok := actual.(*Maybe); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Result:
		if act, ok := actual.(*Result); ok {
			bindInferredTypeVars(exp.Val(), act.Val())
			bindInferredTypeVars(exp.Err(), act.Err())
		}
	case *List:
		if act, ok := actual.(*List); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Chan:
		if act, ok := actual.(*Chan); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Receiver:
		if act, ok := actual.(*Receiver); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Sender:
		if act, ok := actual.(*Sender); ok {
			bindInferredTypeVars(exp.Of(), act.Of())
		}
	case *Map:
		if act, ok := actual.(*Map); ok {
			bindInferredTypeVars(exp.Key(), act.Key())
			bindInferredTypeVars(exp.Value(), act.Value())
		}
	case *StructDef:
		if act, ok := actual.(*StructDef); ok && exp.Name == act.Name && !namedTypeOwnersDiffer(exp.ModulePath, act.ModulePath) {
			limit := len(exp.TypeArgs)
			if len(act.TypeArgs) < limit {
				limit = len(act.TypeArgs)
			}
			for i := 0; i < limit; i++ {
				bindInferredTypeVars(exp.TypeArgs[i], act.TypeArgs[i])
			}
			for fieldName, expectedField := range exp.Fields {
				if actualField, ok := act.Fields[fieldName]; ok {
					bindInferredTypeVars(expectedField, actualField)
				}
			}
		}
	case *FunctionDef:
		if act, ok := actual.(*FunctionDef); ok {
			limit := len(exp.Parameters)
			if len(act.Parameters) < limit {
				limit = len(act.Parameters)
			}
			for i := 0; i < limit; i++ {
				bindInferredTypeVars(exp.Parameters[i].Type, act.Parameters[i].Type)
			}
			bindInferredTypeVars(exp.ReturnType, act.ReturnType)
		}
	}
}

func (c *Checker) checkNumericLiteralAs(expr parse.Expression, expected Type) Expression {
	if unary, ok := expr.(*parse.UnaryExpression); ok && unary.Operator == parse.Minus {
		if num, ok := unary.Operand.(*parse.NumLiteral); ok {
			return c.checkSignedNumericLiteralAs(num, expected, true)
		}
	}
	num, ok := expr.(*parse.NumLiteral)
	if !ok || expected == nil {
		return nil
	}
	return c.checkSignedNumericLiteralAs(num, expected, false)
}

func (c *Checker) checkSignedNumericLiteralAs(num *parse.NumLiteral, expected Type, negative bool) Expression {
	literalType := expected
	if foreign, ok := expected.(*ForeignType); ok {
		literalType = foreign.Underlying
	}
	if expected == nil || (!isIntegerScalar(literalType) && literalType != Float32 && literalType != Float64) {
		return nil
	}
	literalText := num.Value
	if negative {
		literalText = "-" + literalText
	}
	if strings.Contains(num.Value, ".") {
		clean := strings.ReplaceAll(literalText, "_", "")
		value, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			c.addError(fmt.Sprintf("Invalid float: %s", num.Value), num.GetLocation())
			return nil
		}
		if literalType == Float64 {
			if expected != literalType {
				return &TypedFloatLiteral{Value: value, Text: clean, Typed: expected}
			}
			return &FloatLiteral{Value: value}
		}
		if literalType == Float32 {
			float32Value, err := strconv.ParseFloat(clean, 32)
			if err != nil {
				c.addError(fmt.Sprintf("Float literal %s overflows Float32", num.Value), num.GetLocation())
				return &TypedFloatLiteral{Value: float32Value, Text: clean, Typed: expected}
			}
			return &TypedFloatLiteral{Value: float32Value, Text: clean, Typed: expected}
		}
		return nil
	}
	clean := strings.ReplaceAll(literalText, "_", "")
	if isUnsignedScalar(literalType) {
		value := new(big.Int)
		if _, ok := value.SetString(clean, 0); !ok || value.Sign() < 0 || !c.uintLiteralFitsType(value, literalType) {
			c.addError(fmt.Sprintf("Integer literal %s overflows %s", literalText, expected), num.GetLocation())
		}
		return &TypedIntLiteral{Value: int(value.Int64()), Text: clean, Typed: expected}
	}
	if literalType == Float32 || literalType == Float64 {
		return nil
	}
	value64, err := strconv.ParseInt(clean, 0, 64)
	if err != nil {
		c.addError(fmt.Sprintf("Invalid int: %s", literalText), num.GetLocation())
		return nil
	}
	if !c.intLiteralFitsType(value64, literalType) {
		c.addError(fmt.Sprintf("Integer literal %s overflows %s", literalText, expected), num.GetLocation())
		if isIntegerScalar(literalType) {
			return &TypedIntLiteral{Value: int(value64), Text: clean, Typed: expected}
		}
		return nil
	}
	if expected == Int {
		return &IntLiteral{Value: int(value64)}
	}
	if isIntegerScalar(literalType) {
		return &TypedIntLiteral{Value: int(value64), Text: clean, Typed: expected}
	}
	return nil
}

// foreignScalarNarrows reports whether actual is a foreign named scalar that
// satisfies expected only through the narrowing coercion to its underlying
// primitive. The coercion produces a converted value, not the original place,
// so mutable parameters and equality contexts must reject it.
func foreignScalarNarrows(expected Type, actual Type) bool {
	foreign, ok := actual.(*ForeignType)
	return ok && !foreign.Pointer && foreign.Underlying != nil && isPrimitiveScalar(expected) && foreign.Underlying.equal(expected)
}

// foreignScalarWidens reports whether actual is a primitive that satisfies
// expected only through the widening coercion into a foreign named scalar
// type (e.g. Str -> ui::IntentType). Restricted to Str and Bool underlyings,
// where the Go conversion is total; numeric widenings could truncate and stay
// explicit. Like narrowing, the coercion produces a converted value, so
// mutable places and identity contexts must reject it.
func foreignScalarWidens(expected Type, actual Type) bool {
	foreign, ok := expected.(*ForeignType)
	if !ok || foreign.Pointer || foreign.Interface || foreign.Struct || foreign.Underlying == nil {
		return false
	}
	if !foreign.Underlying.equal(Str) && !foreign.Underlying.equal(Bool) {
		return false
	}
	return foreign.Underlying.equal(actual)
}

// ValidForeignScalarConversion reports whether a ForeignScalarConvert between
// the two types is a supported foreign-scalar coercion in either direction.
// The AIR lowering asserts this so future widening of the checker-side gates
// cannot silently emit an unchecked Go conversion.
func ValidForeignScalarConversion(from, to Type) bool {
	if prim := foreignScalarPrimitive(from); prim != nil && prim.equal(to) {
		return true
	}
	if prim := foreignScalarPrimitive(to); prim != nil && prim.equal(from) {
		return true
	}
	return false
}

// foreignScalarPrimitive returns the underlying Ard primitive of a foreign
// named scalar type, or nil when t is not one.
func foreignScalarPrimitive(t Type) Type {
	foreign, ok := t.(*ForeignType)
	if !ok || foreign.Pointer || foreign.Interface || foreign.Struct || foreign.Underlying == nil {
		return nil
	}
	if !isPrimitiveScalar(foreign.Underlying) {
		return nil
	}
	return foreign.Underlying
}

// foreignFuncCoerces reports whether compatibility between expected and actual
// relies on the named/unnamed Go func coercion in either direction. Contexts
// that require exact Go type identity (such as Go interface conformance)
// must exclude this coercion: the generated Go method signature has to match
// the interface's signature exactly.
func foreignFuncCoerces(expected Type, actual Type) bool {
	if foreign, ok := expected.(*ForeignType); ok && !foreign.Pointer {
		if _, ok := foreign.Underlying.(*FunctionDef); ok {
			if _, isFn := actual.(*FunctionDef); isFn {
				return true
			}
		}
	}
	if foreign, ok := actual.(*ForeignType); ok && !foreign.Pointer {
		if _, ok := foreign.Underlying.(*FunctionDef); ok {
			if _, isFn := expected.(*FunctionDef); isFn {
				return true
			}
		}
	}
	return false
}

func isPrimitiveScalar(t Type) bool {
	if t == nil {
		return false
	}
	return t.equal(Int) || t.equal(Float64) || t.equal(Str) || t.equal(Bool) || t.equal(Byte) || t.equal(Rune) || isExplicitScalar(t)
}

func isExplicitScalar(t Type) bool {
	switch t {
	case Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Uintptr, Float32:
		return true
	default:
		return false
	}
}

func isIntegerScalar(t Type) bool {
	switch t {
	case Int, Int8, Int16, Int32, Int64, Uint, Uint8, Uint16, Uint32, Uint64, Uintptr, Byte, Rune:
		return true
	default:
		return false
	}
}

func isRelationalIntegerLike(t Type) bool {
	return isIntegerScalar(t) || isIntegerScalar(foreignScalarPrimitive(t))
}

func isRelationalFloatLike(t Type) bool {
	if t == Float64 || t == Float32 {
		return true
	}
	prim := foreignScalarPrimitive(t)
	return prim == Float64 || prim == Float32
}

func isArithmeticIntegerLike(t Type) bool { return isRelationalIntegerLike(t) }

func isSignedArithmeticLike(t Type) bool {
	switch t {
	case Int, Int8, Int16, Int32, Int64, Float32, Float64:
		return true
	}
	switch foreignScalarPrimitive(t) {
	case Int, Int8, Int16, Int32, Int64, Float32, Float64:
		return true
	default:
		return false
	}
}

func isArithmeticFloatLike(t Type) bool { return isRelationalFloatLike(t) }

// contextualScalarOperandType returns the scalar type an untyped numeric
// literal operand should adopt from the other operand, or nil when default
// literal typing applies. Sized Ard scalars (Int16, Float32, Byte, ...) and
// foreign named scalars (time::Duration) qualify; plain Int and Float64 stay
// on the default path.
func contextualScalarOperandType(t Type) Type {
	if isExplicitScalar(t) || t == Byte || t == Rune {
		return t
	}
	if prim := foreignScalarPrimitive(t); prim != nil && prim != Str && prim != Bool {
		return t
	}
	return nil
}

// isUntypedNumLiteral reports whether an expression is a numeric literal
// (optionally negated) that can adopt a scalar type from context, matching
// Go's untyped-constant behavior for expressions like `5 * time::Second`.
func isUntypedNumLiteral(expr parse.Expression) bool {
	switch e := expr.(type) {
	case *parse.NumLiteral:
		return true
	case *parse.UnaryExpression:
		if e.Operator != parse.Minus {
			return false
		}
		_, ok := e.Operand.(*parse.NumLiteral)
		return ok
	}
	return false
}

// checkScalarOperands checks a binary operator's operands, letting an
// untyped numeric literal adopt the other operand's scalar type.
func (c *Checker) checkScalarOperands(leftExpr, rightExpr parse.Expression) (Expression, Expression) {
	leftLit := isUntypedNumLiteral(leftExpr)
	rightLit := isUntypedNumLiteral(rightExpr)
	if leftLit == rightLit {
		return c.checkExpr(leftExpr), c.checkExpr(rightExpr)
	}
	if leftLit {
		right := c.checkExpr(rightExpr)
		if right == nil {
			return nil, nil
		}
		if target := contextualScalarOperandType(right.Type()); target != nil {
			return c.checkExprAs(leftExpr, target), right
		}
		return c.checkExpr(leftExpr), right
	}
	left := c.checkExpr(leftExpr)
	if left == nil {
		return nil, nil
	}
	if target := contextualScalarOperandType(left.Type()); target != nil {
		return left, c.checkExprAs(rightExpr, target)
	}
	return left, c.checkExpr(rightExpr)
}

func isUnsignedScalar(t Type) bool {
	switch t {
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return true
	default:
		return false
	}
}

func (c *Checker) targetIntBits() int {
	if c.options.Target.IntBits != 0 {
		return c.options.Target.IntBits
	}
	return strconv.IntSize
}

func (c *Checker) targetUintBits() int {
	if c.options.Target.UintBits != 0 {
		return c.options.Target.UintBits
	}
	return c.targetIntBits()
}

func (c *Checker) targetUintptrBits() int {
	if c.options.Target.UintptrBits != 0 {
		return c.options.Target.UintptrBits
	}
	return c.targetIntBits()
}

func unsignedMax(bits int) *big.Int {
	if bits <= 0 {
		bits = strconv.IntSize
	}
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), uint(bits)), big.NewInt(1))
}

func (c *Checker) uintLiteralFitsType(value *big.Int, t Type) bool {
	if value.Sign() < 0 {
		return false
	}
	switch t {
	case Uint:
		return value.Cmp(unsignedMax(c.targetUintBits())) <= 0
	case Uintptr:
		return value.Cmp(unsignedMax(c.targetUintptrBits())) <= 0
	case Uint64:
		return value.Cmp(unsignedMax(64)) <= 0
	case Uint8:
		return value.Cmp(big.NewInt(math.MaxUint8)) <= 0
	case Uint16:
		return value.Cmp(big.NewInt(math.MaxUint16)) <= 0
	case Uint32:
		return value.Cmp(big.NewInt(math.MaxUint32)) <= 0
	default:
		return false
	}
}

func (c *Checker) intLiteralFitsType(value int64, t Type) bool {
	switch t {
	case Int:
		bits := c.targetIntBits()
		if bits <= 0 {
			bits = strconv.IntSize
		}
		if bits >= 64 {
			return true
		}
		min := -(int64(1) << (bits - 1))
		max := (int64(1) << (bits - 1)) - 1
		return value >= min && value <= max
	case Int64:
		return true
	case Int8:
		return value >= math.MinInt8 && value <= math.MaxInt8
	case Int16:
		return value >= math.MinInt16 && value <= math.MaxInt16
	case Int32, Rune:
		return value >= math.MinInt32 && value <= math.MaxInt32
	case Uint, Uint64, Uintptr:
		return value >= 0
	case Uint8, Byte:
		return value >= 0 && value <= math.MaxUint8
	case Uint16:
		return value >= 0 && value <= math.MaxUint16
	case Uint32:
		return value >= 0 && value <= math.MaxUint32
	default:
		return false
	}
}

func (c *Checker) checkExprAs(expr parse.Expression, expectedType Type) Expression {
	return c.checkExprAsWithExpectation(expr, expectedType, nil)
}

func (c *Checker) checkExprAsArgument(expr parse.Expression, expectedType Type, parameter Parameter) Expression {
	result := c.checkExprAsInner(expr, expectedType, nil, &parameter)
	if result != nil {
		result = coerceDiscardingFunction(expectedType, result)
		c.recordExprSpan(expr, result)
	}
	return result
}

func (c *Checker) checkExprAsWithExpectation(expr parse.Expression, expectedType Type, expectation *typeExpectation) Expression {
	result := c.checkExprAsInner(expr, expectedType, expectation, nil)
	if result != nil {
		result = coerceDiscardingFunction(expectedType, result)
		c.recordExprSpan(expr, result)
	}
	return result
}

func coerceDiscardingFunction(expected Type, value Expression) Expression {
	target, actual, ok := discardingFunctionTypes(expected, value.Type())
	if !ok {
		return value
	}
	for i := range target.Parameters {
		want := target.Parameters[i]
		got := actual.Parameters[i]
		if want.Mutable != got.Mutable || want.Variadic != got.Variadic || !want.Type.equal(got.Type) {
			return value
		}
	}
	return &DiscardingFunctionCoercion{Value: value, TargetType: target}
}

func discardingFunctionTypes(expected Type, actual Type) (*FunctionDef, *FunctionDef, bool) {
	target, ok := expected.(*FunctionDef)
	if !ok || target.ReturnType != Void {
		return nil, nil, false
	}
	source, ok := actual.(*FunctionDef)
	if !ok || source.ReturnType == nil || source.ReturnType == Void || len(target.Parameters) != len(source.Parameters) {
		return nil, nil, false
	}
	for i := range target.Parameters {
		if target.Parameters[i].Mutable != source.Parameters[i].Mutable || target.Parameters[i].Variadic != source.Parameters[i].Variadic {
			return nil, nil, false
		}
	}
	return target, source, true
}

func (c *Checker) checkExprAsInner(expr parse.Expression, expectedType Type, expectation *typeExpectation, argumentParameter *Parameter) Expression {
	if literal := c.checkNumericLiteralAs(expr, expectedType); literal != nil {
		return literal
	}
	switch s := (expr).(type) {
	case *parse.MatchExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.SelectExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.IfStatement:
		// A value is expected, so the chain must be exhaustive: without an
		// else there is a path that produces nothing (issue #267).
		if expectedType != nil && expectedType != Void && !ifChainHasElse(s) {
			c.addDiagnostic(nonExhaustiveValueIfDiagnostic{IfSpan: c.sourceSpan(s.GetLocation())}.build())
			return nil
		}
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.ConditionalMatchExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.BlockExpression:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.UnsafeBlock:
		return c.withExpectedExpr(expectedType, func() Expression {
			return c.checkExpr(s)
		})
	case *parse.ListLiteral:
		// Only use collection-specific inference when the expected type is a list or fixed array.
		switch expected := expectedType.(type) {
		case *List, *FixedArray:
			if result := c.checkList(expectedType, s); result != nil {
				return result
			}
			return nil
		case *ForeignType:
			if !expected.Pointer && expected.Underlying != nil {
				if _, ok := expected.Underlying.(*FixedArray); ok {
					if result := c.checkList(expected, s); result != nil {
						return result
					}
					return nil
				}
			}
			if result := c.checkList(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *parse.MapLiteral:
		// Only use collection-specific inference when the expected type is a map.
		if _, ok := expectedType.(*Map); ok {
			if result := c.checkMap(expectedType, s); result != nil {
				return result
			}
			return nil
		}
	case *parse.AnonymousFunction:
		{
			// Try to infer types from the expected type
			expectedFnType := expectedFunctionTypeForClosure(expectedType)
			if expectedFnType == nil {
				// Not a function type (or not a type signature), check normally
				return c.checkExpr(s)
			}

			// Check parameter count
			if len(s.Parameters) != len(expectedFnType.Parameters) {
				c.addArgumentCount(fmt.Sprint(len(expectedFnType.Parameters)), len(s.Parameters), s.GetLocation(), "")
				return nil
			}

			// Resolve parameters with type inference from expected function
			params := c.resolveParametersWithContext(s.Parameters, expectedFnType)
			returnType := c.resolveReturnTypeWithContext(s.ReturnType, expectedFnType)

			// Create function definition
			uniqueName := fmt.Sprintf("anon_func_%p", s)
			fn := &FunctionDef{
				Name:                    uniqueName,
				Parameters:              params,
				ReturnType:              returnType,
				InferReturnTypeFromBody: false,
				Body:                    nil,
			}

			// Check body
			c.pushFunctionGenericContext(fn)
			previousDeferredWorkDepth := c.deferredWorkDepth
			c.deferredWorkDepth = 0
			body := c.checkBlockWithExpected(s.Body, func() {
				c.scope.expectReturn(returnType)
				for _, param := range params {
					c.recordBinding(param.Loc, c.scope.add(param.Name, param.Type, param.Mutable))
				}
			}, returnType, true)
			c.deferredWorkDepth = previousDeferredWorkDepth
			c.popFunctionGenericContext()

			// Add function to scope after checking body
			c.scope.add(uniqueName, fn, false)

			// Nuance: in context-inferred callbacks (map/map_err/and_then, etc), the expected return
			// type can contain unbound generics (for example Result<$Mapped, E>). If these remain
			// unresolved, later method/property lookup can panic with
			// "Cannot look up symbols in unrefined $T". Bind generics from the checked body type here.
			//
			// This is a shared anonymous-function inference path, so changes here affect closure typing
			// beyond Result/Maybe combinators.
			bindInferredTypeVars(returnType, body.Type())

			// Validate return type
			if returnType != Void && !c.areCompatible(returnType, body.Type()) {
				c.addBodyReturnMismatch(s.Body, returnType, body.Type(), s.GetLocation(), s.ReturnType)
				return nil
			}

			fn.Body = body
			return fn
		}
	case *parse.InstanceMethod:
		{
			subj := c.checkExpr(s.Target)
			if subj == nil {
				c.addError(fmt.Sprintf("Cannot access %s on Void", s.Method.Name), s.Method.GetLocation())
				return nil
			}
		}
	case *parse.StaticFunction:
		{
			resultType, expectResult := expectedType.(*Result)
			target, ok := s.Target.(*parse.Identifier)
			if !expectResult || resultType == nil || !ok || target.Name != "Result" || (s.Function.Name != "ok" && s.Function.Name != "err") {
				break
			}

			moduleName := target.Name
			mod := c.resolveModule(moduleName)
			if mod == nil {
				c.addUnresolvedReference(undefinedStaticRoot, moduleName, s.GetLocation())
				return nil
			}

			sym := mod.Get(s.Function.Name)
			if sym.IsZero() {
				c.addUnresolvedReference(undefinedQualifiedMember, fmt.Sprintf("%s::%s", moduleName, s.Function.Name), s.GetLocation())
				return nil
			}

			// Handle both regular functions and external functions
			var fnDef *FunctionDef
			var isFunc bool
			switch fn := sym.Type.(type) {
			case *FunctionDef:
				fnDef = fn
				isFunc = true
			default:
				isFunc = false
			}

			if !isFunc {
				c.addNonCallable(fmt.Sprintf("%s::%s", moduleName, s.Function.Name), s.GetLocation(), nil, nonCallableSuffix)
				return nil
			}

			if len(s.Function.Args) != len(fnDef.Parameters) {
				c.addArgumentCount(fmt.Sprint(len(fnDef.Parameters)), len(s.Function.Args), s.GetLocation(), "")
				return nil
			}

			var arg Expression = nil
			if fnDef.name() == "ok" {
				arg = c.checkExpr(s.Function.Args[0].Value)
				if arg == nil {
					return nil
				}
				if !resultType.Val().equal(arg.Type()) {
					c.addTypeMismatch(resultType.Val(), arg.Type(), s.Function.Args[0].Value.GetLocation())
					return nil
				}
				bindInferredTypeVars(resultType.Val(), arg.Type())
			}
			if fnDef.name() == "err" {
				arg = c.checkExpr(s.Function.Args[0].Value)
				if arg == nil {
					return nil
				}
				if !resultType.Err().equal(arg.Type()) {
					c.addTypeMismatch(resultType.Err(), arg.Type(), s.Function.Args[0].Value.GetLocation())
					return nil
				}
				bindInferredTypeVars(resultType.Err(), arg.Type())
			}

			fnDef.ReturnType = resultType
			return &ModuleFunctionCall{
				Module: mod.Path(),
				Call: &FunctionCall{
					Name:       fnDef.name(),
					Args:       []Expression{arg},
					fn:         fnDef,
					ReturnType: fnDef.ReturnType,
				},
			}
		}
	}

	checked := c.checkExpr(expr)
	if checked == nil {
		return nil
	}
	checked = coerceDiscardingFunction(expectedType, checked)

	if expectedType == Void {
		return checked
	}

	if !c.areCompatible(expectedType, checked.Type()) {
		if argumentParameter != nil {
			legacyMessage := typeMismatch(expectedType, checked.Type())
			c.addIncorrectArgumentType(legacyMessage, expectedType, checked.Type(), expr.GetLocation(), *argumentParameter, false)
		} else if expectation != nil {
			c.addDiagnostic(typeMismatchDiagnostic{
				Expected:    expectedType,
				Actual:      checked.Type(),
				ActualSpan:  c.sourceSpan(expr.GetLocation()),
				Expectation: expectation,
			}.build())
		} else {
			c.addTypeMismatch(expectedType, checked.Type(), expr.GetLocation())
		}
		return nil
	}

	return checked
}

// resolveParameterType resolves a parameter's declared type. Mutability is
// type syntax (`name: mut T`); an outermost `mut` normalizes into the
// internal flag representation:
//
//   - natives and descriptor types (lists, maps, channels, named Go
//     slices/maps) carry their base type with the Mutable flag — content
//     mutation flows through the value (ADR 0040);
//   - other foreign types (structs, scalars) carry their pointer form so
//     type identity lines up with `mut pkg::T` values and Go signatures.
func (c *Checker) resolveParameterType(t parse.DeclaredType) (Type, bool) {
	var inner parse.DeclaredType
	switch mt := t.(type) {
	case *parse.MutableType:
		inner = mt.Inner
	case parse.MutableType:
		inner = mt.Inner
	default:
		return c.resolveType(t), false
	}

	base := c.resolveType(inner)
	if base == nil {
		return nil, true
	}
	if foreign, ok := base.(*ForeignType); ok && !foreign.Pointer {
		if mutableParamNeedsGoPointer(foreign) {
			if pointer := foreign.PointerForm(); pointer != nil {
				return pointer, true
			}
		}
	}
	return derefMutableRef(base), true
}

func (c *Checker) resolveParametersWithContext(params []parse.Parameter, expectedFnType *FunctionDef) []Parameter {
	result := make([]Parameter, len(params))
	for i, param := range params {
		var paramType Type = Void
		mutable := false

		if param.Type != nil {
			// Explicit type provided
			paramType, mutable = c.resolveParameterType(param.Type)
		} else if expectedFnType != nil && i < len(expectedFnType.Parameters) {
			// Infer from expected function type
			paramType = expectedFnType.Parameters[i].Type
			mutable = expectedFnType.Parameters[i].Mutable
		}
		// Otherwise defaults to Void

		result[i] = Parameter{
			Name:       param.Name,
			Type:       paramType,
			Mutable:    mutable,
			Loc:        param.GetLocation(),
			declaredAt: c.sourceSpan(param.GetLocation()),
		}
	}
	return result
}

// resolveReturnTypeWithContext resolves return type, optionally inferring from an expected function type
func (c *Checker) resolveReturnTypeWithContext(returnTypeNode parse.DeclaredType, expectedFnType *FunctionDef) Type {
	if returnTypeNode != nil {
		// Explicit type provided
		return c.resolveType(returnTypeNode)
	} else if expectedFnType != nil {
		// Infer from expected function type
		return expectedFnType.ReturnType
	}
	// Default to Void
	return Void
}

// checkFunctionBody validates the function body and returns the checked block
func (c *Checker) checkFunctionBody(fn *FunctionDef, bodyStmts []parse.Statement, params []Parameter, returnType Type, location parse.Location) *Block {
	// Add function to scope BEFORE checking body
	c.scope.add(fn.Name, fn, false)

	previousDiscard := c.discardExprContext
	c.discardExprContext = false
	defer func() {
		c.discardExprContext = previousDiscard
	}()

	// Check function body
	c.pushFunctionGenericContext(fn)
	body := c.checkBlockWithExpected(bodyStmts, func() {
		// Set the expected return type to the scope
		c.scope.expectReturn(returnType)
		// Add parameters to scope
		for _, param := range params {
			c.recordBinding(param.Loc, c.scope.add(param.Name, param.Type, param.Mutable))
		}
	}, returnType, true)
	c.popFunctionGenericContext()

	// Check that the function's return type matches its body's type
	if returnType != Void && !c.areCompatible(returnType, body.Type()) {
		c.addBodyReturnMismatch(bodyStmts, returnType, body.Type(), location, nil)
	}

	return body
}

func bodyResultLocation(bodyStmts []parse.Statement, fallback parse.Location) parse.Location {
	for i := len(bodyStmts) - 1; i >= 0; i-- {
		if bodyStmts[i] == nil {
			continue
		}
		if _, ok := bodyStmts[i].(*parse.Comment); ok {
			continue
		}
		return bodyStmts[i].GetLocation()
	}
	return fallback
}

func (c *Checker) addBodyReturnMismatch(bodyStmts []parse.Statement, expected Type, got Type, fallback parse.Location, returnTypeNode parse.DeclaredType) {
	if bodyReturnMismatch(bodyStmts, expected, got) == "if used as a value must have an else branch" {
		c.addDiagnostic(nonExhaustiveValueIfDiagnostic{
			IfSpan: c.sourceSpan(bodyResultLocation(bodyStmts, fallback)),
		}.build())
		return
	}
	var expectation *typeExpectation
	if returnTypeNode != nil {
		expectation = &typeExpectation{
			Span: c.sourceSpan(returnTypeNode.GetLocation()),
			Kind: expectationReturnAnnotation,
		}
	}
	c.addDiagnostic(typeMismatchDiagnostic{
		Expected:    expected,
		Actual:      got,
		ActualSpan:  c.sourceSpan(bodyResultLocation(bodyStmts, fallback)),
		Expectation: expectation,
	}.build())
}

// bodyReturnMismatch picks the diagnostic for a body whose type does not
// match the declared return type. When the body ends in an if chain without
// an else, the real problem is exhaustiveness (issue #267), so the message
// teaches that rule instead of reporting the Void it implies.
func bodyReturnMismatch(bodyStmts []parse.Statement, expected Type, got Type) string {
	for i := len(bodyStmts) - 1; i >= 0; i-- {
		if _, ok := bodyStmts[i].(*parse.Comment); ok {
			continue
		}
		if chain, ok := bodyStmts[i].(*parse.IfStatement); ok && !ifChainHasElse(chain) && got == Void {
			return "if used as a value must have an else branch"
		}
		break
	}
	return typeMismatch(expected, got)
}

func (c *Checker) resolveMethodSignature(def *parse.FunctionDeclaration) *FunctionDef {
	params := c.resolveParametersWithContext(def.Parameters, nil)
	returnType := c.resolveReturnTypeWithContext(def.ReturnType, nil)
	for i, param := range def.Parameters {
		if param.Type != nil && params[i].Type == nil {
			panic(fmt.Errorf("Cannot resolve type for parameter %s", param.Name))
		}
	}
	return &FunctionDef{
		Name:          def.Name,
		GenericParams: append([]string(nil), def.TypeParams...),
		Parameters:    params,
		ReturnType:    returnType,
		Private:       def.Private,
		IsTest:        def.IsTest,
	}
}

func (c *Checker) checkFunction(def *parse.FunctionDeclaration, init func(), extraGenericParams ...string) *FunctionDef {
	return c.checkFunctionWithSignature(def, init, nil, extraGenericParams...)
}

func (c *Checker) checkFunctionWithSignature(def *parse.FunctionDeclaration, init func(), signature *FunctionDef, extraGenericParams ...string) *FunctionDef {
	if init != nil {
		init()
	}
	if c.spans != nil && init == nil {
		// Module-level function definition. Methods (init != nil) are keyed
		// separately when method identity recording lands.
		c.recordDef(def.GetLocation(), FunctionKey(c.typeOwnerPath(), def.Name))
	}

	// Reuse the hoisted signature when this is a top-level declaration whose
	// signature was pre-resolved for forward references. This keeps earlier
	// call sites pointing at the same definition instance and avoids duplicate
	// signature diagnostics.
	var fn *FunctionDef
	var params []Parameter
	var returnType Type
	if signature != nil {
		fn = signature
		params = fn.Parameters
		returnType = fn.ReturnType
	} else if hoisted, ok := c.hoistedTopLevelFunctions[def]; ok && init == nil {
		fn = hoisted
		params = fn.Parameters
		returnType = fn.ReturnType
	} else {
		// Resolve parameters and return type
		params = c.resolveParametersWithContext(def.Parameters, nil)
		returnType = c.resolveReturnTypeWithContext(def.ReturnType, nil)

		// Validate parameters resolved correctly (for named functions, types must be explicit)
		for i, param := range def.Parameters {
			if param.Type != nil && params[i].Type == nil {
				panic(fmt.Errorf("Cannot resolve type for parameter %s", param.Name))
			}
		}

		// Create function definition
		fn = &FunctionDef{
			Name:          def.Name,
			GenericParams: append([]string(nil), def.TypeParams...),
			Parameters:    params,
			ReturnType:    returnType,
			Body:          nil,
			Private:       def.Private,
			IsTest:        def.IsTest,
		}
	}

	if def.IsTest {
		if init != nil {
			c.addDiagnostic(invalidTestFunctionDiagnostic{
				Kind: testNotTopLevel,
				Span: c.sourceSpan(def.GetLocation()),
			}.build())
		}
		if len(def.Parameters) > 0 {
			c.addDiagnostic(invalidTestFunctionDiagnostic{
				Kind: testParametersNotAllowed,
				Span: c.sourceSpan(def.Parameters[0].GetLocation()),
			}.build())
		}
		if len(def.TypeParams) > 0 {
			c.addDiagnostic(invalidTestFunctionDiagnostic{
				Kind: genericTestNotAllowed,
				Span: c.sourceSpan(def.GetLocation()),
			}.build())
		}
		expectedReturnType := MakeResult(Void, Str)
		if !returnType.equal(expectedReturnType) {
			location := def.GetLocation()
			if def.ReturnType != nil {
				location = def.ReturnType.GetLocation()
			}
			c.addDiagnostic(invalidTestFunctionDiagnostic{
				Kind: invalidTestReturnType,
				Span: c.sourceSpan(location),
			}.build())
		}
	}

	// Add function to scope before checking body (for recursion support).
	// Hoisted top-level functions are already in scope.
	// For methods (when init != nil), only add within the body scope.
	if init == nil {
		if _, ok := c.hoistedTopLevelFunctions[def]; !ok {
			c.scope.add(def.Name, fn, false)
		}
	}

	c.pushFunctionGenericContext(fn, extraGenericParams...)
	body := c.checkBlockWithExpected(def.Body, func() {
		c.scope.expectReturn(returnType)
		for _, param := range params {
			c.recordBinding(param.Loc, c.scope.add(param.Name, param.Type, param.Mutable))
		}
	}, returnType, true)
	c.popFunctionGenericContext()

	// Validate return type
	if returnType != Void && !c.areCompatible(returnType, body.Type()) {
		c.addBodyReturnMismatch(def.Body, returnType, body.Type(), def.GetLocation(), def.ReturnType)
	}

	fn.Body = body
	return fn
}

// Substitute generic parameters in a type
func substituteType(t Type, typeMap map[string]Type) Type {
	switch typ := t.(type) {
	case *TypeVar:
		if concrete, exists := typeMap[typ.name]; exists {
			return concrete
			// typ.actual = concrete
		}
		return typ
	case *Maybe:
		return &Maybe{of: substituteType(typ.of, typeMap)}
	case *Result:
		return MakeResult(
			substituteType(typ.val, typeMap),
			substituteType(typ.err, typeMap),
		)
	case *List:
		return &List{of: substituteType(typ.of, typeMap)}
	case *Chan:
		return &Chan{of: substituteType(typ.of, typeMap)}
	case *Receiver:
		return &Receiver{of: substituteType(typ.of, typeMap)}
	case *Sender:
		return &Sender{of: substituteType(typ.of, typeMap)}
	case *Map:
		return &Map{key: substituteType(typ.key, typeMap), value: substituteType(typ.value, typeMap)}
	case *Union:
		types := make([]Type, len(typ.Types))
		for i, member := range typ.Types {
			types[i] = substituteType(member, typeMap)
		}
		return &Union{Name: typ.Name, ModulePath: typ.ModulePath, Types: types, Private: typ.Private}
	case *StructDef:
		var out Type = typ
		for genericName, concrete := range typeMap {
			out = replaceGeneric(out, genericName, concrete)
		}
		return out
	case *FunctionDef:
		// Substitute generics in function parameters and return type
		substitutedParams := make([]Parameter, len(typ.Parameters))
		for i, param := range typ.Parameters {
			substitutedParams[i] = param
			substitutedParams[i].Type = substituteType(param.Type, typeMap)
		}
		return &FunctionDef{
			Name:                    typ.Name,
			GenericParams:           append([]string(nil), typ.GenericParams...),
			Parameters:              substitutedParams,
			ReturnType:              substituteType(typ.ReturnType, typeMap),
			InferReturnTypeFromBody: typ.InferReturnTypeFromBody,
			Body:                    typ.Body,
			Mutates:                 typ.Mutates,
			IsTest:                  typ.IsTest,
			Private:                 typ.Private,
			GenericBindings:         cloneTypeMap(typ.GenericBindings),
		}
	// Handle other compound types
	default:
		return t
	}
}

func cloneTypeMap(in map[string]Type) map[string]Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]Type, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func concreteTypeVarBindings(in map[string]*TypeVar) map[string]Type {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]Type, len(in))
	for key, value := range in {
		if value != nil && value.Actual() != nil {
			out[key] = value.Actual()
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func inferGenericBindingsFromFunction(original, specialized *FunctionDef) map[string]Type {
	if original == nil || specialized == nil {
		return nil
	}
	bindings := map[string]Type{}
	for i, param := range original.Parameters {
		if i < len(specialized.Parameters) {
			inferGenericBindingsFromTypes(param.Type, specialized.Parameters[i].Type, bindings)
		}
	}
	inferGenericBindingsFromTypes(original.ReturnType, specialized.ReturnType, bindings)
	if len(bindings) == 0 {
		return nil
	}
	return bindings
}

func inferGenericBindingsFromTypes(original, specialized Type, bindings map[string]Type) {
	specialized = derefType(specialized)
	switch orig := original.(type) {
	case *TypeVar:
		if specialized != nil && !hasGenericsInType(specialized) {
			bindings[orig.Name()] = specialized
		}
	case *Maybe:
		if spec, ok := specialized.(*Maybe); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Result:
		if spec, ok := specialized.(*Result); ok {
			inferGenericBindingsFromTypes(orig.Val(), spec.Val(), bindings)
			inferGenericBindingsFromTypes(orig.Err(), spec.Err(), bindings)
		}
	case *Chan:
		if spec, ok := specialized.(*Chan); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Receiver:
		if spec, ok := specialized.(*Receiver); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Sender:
		if spec, ok := specialized.(*Sender); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *List:
		if spec, ok := specialized.(*List); ok {
			inferGenericBindingsFromTypes(orig.Of(), spec.Of(), bindings)
		}
	case *Map:
		if spec, ok := specialized.(*Map); ok {
			inferGenericBindingsFromTypes(orig.Key(), spec.Key(), bindings)
			inferGenericBindingsFromTypes(orig.Value(), spec.Value(), bindings)
		}
	case *StructDef:
		if spec, ok := specialized.(*StructDef); ok && orig.Name == spec.Name && !namedTypeOwnersDiffer(orig.ModulePath, spec.ModulePath) {
			for i, typeArg := range orig.TypeArgs {
				if i < len(spec.TypeArgs) {
					inferGenericBindingsFromTypes(typeArg, spec.TypeArgs[i], bindings)
				}
			}
			for fieldName, fieldType := range orig.Fields {
				if specializedField, ok := spec.Fields[fieldName]; ok {
					inferGenericBindingsFromTypes(fieldType, specializedField, bindings)
				}
			}
		}
	case *FunctionDef:
		if spec, ok := specialized.(*FunctionDef); ok {
			for i, param := range orig.Parameters {
				if i < len(spec.Parameters) {
					inferGenericBindingsFromTypes(param.Type, spec.Parameters[i].Type, bindings)
				}
			}
			inferGenericBindingsFromTypes(orig.ReturnType, spec.ReturnType, bindings)
		}
	}
}

// setupFunctionGenerics sets up generic scope and function copy for generic functions.
// Returns the function copy (with fresh TypeVar instances for generics) and the generic scope.
// For non-generic functions, returns the original function and a nil scope.
func (c *Checker) setupFunctionGenerics(fnDef *FunctionDef) (*FunctionDef, *SymbolTable) {
	if !fnDef.hasGenerics() {
		return fnDef, nil
	}

	genericParams := genericParamsForFunction(fnDef)

	// Create generic scope and fresh function copy
	genericScope := c.scope.createGenericScope(genericParams)
	fnDefCopy := copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)

	return fnDefCopy, genericScope
}

func (c *Checker) checkMaybeNewStatic(s *parse.StaticFunction, mod Module) Expression {
	callTypeArgs := c.resolveCallTypeArgs(s.Function.TypeArgs)
	if len(callTypeArgs) > 1 {
		c.addInvalidFunctionTypeArguments("Maybe::new", 1, len(callTypeArgs), true, s.GetLocation(), "Maybe::new accepts at most one explicit type argument")
		return nil
	}
	if len(s.Function.Args) > 1 {
		c.addArgumentCount("0 or 1", len(s.Function.Args), s.GetLocation(), "")
		return nil
	}
	typeVar := Type(&TypeVar{name: "T"})
	if len(callTypeArgs) == 1 {
		typeVar = callTypeArgs[0]
	}
	var maybeType Type = MakeMaybe(typeVar)
	if len(s.Function.Args) == 0 {
		call := &FunctionCall{
			Name:     "none",
			Args:     []Expression{},
			TypeArgs: callTypeArgs,
			fn: &FunctionDef{
				Name:          "new",
				GenericParams: []string{"T"},
				Parameters:    []Parameter{},
				ReturnType:    maybeType,
			},
			ReturnType: maybeType,
		}
		c.recordTarget(s, call, SpanTarget{Kind: TargetFunction, Module: mod.Path(), Symbol: "new"})
		return &ModuleFunctionCall{Module: mod.Path(), Call: call}
	}
	arg := s.Function.Args[0]
	if arg.Name != "" && arg.Name != "value" {
		c.addUnknownNamedArgument(arg.Name, arg.GetLocation(), "unknown argument: "+arg.Name)
		return nil
	}
	value := c.checkExpr(arg.Value)
	if value == nil {
		return nil
	}
	if valueMaybe, ok := value.Type().(*Maybe); ok {
		if len(callTypeArgs) == 1 && !c.areCompatible(maybeType, value.Type()) {
			c.addTypeMismatch(maybeType, value.Type(), arg.GetLocation())
			return nil
		}
		maybeType = value.Type()
		call := &FunctionCall{
			Name:     "new",
			Args:     []Expression{value},
			TypeArgs: callTypeArgs,
			fn: &FunctionDef{
				Name:          "new",
				GenericParams: []string{"T"},
				Parameters:    []Parameter{{Name: "value", Type: valueMaybe}},
				ReturnType:    maybeType,
			},
			ReturnType: maybeType,
		}
		c.recordTarget(s, call, SpanTarget{Kind: TargetFunction, Module: mod.Path(), Symbol: "new"})
		return &ModuleFunctionCall{Module: mod.Path(), Call: call}
	}
	if len(callTypeArgs) == 1 && !c.areCompatible(typeVar, value.Type()) {
		value = c.checkExprAs(arg.Value, typeVar)
		if value == nil {
			return nil
		}
		if !c.areCompatible(typeVar, value.Type()) {
			c.addTypeMismatch(typeVar, value.Type(), arg.GetLocation())
			return nil
		}
	}
	maybeType = MakeMaybe(value.Type())
	call := &FunctionCall{
		Name:     "some",
		Args:     []Expression{value},
		TypeArgs: callTypeArgs,
		fn: &FunctionDef{
			Name:          "new",
			GenericParams: []string{"T"},
			Parameters:    []Parameter{{Name: "value", Type: value.Type()}},
			ReturnType:    maybeType,
		},
		ReturnType: maybeType,
	}
	c.recordTarget(s, call, SpanTarget{Kind: TargetFunction, Module: mod.Path(), Symbol: "new"})
	return &ModuleFunctionCall{Module: mod.Path(), Call: call}
}

// synthesizeMaybeNone creates a synthetic Maybe::new() call for an omitted nullable argument.
// This transforms the omitted argument into an explicit function call, allowing backends
// to treat all arguments uniformly without special OmittedArg handling.
func (c *Checker) synthesizeMaybeNone(paramType Type) Expression {
	// paramType should be a Maybe type for omitted arguments
	_, ok := paramType.(*Maybe)
	if !ok {
		// Defensive: if somehow we got here with non-Maybe, return a VoidLiteral
		// (this shouldn't happen due to validation earlier)
		return &VoidLiteral{}
	}

	// Create a module function call: Maybe::new()
	// The return type of Maybe::new() depends on its context, which will be the Maybe type
	return &ModuleFunctionCall{
		Module: "builtin/Maybe",
		Call: &FunctionCall{
			Name: "none",
			Args: []Expression{},
			fn: &FunctionDef{
				Name:       "none",
				Parameters: []Parameter{},
				ReturnType: paramType, // The return type is the Maybe type we're filling in
				Body:       nil,       // No body for synthesized calls
			},
			ReturnType: paramType,
		},
	}
}

// synthesizeMaybeSome wraps a value in Maybe::new() for automatic coercion of T to Maybe<T>.
// This allows calling functions with nullable parameters using unwrapped values:
// instead of add(1, Maybe::new(5)), you can write add(1, 5).
func (c *Checker) synthesizeMaybeSome(value Expression, maybeType Type) Expression {
	return &ModuleFunctionCall{
		Module: "builtin/Maybe",
		Call: &FunctionCall{
			Name: "some",
			Args: []Expression{value},
			fn: &FunctionDef{
				Name: "some",
				Parameters: []Parameter{
					{
						Name:    "value",
						Type:    value.Type(),
						Mutable: false,
					},
				},
				ReturnType: maybeType,
				Body:       nil, // No body for synthesized calls
			},
			ReturnType: maybeType,
		},
	}
}

// checkAndProcessArguments validates and type-checks function arguments with generic support.
// Returns the processed arguments and the specialized function (with generics resolved if applicable).
// Mutable parameters require addressable mutable arguments.
// Synthesizes Maybe::new() calls for omitted nullable arguments.
// If any error occurs, it's added to the checker's diagnostics.

// parameterOmittable reports whether a trailing parameter may be omitted at a
// call site: nullable parameters default to none, and a Go variadic parameter
// may receive zero arguments.
func parameterOmittable(param Parameter) bool {
	if param.Variadic {
		return true
	}
	_, isMaybe := param.Type.(*Maybe)
	return isMaybe
}

func variadicExpectedArgumentCount(fnDef *FunctionDef) int {
	if fnDef != nil && len(fnDef.Parameters) > 0 && fnDef.Parameters[len(fnDef.Parameters)-1].Variadic {
		return len(fnDef.Parameters) - 1
	}
	if fnDef == nil {
		return 0
	}
	return len(fnDef.Parameters)
}

func expandFunctionDefForRepeatedVariadic(fnDef *FunctionDef, argCount int) *FunctionDef {
	if fnDef == nil || len(fnDef.Parameters) == 0 || !fnDef.Parameters[len(fnDef.Parameters)-1].Variadic || argCount <= len(fnDef.Parameters) {
		return fnDef
	}
	expanded := *fnDef
	expanded.Parameters = append([]Parameter(nil), fnDef.Parameters...)
	last := expanded.Parameters[len(expanded.Parameters)-1]
	for len(expanded.Parameters) < argCount {
		expanded.Parameters = append(expanded.Parameters, last)
	}
	return &expanded
}

func (c *Checker) checkAndProcessArguments(fnDef *FunctionDef, resolvedExprs []parse.Expression, fnDefCopy *FunctionDef, genericScope *SymbolTable, numOmittedArgs int) ([]Expression, *FunctionDef) {
	// Create the full argument list including synthesized Maybe::new() calls for omitted arguments
	// Need to maintain parameter order, so use indexed assignment instead of appending
	totalArgs := len(fnDefCopy.Parameters)
	allExprs := make([]Expression, totalArgs)

	// Process provided arguments
	for i := range resolvedExprs {
		// Skip omitted arguments (nil entries) - handle them separately below
		if resolvedExprs[i] == nil {
			continue
		}

		// Get the expected parameter type from the copy (which has fresh TypeVar instances for generics).
		// For generic functions, dereference to see bound generics from previous arguments.
		// derefType walks the type tree so List($T) becomes List(Int) if $T was bound to Int.
		paramType := fnDefCopy.Parameters[i].Type
		if fnDef.hasGenerics() && genericScope != nil {
			paramType = derefType(paramType)
		}

		// For list and map literals, use checkExprAs to infer type from context.
		// Anonymous functions also use checkExprAs so parameter types are inferred from
		// the (possibly bound) paramType.
		// For nullable parameters, use the inner type since literals can't be Maybe
		expectedType := paramType
		if maybeParam, isMaybe := paramType.(*Maybe); isMaybe {
			_, isLiteralOrFunc := resolvedExprs[i].(*parse.ListLiteral)
			if !isLiteralOrFunc {
				_, isLiteralOrFunc = resolvedExprs[i].(*parse.MapLiteral)
			}
			if !isLiteralOrFunc {
				_, isLiteralOrFunc = resolvedExprs[i].(*parse.AnonymousFunction)
			}
			// For literals and anonymous functions in Maybe parameters, use inner type
			if isLiteralOrFunc {
				expectedType = maybeParam.Of()
			}
		}

		var checkedArg Expression
		c.withValueExprContext(func() {
			switch resolvedExprs[i].(type) {
			case *parse.ListLiteral, *parse.MapLiteral:
				if hasGenericsInType(expectedType) {
					checkedArg = c.checkExpr(resolvedExprs[i])
				} else {
					checkedArg = c.checkExprAsArgument(resolvedExprs[i], expectedType, fnDefCopy.Parameters[i])
				}
			case *parse.AnonymousFunction:
				checkedArg = c.checkExprAsArgument(resolvedExprs[i], expectedType, fnDefCopy.Parameters[i])
			default:
				checkedArg = c.checkExpr(resolvedExprs[i])
			}
		})

		if checkedArg == nil {
			return nil, nil
		}

		// Check if we need to wrap the argument in Maybe::new() for nullable parameters
		// If parameter is Maybe<T> and argument is T, wrap it
		if maybeParam, isMaybe := paramType.(*Maybe); isMaybe {
			if argType := checkedArg.Type(); !argType.equal(paramType) {
				if fnDef.hasGenerics() && genericScope != nil {
					if target, source, ok := discardingFunctionTypes(maybeParam.Of(), checkedArg.Type()); ok {
						for paramIndex := range target.Parameters {
							if err := c.unifyTypes(target.Parameters[paramIndex].Type, source.Parameters[paramIndex].Type, genericScope); err != nil {
								c.addIncorrectArgumentType(err.Error(), paramType, checkedArg.Type(), resolvedExprs[i].GetLocation(), fnDefCopy.Parameters[i], false)
								return nil, nil
							}
						}
					}
				}
				checkedArg = coerceDiscardingFunction(maybeParam.Of(), checkedArg)
				// Check if argument type matches the inner Maybe type
				if c.areCompatible(maybeParam.Of(), checkedArg.Type()) {
					// Wrap non-Maybe value in Maybe::new()
					checkedArg = c.synthesizeMaybeSome(checkedArg, paramType)
				}
			}
		}

		// For generic functions, unify the argument type with the parameter type.
		// unifyTypes uses deref() to see bound generics and calls bindGeneric()
		// to mutate TypeVar instances in-place. This binds generics so that
		// subsequent arguments see bound types.
		if fnDef.hasGenerics() && genericScope != nil {
			discardCoerced := false
			if target, source, ok := discardingFunctionTypes(paramType, checkedArg.Type()); ok {
				for paramIndex := range target.Parameters {
					if err := c.unifyTypes(target.Parameters[paramIndex].Type, source.Parameters[paramIndex].Type, genericScope); err != nil {
						c.addIncorrectArgumentType(err.Error(), paramType, checkedArg.Type(), resolvedExprs[i].GetLocation(), fnDefCopy.Parameters[i], false)
						return nil, nil
					}
				}
				checkedArg = coerceDiscardingFunction(paramType, checkedArg)
				discardCoerced = true
			}
			if !discardCoerced {
				if err := c.unifyTypes(paramType, checkedArg.Type(), genericScope); err != nil {
					c.addIncorrectArgumentType(err.Error(), paramType, checkedArg.Type(), resolvedExprs[i].GetLocation(), fnDefCopy.Parameters[i], false)
					return nil, nil
				}
			}
		} else {
			checkedArg = coerceDiscardingFunction(paramType, checkedArg)
			// For non-generic functions, do regular type compatibility check
			if !c.areCompatible(paramType, checkedArg.Type()) {
				upcast, ok := c.foreignInterfaceArgUpcast(paramType, checkedArg)
				if !ok {
					legacyMessage := typeMismatch(paramType, checkedArg.Type())
					c.addIncorrectArgumentType(legacyMessage, paramType, checkedArg.Type(), resolvedExprs[i].GetLocation(), fnDefCopy.Parameters[i], false)
					return nil, nil
				}
				checkedArg = upcast
			}
		}

		// Check mutable-reference constraints if needed. A mutable parameter
		// requires an addressable mutable place; a call-site `mut` marker no longer
		// requests a defensive copy.
		if fnDefCopy.Parameters[i].Mutable {
			if (!c.isMutable(checkedArg) && !freshContainerSatisfiesMutable(paramType, checkedArg)) || foreignScalarNarrows(paramType, checkedArg.Type()) || foreignScalarWidens(paramType, checkedArg.Type()) {
				legacyMessage := fmt.Sprintf("Type mismatch: Expected a mutable %s", fnDefCopy.Parameters[i].Type.String())
				c.addIncorrectArgumentType(legacyMessage, fnDefCopy.Parameters[i].Type, checkedArg.Type(), resolvedExprs[i].GetLocation(), fnDefCopy.Parameters[i], true)
				return nil, nil
			}
			allExprs[i] = checkedArg
		} else {
			allExprs[i] = checkedArg
		}
	}

	// An omitted trailing Go variadic argument stays omitted: the generated
	// call simply passes nothing for the variadic tail.
	if len(allExprs) > 0 && allExprs[len(allExprs)-1] == nil && fnDefCopy.Parameters[len(allExprs)-1].Variadic {
		allExprs = allExprs[:len(allExprs)-1]
	}

	// Fill in synthesized Maybe::new() calls for omitted arguments
	for i := range allExprs {
		if allExprs[i] == nil {
			paramType := fnDefCopy.Parameters[i].Type
			if fnDef.hasGenerics() && genericScope != nil {
				paramType = derefType(paramType)
			}
			allExprs[i] = c.synthesizeMaybeNone(paramType)
		}
	}

	// Finalize generic resolution by creating the specialized function
	var fnToUse *FunctionDef
	if genericScope != nil {
		bindings := genericScope.getGenericBindings()

		if len(bindings) == 0 {
			// No generics were bound from arguments. A receiver specialization
			// may still have pre-bound generics, as with Box<T>.get().
			if len(fnDefCopy.GenericBindings) == 0 {
				fnDefCopy.GenericBindings = inferGenericBindingsFromFunction(fnDef, fnDefCopy)
			}
			if len(fnDefCopy.GenericBindings) > 0 {
				fnToUse = fnDefCopy
			} else {
				fnToUse = fnDef
			}
		} else {
			// Create specialized function with resolved generics
			fnToUse = &FunctionDef{
				Name:                    fnDefCopy.Name,
				GenericParams:           append([]string(nil), fnDefCopy.GenericParams...),
				Parameters:              make([]Parameter, len(fnDefCopy.Parameters)),
				ReturnType:              substituteType(fnDefCopy.ReturnType, bindings),
				InferReturnTypeFromBody: fnDefCopy.InferReturnTypeFromBody,
				Body:                    fnDefCopy.Body,
				Mutates:                 fnDefCopy.Mutates,
				Private:                 fnDefCopy.Private,
				GenericBindings:         cloneTypeMap(bindings),
			}

			// Replace generics in parameters
			for i, param := range fnDefCopy.Parameters {
				fnToUse.Parameters[i] = Parameter{
					Name:       param.Name,
					Type:       substituteType(param.Type, bindings),
					Mutable:    param.Mutable,
					Loc:        param.Loc,
					declaredAt: param.declaredAt,
					Variadic:   param.Variadic,
				}
			}
		}
	} else {
		fnToUse = fnDef
	}

	if fnToUse != nil {
		if derefFn, ok := derefType(fnToUse).(*FunctionDef); ok {
			fnToUse = derefFn
		}
	}

	return allExprs, fnToUse
}

type functionTypeArgumentError struct {
	Name          string
	Expected      int
	Actual        int
	TakesTypeArgs bool
	Message       string
}

func (e *functionTypeArgumentError) Error() string { return e.Message }

func (c *Checker) addGenericFunctionResolutionError(err error, location parse.Location) {
	if typeArgErr, ok := err.(*functionTypeArgumentError); ok {
		c.addInvalidFunctionTypeArguments(typeArgErr.Name, typeArgErr.Expected, typeArgErr.Actual, typeArgErr.TakesTypeArgs, location, typeArgErr.Message)
		return
	}
	c.addError(err.Error(), location)
}

// New generic resolution using the enhanced symbol table
func (c *Checker) resolveGenericFunction(fnDef *FunctionDef, args []Expression, typeArgs []Type, _ parse.Location) (*FunctionDef, error) {
	genericParams := genericParamsForFunction(fnDef)
	if !fnDef.hasGenerics() || len(genericParams) == 0 {
		if len(typeArgs) > 0 {
			message := fmt.Sprintf("function %s does not take type arguments", fnDef.Name)
			return nil, &functionTypeArgumentError{Name: fnDef.Name, Actual: len(typeArgs), Message: message}
		}
		return fnDef, nil
	}

	// Create a call-site-specific generic context scope
	genericScope := c.scope.createGenericScope(genericParams)

	// Create a call-site-specific copy of the function with fresh TypeVar instances
	// This copy is isolated to this call and its TypeVar instances will be mutated during unification
	fnDefCopy := copyFunctionWithTypeVarMap(fnDef, *genericScope.genericContext)

	// Handle explicit type arguments
	if len(typeArgs) > 0 {
		if len(typeArgs) != len(genericParams) {
			message := fmt.Sprintf("Expected %d type arguments, got %d", len(genericParams), len(typeArgs))
			return nil, &functionTypeArgumentError{Name: fnDef.Name, Expected: len(genericParams), Actual: len(typeArgs), TakesTypeArgs: true, Message: message}
		}

		for i, actual := range typeArgs {
			if actual == nil {
				return nil, &functionTypeArgumentError{Name: fnDef.Name, Expected: len(genericParams), Actual: len(typeArgs), TakesTypeArgs: true, Message: "could not resolve type argument"}
			}

			if err := genericScope.bindGeneric(genericParams[i], actual); err != nil {
				return nil, err
			}
		}
	}

	// Infer types from arguments using the copied function definition
	// The fresh TypeVar instances in fnDefCopy will be mutated as generics are bound
	for i, param := range fnDefCopy.Parameters {
		if err := c.unifyTypes(param.Type, args[i].Type(), genericScope); err != nil {
			return nil, err
		}
	}

	// Allow unresolved generics - they will be resolved through context
	// (e.g., variable assignment, return type inference)
	// Don't require all generics to be resolved at function call time

	// Get bindings to determine if we need to create a specialized version
	bindings := genericScope.getGenericBindings()

	// If no generics were bound, return the original function
	if len(bindings) == 0 {
		return fnDef, nil
	}

	// The function copy already has fresh TypeVar instances that have been bound.
	// We now need to substitute the bindings to create the final specialized function.
	specialized := &FunctionDef{
		Name:                    fnDefCopy.Name,
		GenericParams:           append([]string(nil), fnDefCopy.GenericParams...),
		Parameters:              make([]Parameter, len(fnDefCopy.Parameters)),
		ReturnType:              substituteType(fnDefCopy.ReturnType, bindings),
		InferReturnTypeFromBody: fnDefCopy.InferReturnTypeFromBody,
		Body:                    fnDefCopy.Body,
		Mutates:                 fnDefCopy.Mutates,
		Private:                 fnDefCopy.Private,
		GenericBindings:         cloneTypeMap(bindings),
	}

	// Replace generics in parameters
	for i, param := range fnDefCopy.Parameters {
		specialized.Parameters[i] = param
		specialized.Parameters[i].Type = substituteType(param.Type, bindings)
	}

	return specialized, nil
}

// unifyTypes performs type unification for generic function arguments.
// It recursively walks the type tree, binding generics to concrete types.
// When expected is a TypeVar (generic parameter), bindGeneric mutates the TypeVar in-place
// with the concrete type, making the binding immediately visible to all callers.
// This enables single-pass argument checking where later arguments see bindings from earlier ones.
func (c *Checker) unifyTypes(expected Type, actual Type, genericScope *SymbolTable) error {
	expected = deref(expected)
	actual = deref(actual)

	switch expectedType := expected.(type) {
	case *TypeVar:
		// Generic type - bind it to the actual type using in-place mutation.
		// This mutates expectedType.bound and expectedType.actual directly.
		return genericScope.bindGeneric(expectedType.name, actual)
	case *FunctionDef:
		// Function type unification
		var actualParams []Parameter
		var actualReturnType Type

		if actualFn, ok := actual.(*FunctionDef); ok {
			actualParams = actualFn.Parameters
			actualReturnType = actualFn.ReturnType
		} else {
			return fmt.Errorf("expected function, got %s", actual.String())
		}

		// Check parameter count
		if len(expectedType.Parameters) != len(actualParams) {
			return fmt.Errorf("parameter count mismatch")
		}

		// Unify parameters
		for i, expectedParam := range expectedType.Parameters {
			if err := c.unifyTypes(expectedParam.Type, actualParams[i].Type, genericScope); err != nil {
				return err
			}
		}

		// Unify return types
		return c.unifyTypes(expectedType.ReturnType, actualReturnType, genericScope)
	case *Result:
		if actualResult, ok := actual.(*Result); ok {
			if err := c.unifyTypes(expectedType.val, actualResult.val, genericScope); err != nil {
				return err
			}
			return c.unifyTypes(expectedType.err, actualResult.err, genericScope)
		}
		return fmt.Errorf("expected result type, got %T", actual)
	case *Maybe:
		if actualMaybe, ok := actual.(*Maybe); ok {
			return c.unifyTypes(expectedType.of, actualMaybe.of, genericScope)
		}
		return fmt.Errorf("expected maybe type, got %T", actual)
	case *List:
		if actualList, ok := actual.(*List); ok {
			return c.unifyTypes(expectedType.of, actualList.of, genericScope)
		}
		return fmt.Errorf("expected list type, got %T", actual)
	case *Chan:
		if actualChannel, ok := actual.(*Chan); ok {
			return c.unifyTypes(expectedType.of, actualChannel.of, genericScope)
		}
		return fmt.Errorf("expected channel type, got %T", actual)
	case *Receiver:
		if act, ok := actual.(*Receiver); ok {
			return c.unifyTypes(expectedType.of, act.of, genericScope)
		}
		return fmt.Errorf("expected receiver type, got %T", actual)
	case *Sender:
		if act, ok := actual.(*Sender); ok {
			return c.unifyTypes(expectedType.of, act.of, genericScope)
		}
		return fmt.Errorf("expected sender type, got %T", actual)
	case *StructDef:
		actualStruct, ok := actual.(*StructDef)
		if !ok || expectedType.Name != actualStruct.Name || namedTypeOwnersDiffer(expectedType.ModulePath, actualStruct.ModulePath) || len(expectedType.TypeArgs) != len(actualStruct.TypeArgs) {
			return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
		}
		for i := range expectedType.TypeArgs {
			if err := c.unifyTypes(expectedType.TypeArgs[i], actualStruct.TypeArgs[i], genericScope); err != nil {
				return err
			}
		}
		for fieldName, expectedField := range expectedType.Fields {
			actualField, ok := actualStruct.Fields[fieldName]
			if !ok {
				return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
			}
			if err := c.unifyTypes(expectedField, actualField, genericScope); err != nil {
				return err
			}
		}
		return nil
	default:
		// Concrete types - must match exactly
		if !expected.equal(actual) {
			return fmt.Errorf("type mismatch: expected %s, got %s", expected.String(), actual.String())
		}
		return nil
	}
}

type argumentBindingError struct {
	Kind      argumentBindingDiagnosticKind
	Name      string
	Location  parse.Location
	Previous  *parse.Location
	Parameter *Parameter
}

func (c *Checker) addArgumentBindingError(err *argumentBindingError, fallback parse.Location) {
	if err.Parameter != nil {
		c.addDiagnostic(missingArgumentDiagnostic{Parameter: *err.Parameter, Span: c.sourceSpan(fallback)}.build())
		return
	}
	location := err.Location
	if location == (parse.Location{}) {
		location = fallback
	}
	var previous *SourceSpan
	if err.Previous != nil {
		span := c.sourceSpan(*err.Previous)
		previous = &span
	}
	c.addDiagnostic(argumentBindingDiagnostic{
		Kind:         err.Kind,
		Name:         err.Name,
		Span:         c.sourceSpan(location),
		PreviousSpan: previous,
	}.build())
}

// resolveArguments converts unified argument list to positional arguments.
func (c *Checker) resolveArguments(args []parse.Argument, params []Parameter) ([]parse.Expression, *argumentBindingError) {
	var positionalArgs []parse.Argument
	var namedArgs []parse.Argument
	for _, arg := range args {
		if arg.Name == "" {
			positionalArgs = append(positionalArgs, arg)
		} else {
			namedArgs = append(namedArgs, arg)
		}
	}

	if len(namedArgs) == 0 {
		result := make([]parse.Expression, len(positionalArgs))
		for i := range positionalArgs {
			result[i] = positionalArgs[i].Value
		}
		return result, nil
	}

	paramMap := make(map[string]int)
	for i, param := range params {
		paramMap[param.Name] = i
	}
	result := make([]parse.Expression, len(params))
	used := make([]bool, len(params))
	usedAt := make([]parse.Location, len(params))

	for i, arg := range positionalArgs {
		if i >= len(params) {
			return nil, &argumentBindingError{Kind: tooManyPositionalArguments, Location: arg.GetLocation()}
		}
		result[i] = arg.Value
		used[i] = true
		usedAt[i] = arg.GetLocation()
	}

	for _, namedArg := range namedArgs {
		paramIndex, exists := paramMap[namedArg.Name]
		if !exists {
			return nil, &argumentBindingError{Kind: unknownNamedArgument, Name: namedArg.Name, Location: namedArg.GetLocation()}
		}
		if used[paramIndex] {
			previous := usedAt[paramIndex]
			return nil, &argumentBindingError{Kind: duplicateArgument, Name: namedArg.Name, Location: namedArg.GetLocation(), Previous: &previous}
		}
		result[paramIndex] = namedArg.Value
		used[paramIndex] = true
		usedAt[paramIndex] = namedArg.GetLocation()
	}

	for i, param := range params {
		if !used[i] && !parameterOmittable(param) {
			parameter := param
			return nil, &argumentBindingError{Name: param.Name, Parameter: &parameter}
		}
	}
	return result, nil
}

// buildComparison builds a comparison expression node from two operands and an operator
// It returns the appropriate typed comparison node (IntGreater, FloatLess, etc.)
func (c *Checker) buildComparison(leftExpr parse.Expression, op parse.Operator, rightExpr parse.Expression) Expression {
	left := c.checkExpr(leftExpr)
	right := c.checkExpr(rightExpr)
	if left == nil || right == nil {
		return nil
	}

	// Allow Enum vs Int comparisons
	operator := comparisonOperatorText(op)
	if !c.areTypesComparable(left.Type(), right.Type()) {
		c.addInvalidRelational(operator, left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), "Cannot compare different types")
		return nil
	}

	// Build the appropriate comparison node based on operator and type
	switch op {
	case parse.GreaterThan:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntGreater{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatGreater{left, right}
		}
		c.addInvalidRelational(">", left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), "The '>' operator can only be used for Int or Float64")
		return nil

	case parse.GreaterThanOrEqual:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntGreaterEqual{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatGreaterEqual{left, right}
		}
		c.addInvalidRelational(">=", left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), "The '>=' operator can only be used for Int or Float64")
		return nil

	case parse.LessThan:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntLess{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatLess{left, right}
		}
		c.addInvalidRelational("<", left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), "The '<' operator can only be used for Int or Float64")
		return nil

	case parse.LessThanOrEqual:
		if isRelationalIntegerLike(left.Type()) || c.isEnum(left.Type()) {
			return &IntLessEqual{left, right}
		}
		if isRelationalFloatLike(left.Type()) {
			return &FloatLessEqual{left, right}
		}
		c.addInvalidRelational("<=", left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), "The '<=' operator can only be used for Int or Float64")
		return nil

	default:
		legacy := fmt.Sprintf("Unsupported operator in comparison: %v", op)
		c.addInvalidRelational(operator, left, right, leftExpr.GetLocation(), rightExpr.GetLocation(), legacy)
		return nil
	}
}

// tryCheckAccessorChain checks if the expression is a property/method accessor chain with Maybes
// and handles cascading None propagation via OptionMatch expressions
func (c *Checker) tryCheckAccessorChain(parseExpr parse.Expression) Expression {
	// Detect if this is a property/method accessor chain
	if !c.isAccessorChain(parseExpr) {
		// Not an accessor chain, check normally
		return c.checkExpr(parseExpr)
	}

	// Try to build the accessor chain with Maybe handling
	return c.checkAccessorChainWithMaybes(parseExpr)
}

// isAccessorChain checks if an expression is a property or method accessor (possibly chained)
func (c *Checker) isAccessorChain(parseExpr parse.Expression) bool {
	switch parseExpr.(type) {
	case *parse.InstanceProperty, *parse.InstanceMethod:
		return true
	default:
		return false
	}
}

// checkAccessorChainWithMaybes checks an accessor chain and wraps Maybe property accesses in OptionMatch
func (c *Checker) checkAccessorChainWithMaybes(parseExpr parse.Expression) Expression {
	switch p := parseExpr.(type) {
	case *parse.InstanceProperty:
		// First check the target
		target := c.checkAccessorChainWithMaybes(p.Target)
		if target == nil {
			return nil
		}

		// Try to get the property type
		innerType := target.Type()
		var isMaybe bool
		if maybeType, ok := innerType.(*Maybe); ok {
			innerType = maybeType.of
			isMaybe = true
		}

		propType := innerType.get(p.Property.Name)
		if propType == nil {
			c.addDiagnostic(undefinedMemberDiagnostic{
				Kind:     undefinedField,
				Receiver: fmt.Sprint(innerType),
				Member:   p.Property.Name,
				Span:     c.sourceSpan(p.Property.GetLocation()),
			}.build())
			return nil
		}

		prop := &InstanceProperty{
			Subject:  target,
			Property: p.Property.Name,
			_type:    propType,
			Kind:     StructSubject,
		}

		// If the target is Maybe, wrap in OptionMatch
		if isMaybe {
			return c.wrapAccessorInMatch(target, prop, innerType, propType)
		}

		return prop

	case *parse.InstanceMethod:
		// Similar logic for methods
		target := c.checkAccessorChainWithMaybes(p.Target)
		if target == nil {
			return nil
		}

		// Try to get the method signature
		innerType := target.Type()
		var isMaybe bool
		if maybeType, ok := innerType.(*Maybe); ok {
			innerType = maybeType.of
			isMaybe = true
		}

		var sig Type
		if structDef, ok := innerType.(*StructDef); ok {
			if method, ok := c.structMethod(structDef, p.Method.Name); ok {
				sig = method
			}
		} else {
			sig = innerType.get(p.Method.Name)
		}
		if sig == nil {
			// This accessor-chain path only speculatively verifies that a method
			// exists before falling back to normal expression checking below.
			// Pointer-receiver mutability/addressability is enforced by the normal
			// InstanceMethod checker, not here.
			if foreign, ok := innerType.(*ForeignType); ok && !foreign.Pointer {
				pointerForeign := *foreign
				pointerForeign.Pointer = true
				pointerForeign.Methods = nil
				pointerForeign.MethodsLoaded = false
				if pointerSig := pointerForeign.get(p.Method.Name); pointerSig != nil {
					sig = pointerSig
				}
			}
		}
		if sig == nil {
			if !isMaybe {
				if call, ok := c.checkFunctionFieldCall(target, p.Method, p.GetLocation()); ok {
					return call
				}
			}
			c.addDiagnostic(undefinedMemberDiagnostic{
				Kind:     undefinedMethod,
				Receiver: fmt.Sprint(innerType),
				Member:   p.Method.Name,
				Span:     c.sourceSpan(p.Method.GetLocation()),
			}.build())
			return nil
		}

		_, ok := sig.(*FunctionDef)
		if !ok {
			c.addNonCallable(fmt.Sprintf("%s.%s", innerType, p.Method.Name), p.Method.GetLocation(), nil, nonCallableSuffix)
			return nil
		}

		// For now, just check the method normally and return if not Maybe
		// (full method call handling is complex, only handle property accessor chains for now)
		if !isMaybe {
			// Fall back to normal checking for non-Maybe methods
			return c.checkExpr(parseExpr)
		}

		// If the target is Maybe, we'd need to wrap the entire method call
		// For simplicity, just check normally - the user should use property access if they want cascading
		return c.checkExpr(parseExpr)

	default:
		// Not an accessor, check normally
		return c.checkExpr(parseExpr)
	}
}

// wrapAccessorInMatch wraps a property access on a Maybe type in an OptionMatch expression
func (c *Checker) wrapAccessorInMatch(subject Expression, prop *InstanceProperty, innerType Type, propType Type) Expression {
	// Generate a pattern variable name
	patternVar := "_maybe_prop"

	// Create a symbol for the pattern variable
	patternSym := Symbol{
		Name:    patternVar,
		Type:    innerType,
		mutable: false,
	}

	// Create an identifier for the pattern variable with the symbol
	patternIdent := &Identifier{Name: patternVar}
	patternIdent.sym = patternSym

	// The Some block accesses the property on the unwrapped value
	// We create a new InstanceProperty with the pattern variable as subject
	propOnUnwrapped := &InstanceProperty{
		Subject:  patternIdent,
		Property: prop.Property,
		_type:    propType,
		Kind:     StructSubject,
	}

	// Create the Some block containing the property access
	someBlock := &Block{
		Stmts: []Statement{
			{Expr: propOnUnwrapped},
		},
	}

	// The None block just returns the subject (which is None)
	// The subject's type is Maybe<innerType>, so it will propagate as None of type propType
	noneBlock := &Block{
		Stmts: []Statement{
			{Expr: subject},
		},
	}

	// Create and return the OptionMatch
	return &OptionMatch{
		Subject: subject,
		Some: &Match{
			Pattern: patternIdent,
			Body:    someBlock,
		},
		None:      noneBlock,
		InnerType: innerType,
	}
}
