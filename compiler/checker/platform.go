package checker

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/platform"
)

// platformAttributeName is the attribute that selects top-level declarations
// and `use` lines by target platform (ADR 0082).
const platformAttributeName = "platform"

// platformIssue is a problem with a module's #platform attributes or its
// platform-specific siblings. Issues do not depend on the target platform.
type platformIssue struct {
	code     DiagnosticCode
	title    string
	message  string
	location parse.Location
	related  *parse.Location
	note     string
}

// platformMember is a top-level declaration or `use` line that may take part
// in platform selection.
type platformMember struct {
	isImport  bool
	index     int // index into Program.Imports or Program.Statements
	statement parse.Statement
	location  parse.Location
	key       string
	// annotated reports a #platform attribute. names holds its platform
	// names; invalid marks an attribute with errors, whose declaration is
	// never selected.
	annotated bool
	attribute parse.Attribute
	names     []string
	invalid   bool
}

// PlatformAnalysis is a module's platform-specific declarations, grouped by
// the name they bind.
type PlatformAnalysis struct {
	members []platformMember
	// groups lists member indexes by key, for keys with at least one
	// annotated member, in source order.
	groups [][]int
	issues []platformIssue
}

// AnalyzePlatforms reads a module's #platform attributes and checks the
// rules that hold on every platform: valid platform names, at most one
// fallback, no overlapping siblings, and matching sibling signatures. It
// returns nil when the module has no declaration attributes.
func AnalyzePlatforms(program *parse.Program) *PlatformAnalysis {
	if program == nil || !hasDeclarationAttributes(program) {
		return nil
	}
	analysis := &PlatformAnalysis{}
	for index := range program.Imports {
		imp := &program.Imports[index]
		member := platformMember{isImport: true, index: index, location: imp.Location, key: "use " + imp.Name}
		analysis.readAttributes(&member, imp.Attributes)
		analysis.members = append(analysis.members, member)
	}
	for index, statement := range program.Statements {
		key, ok := platformKey(statement)
		if !ok {
			continue
		}
		member := platformMember{index: index, statement: statement, location: statement.GetLocation(), key: key}
		analysis.readAttributes(&member, parse.DeclarationAttributes(statement))
		analysis.members = append(analysis.members, member)
	}

	byKey := map[string][]int{}
	keys := []string{}
	for index, member := range analysis.members {
		if _, seen := byKey[member.key]; !seen {
			keys = append(keys, member.key)
		}
		byKey[member.key] = append(byKey[member.key], index)
	}
	for _, key := range keys {
		group := byKey[key]
		annotated := false
		for _, index := range group {
			if analysis.members[index].annotated {
				annotated = true
				break
			}
		}
		if !annotated {
			continue
		}
		analysis.groups = append(analysis.groups, group)
		analysis.checkGroup(group)
	}
	return analysis
}

func hasDeclarationAttributes(program *parse.Program) bool {
	for _, imp := range program.Imports {
		if len(imp.Attributes) > 0 {
			return true
		}
	}
	for _, statement := range program.Statements {
		if len(parse.DeclarationAttributes(statement)) > 0 {
			return true
		}
	}
	return false
}

// platformKey is the name a top-level declaration binds, which groups
// platform-specific siblings. Values, functions, and types share one
// namespace; impl blocks are identified by their header.
func platformKey(statement parse.Statement) (string, bool) {
	switch node := statement.(type) {
	case *parse.FunctionDeclaration:
		return node.Name, true
	case *parse.StaticFunctionDeclaration:
		return node.Path.String(), true
	case *parse.VariableDeclaration:
		return node.Name, true
	case *parse.TypeDeclaration:
		return node.Name.Name, true
	case *parse.StructDefinition:
		return node.Name.Name, true
	case *parse.EnumDefinition:
		return node.Name, true
	case *parse.TraitDefinition:
		return node.Name.Name, true
	case *parse.ImplBlock:
		return "impl " + node.Target.Name, true
	case *parse.TraitImplementation:
		trait := ""
		if node.Trait != nil {
			trait = node.Trait.String()
		}
		return "impl " + trait + " for " + node.ForType.Name, true
	}
	return "", false
}

func (a *PlatformAnalysis) addIssue(issue platformIssue) {
	a.issues = append(a.issues, issue)
}

