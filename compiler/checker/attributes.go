package checker

import (
	"strings"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
)

func (c *Checker) addAttributeDiagnostic(code DiagnosticCode, message, title string, location parse.Location, secondary ...DiagnosticLabel) {
	diagnostic := newLabeledDiagnostic(
		Error,
		message,
		title,
		message,
		DiagnosticLabel{Span: c.sourceSpan(location), Message: message},
		secondary...,
	)
	diagnostic.Code = code
	c.addDiagnostic(diagnostic)
}

func attributeDisplayName(attribute parse.Attribute) string {
	if attribute.Namespace != nil {
		return "#" + attribute.Namespace.Name + ":" + attribute.Name.Name
	}
	return "#" + attribute.Name.Name
}

func (c *Checker) checkGoFieldTagAttribute(attribute parse.Attribute, seen map[string]parse.Location) (GoFieldTag, bool) {
	name := attributeDisplayName(attribute)
	key := attribute.Name.Name
	if original, duplicate := seen[key]; duplicate {
		c.addAttributeDiagnostic(
			DiagnosticCodeDuplicateAttribute,
			"Duplicate attribute: "+name,
			"Duplicate attribute",
			attribute.Name.GetLocation(),
			DiagnosticLabel{Span: c.sourceSpan(original), Message: "first " + name + " attribute"},
		)
		return GoFieldTag{}, false
	}
	seen[key] = attribute.Name.GetLocation()
	if key == "json" {
		c.addAttributeDiagnostic(
			DiagnosticCodeInvalidAttributeArgument,
			"#go:json is reserved; use #json",
			"Reserved Go struct tag",
			attribute.Name.GetLocation(),
		)
		return GoFieldTag{}, false
	}
	if len(attribute.Arguments) != 1 || attribute.Arguments[0].Name != "" || attribute.Arguments[0].Value.Kind != parse.AttributeString {
		c.addAttributeDiagnostic(
			DiagnosticCodeInvalidAttributeArgument,
			name+" requires exactly one positional string argument",
			"Invalid Go struct tag",
			attribute.GetLocation(),
		)
		return GoFieldTag{}, false
	}
	return GoFieldTag{Key: key, Value: attribute.Arguments[0].Value.Text}, true
}

