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

func (c *Checker) checkStructFieldAttributes(field parse.StructField, fieldType Type) (JSONFieldOptions, parse.Location, bool, bool) {
	diagnosticCount := len(c.diagnostics)
	options := JSONFieldOptions{}
	jsonNameLocation := field.Name.GetLocation()
	seenJSON := false
	var jsonLocation parse.Location
	for _, attribute := range field.Attributes {
		if attribute.Name.Name != "json" {
			c.addAttributeDiagnostic(
				DiagnosticCodeUnknownAttribute,
				"Unknown attribute: "+attribute.Name.Name,
				"Unknown attribute",
				attribute.Name.GetLocation(),
			)
			continue
		}
		if seenJSON {
			c.addAttributeDiagnostic(
				DiagnosticCodeDuplicateAttribute,
				"Duplicate attribute: #json",
				"Duplicate attribute",
				attribute.Name.GetLocation(),
				DiagnosticLabel{Span: c.sourceSpan(jsonLocation), Message: "first #json attribute"},
			)
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
	}
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
	return options, jsonNameLocation, seenJSON, len(c.diagnostics) == diagnosticCount
}

// JSONFieldNameRepresentable reports whether Go 1.27's JSON struct-tag
// grammar can express name without changing its meaning.
func JSONFieldNameRepresentable(name string) bool {
	return utf8.ValidString(name) && name != "" && name != "-" && !strings.ContainsAny(name, ",\\'\"`")
}