func (a *PlatformAnalysis) readAttributes(member *platformMember, attributes []parse.Attribute) {
	for _, attribute := range attributes {
		if attribute.Namespace != nil || attribute.Name.Name != platformAttributeName {
			a.addIssue(platformIssue{
				code:     DiagnosticCodeUnknownAttribute,
				title:    "Unknown attribute",
				message:  "Unknown attribute: " + attributeDisplayName(attribute),
				location: attribute.Name.GetLocation(),
			})
			continue
		}
		if member.annotated {
			first := member.attribute.Name.GetLocation()
			a.addIssue(platformIssue{
				code:     DiagnosticCodeDuplicateAttribute,
				title:    "Duplicate attribute",
				message:  "Duplicate attribute: #platform",
				location: attribute.Name.GetLocation(),
				related:  &first,
				note:     "first #platform attribute",
			})
			continue
		}
		member.annotated = true
		member.attribute = attribute
		if len(attribute.Arguments) == 0 {
			member.invalid = true
			a.addIssue(platformIssue{
				code:     DiagnosticCodeInvalidAttributeArgument,
				title:    "Missing platform",
				message:  "#platform requires at least one platform name, such as unix or linux_amd64",
				location: attribute.GetLocation(),
			})
			continue
		}
		for _, argument := range attribute.Arguments {
			if argument.Name != "" || argument.Value.Kind != parse.AttributeSymbol {
				member.invalid = true
				a.addIssue(platformIssue{
					code:     DiagnosticCodeInvalidAttributeArgument,
					title:    "Invalid platform",
					message:  "#platform arguments must be platform names, such as unix or linux_amd64",
					location: argument.GetLocation(),
				})
				continue
			}
			if err := platform.ValidateName(argument.Value.Text); err != nil {
				member.invalid = true
				a.addIssue(platformIssue{
					code:     DiagnosticCodeUnknownPlatform,
					title:    "Unknown platform",
					message:  err.Error(),
					location: argument.GetLocation(),
				})
				continue
			}
			member.names = append(member.names, argument.Value.Text)
		}
	}
}

func platformAttributeText(names []string) string {
	return "#platform(" + strings.Join(names, ", ") + ")"
}

func (a *PlatformAnalysis) checkGroup(group []int) {
	members := make([]*platformMember, 0, len(group))
	for _, index := range group {
		members = append(members, &a.members[index])
	}

	var fallback *platformMember
	for _, member := range members {
		if member.annotated {
			continue
		}
		if fallback == nil {
			fallback = member
			continue
		}
		first := fallback.location
		a.addIssue(platformIssue{
			code:     DiagnosticCodePlatformSibling,
			title:    "Second fallback declaration",
			message:  fmt.Sprintf("`%s` already has a declaration without #platform; only one declaration can be the fallback for its platform-specific siblings", member.key),
			location: member.location,
			related:  &first,
			note:     "the fallback declaration",
		})
	}

	// Annotated siblings must not select the same platform.
	for i, left := range members {
		if !left.annotated || left.invalid {
			continue
		}
		for _, right := range members[i+1:] {
			if !right.annotated || right.invalid {
				continue
			}
			if overlap, ok := platform.Overlap(left.names, right.names); ok {
				first := left.attribute.GetLocation()
				a.addIssue(platformIssue{
					code:     DiagnosticCodePlatformOverlap,
					title:    "Overlapping platforms",
					message:  fmt.Sprintf("%s overlaps %s on %s", platformAttributeText(right.names), platformAttributeText(left.names), overlap),
					location: right.attribute.GetLocation(),
					related:  &first,
					note:     "overlapping sibling",
				})
			}
		}
	}

	if len(members) < 2 {
		return
	}
	for _, member := range members {
		switch member.statement.(type) {
		case *parse.StructDefinition, *parse.EnumDefinition, *parse.TraitDefinition:
			for _, sibling := range members[1:] {
				first := members[0].location
				a.addIssue(platformIssue{
					code:     DiagnosticCodePlatformSibling,
					title:    "Platform-specific type sibling",
					message:  "Structs, enums, and traits cannot have platform-specific siblings; `" + sibling.key + "` is declared more than once",
					location: sibling.location,
					related:  &first,
					note:     "first declaration",
				})
			}
			return
		}
	}
	if members[0].isImport {
		// `use` siblings share an alias; their import paths may differ.
		return
	}

	reference := members[0]
	if fallback != nil {
		reference = fallback
	}
	untyped := false
	for _, member := range members {
		if variable, ok := member.statement.(*parse.VariableDeclaration); ok && variable.Type == nil {
			untyped = true
			a.addIssue(platformIssue{
				code:     DiagnosticCodePlatformSignatureMismatch,
				title:    "Missing type",
				message:  "Platform-specific `" + variable.Name + "` needs an explicit type, so every platform declares the same type",
				location: member.location,
			})
		}
	}
	if untyped {
		return
	}
	referenceSignature, _ := formatter.DeclarationSignature(reference.statement)
	for _, member := range members {
		if member == reference {
			continue
		}
		signature, _ := formatter.DeclarationSignature(member.statement)
		if signature == referenceSignature {
			continue
		}
		related := reference.location
		message := fmt.Sprintf("Platform-specific declarations must have the same signature: `%s` does not match `%s`", signature, referenceSignature)
		if strings.Contains(signature, "\n") || strings.Contains(referenceSignature, "\n") {
			header, _, _ := strings.Cut(signature, "\n")
			message = fmt.Sprintf("Platform-specific `%s` blocks must declare the same method signatures", header)
		}
		a.addIssue(platformIssue{
			code:     DiagnosticCodePlatformSignatureMismatch,
			title:    "Platform signature mismatch",
			message:  message,
			location: member.location,
			related:  &related,
			note:     "sibling declared here",
		})
	}
}