func (c *Checker) checkStructFieldAttributes(field parse.StructField, fieldType Type) (JSONFieldOptions, parse.Location, bool, bool, []GoFieldTag) {
	options := JSONFieldOptions{}
	jsonNameLocation := field.Name.GetLocation()
	seenJSON := false
	validJSON := true
	var jsonLocation parse.Location
	seenGoTags := make(map[string]parse.Location)
	goTags := make([]GoFieldTag, 0)
	for _, attribute := range field.Attributes {
		if attribute.Namespace != nil {
			if attribute.Namespace.Name != "go" {
				c.addAttributeDiagnostic(
					DiagnosticCodeUnknownAttribute,
					"Unknown attribute: "+attributeDisplayName(attribute),
					"Unknown attribute",
					attribute.Namespace.GetLocation(),
				)
				continue
			}
			if tag, valid := c.checkGoFieldTagAttribute(attribute, seenGoTags); valid {
				goTags = append(goTags, tag)
			}
			continue
		}
		if attribute.Name.Name != "json" {
			c.addAttributeDiagnostic(
				DiagnosticCodeUnknownAttribute,
				"Unknown attribute: "+attribute.Name.Name,
				"Unknown attribute",
				attribute.Name.GetLocation(),
			)
			continue
		}

		jsonDiagnosticCount := len(c.diagnostics)
		if seenJSON {
			c.addAttributeDiagnostic(
				DiagnosticCodeDuplicateAttribute,
				"Duplicate attribute: #json",
				"Duplicate attribute",
				attribute.Name.GetLocation(),
				DiagnosticLabel{Span: c.sourceSpan(jsonLocation), Message: "first #json attribute"},
			)
			validJSON = false
			continue
		}
		seenJSON = true
		jsonLocation = attribute.Name.GetLocation()
		if len(attribute.Arguments) == 0 {
			c.addAttributeDiagnostic(
				DiagnosticCodeInvalidAttributeArgument,
				"#json requires at least one argument",
				"Missing #json argument",
				attribute.GetLocation(),
			)
			validJSON = false
			continue
		}

		seenArguments := map[string]parse.Location{}
		for _, argument := range attribute.Arguments {
			argumentLocation := argument.GetLocation()
			if argument.Name == "" {
				c.addAttributeDiagnostic(
					DiagnosticCodeInvalidAttributeArgument,
					"#json only accepts named arguments",
					"Invalid #json argument",
					argumentLocation,
				)
				continue
			}
			if original, duplicate := seenArguments[argument.Name]; duplicate {
				c.addAttributeDiagnostic(
					DiagnosticCodeInvalidAttributeArgument,
					"Duplicate #json argument: "+argument.Name,
					"Duplicate #json argument",
					argument.NameLocation,
					DiagnosticLabel{Span: c.sourceSpan(original), Message: "first argument with this name"},
				)
				continue
			}
			seenArguments[argument.Name] = argument.NameLocation
			switch argument.Name {
			case "name":
				if argument.Value.Kind != parse.AttributeString {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json argument `name` must be a string",
						"Invalid #json name",
						argument.Value.GetLocation(),
					)
					continue
				}
				if !utf8.ValidString(argument.Value.Text) {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json argument `name` must be valid UTF-8",
						"Invalid #json name",
						argument.Value.GetLocation(),
					)
					continue
				}
				if !JSONFieldNameRepresentable(argument.Value.Text) {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json name cannot be represented by Go 1.27 JSON struct tags",
						"Unsupported #json name",
						argument.Value.GetLocation(),
					)
					continue
				}
				options.Name = argument.Value.Text
				options.HasName = true
				jsonNameLocation = argument.Value.GetLocation()
			case "omit":
				if argument.Value.Kind != parse.AttributeSymbol || argument.Value.Text != "none" {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json argument `omit` only supports `none`",
						"Invalid #json omission mode",
						argument.Value.GetLocation(),
					)
					continue
				}
				options.OmitNone = true
			case "skip":
				if argument.Value.Kind != parse.AttributeBool {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json argument `skip` must be `true`",
						"Invalid #json skip value",
						argument.Value.GetLocation(),
					)
					continue
				}
				if !argument.Value.Bool {
					c.addAttributeDiagnostic(
						DiagnosticCodeInvalidAttributeArgument,
						"#json(skip: false) has no effect",
						"Ineffective #json argument",
						argument.Value.GetLocation(),
					)
					continue
				}
				options.Skip = true
			default:
				c.addAttributeDiagnostic(
					DiagnosticCodeInvalidAttributeArgument,
					"Unknown #json argument: "+argument.Name,
					"Unknown #json argument",
					argument.NameLocation,
				)
			}
		}
		if len(c.diagnostics) != jsonDiagnosticCount {
			validJSON = false
		}
	}

	jsonDiagnosticCount := len(c.diagnostics)
	if options.OmitNone && !IsMaybe(fieldType) {
		c.addAttributeDiagnostic(
			DiagnosticCodeInvalidAttributeArgument,
			"#json(omit: none) requires a nullable field",
			"Invalid #json omission",
			field.Name.GetLocation(),
		)
	}
	if options.Skip && (options.HasName || options.OmitNone) {
		c.addAttributeDiagnostic(
			DiagnosticCodeInvalidAttributeArgument,
			"#json argument `skip` cannot be combined with `name` or `omit`",
			"Conflicting #json arguments",
			jsonLocation,
		)
	}
	if len(c.diagnostics) != jsonDiagnosticCount {
		validJSON = false
	}
	return options, jsonNameLocation, seenJSON, validJSON, goTags
}

// JSONFieldNameRepresentable reports whether Go 1.27's JSON struct-tag
// grammar can express name without changing its meaning.
func JSONFieldNameRepresentable(name string) bool {
	return utf8.ValidString(name) && name != "" && name != "-" && !strings.ContainsAny(name, ",\\'\"`")
}