// active reports which members are selected for target.
func (a *PlatformAnalysis) active(target platform.Platform) []bool {
	active := make([]bool, len(a.members))
	for index := range a.members {
		active[index] = true
	}
	for _, group := range a.groups {
		selected := -1
		for _, index := range group {
			member := a.members[index]
			if member.annotated && !member.invalid && platform.MatchesAny(member.names, target) {
				selected = index
				break
			}
		}
		if selected < 0 {
			for _, index := range group {
				if !a.members[index].annotated {
					selected = index
					break
				}
			}
		}
		for _, index := range group {
			active[index] = index == selected
		}
	}
	return active
}

// Select returns program with only the declarations and `use` lines that are
// active on target. The input program is not modified.
func (a *PlatformAnalysis) Select(program *parse.Program, target platform.Platform) *parse.Program {
	if a == nil {
		return program
	}
	active := a.active(target)
	dropImports := map[int]bool{}
	dropStatements := map[int]bool{}
	for index, member := range a.members {
		if active[index] {
			continue
		}
		if member.isImport {
			dropImports[member.index] = true
		} else {
			dropStatements[member.index] = true
		}
	}
	selected := &parse.Program{
		Imports:    make([]parse.Import, 0, len(program.Imports)),
		Statements: make([]parse.Statement, 0, len(program.Statements)),
	}
	for index, imp := range program.Imports {
		if !dropImports[index] {
			selected.Imports = append(selected.Imports, imp)
		}
	}
	for index, statement := range program.Statements {
		if !dropStatements[index] {
			selected.Statements = append(selected.Statements, statement)
		}
	}
	return selected
}

// SelectionKey identifies which declarations target selects, so platforms
// that select the same declarations can share one check.
func (a *PlatformAnalysis) SelectionKey(target platform.Platform) string {
	if a == nil {
		return ""
	}
	var key strings.Builder
	for _, selected := range a.active(target) {
		if selected {
			key.WriteByte('1')
		} else {
			key.WriteByte('0')
		}
	}
	return key.String()
}

// SelectPlatform returns program with only the declarations and `use` lines
// active on target.
func SelectPlatform(program *parse.Program, target platform.Platform) *parse.Program {
	return AnalyzePlatforms(program).Select(program, target)
}

// targetPlatform is the platform the module is checked for: the explicit
// check option, else the resolver's platform, else the go tool's default.
func (c *Checker) targetPlatform() platform.Platform {
	if !c.options.Platform.IsZero() {
		return c.options.Platform
	}
	if c.moduleResolver != nil {
		return c.moduleResolver.Platform()
	}
	if resolver, ok := c.options.GoResolver.(*GoPackagesResolver); ok && resolver != nil && !resolver.Platform.IsZero() {
		return resolver.Platform
	}
	return platform.Detect(filepath.Dir(c.filePath))
}

func (c *Checker) addPlatformIssues(issues []platformIssue) {
	for _, issue := range issues {
		var secondary []DiagnosticLabel
		if issue.related != nil {
			secondary = append(secondary, DiagnosticLabel{Span: c.sourceSpan(*issue.related), Message: issue.note})
		}
		diagnostic := newLabeledDiagnostic(Error, issue.message, issue.title, issue.message,
			DiagnosticLabel{Span: c.sourceSpan(issue.location), Message: issue.message}, secondary...)
		diagnostic.Code = issue.code
		c.addDiagnostic(diagnostic)
	}
}
