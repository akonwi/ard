package diagnostics

import (
	"io"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func ParseErrors(filePath string, parseErrors []parse.ParseError) []checker.Diagnostic {
	result := make([]checker.Diagnostic, len(parseErrors))
	for i, parseError := range parseErrors {
		location := normalizeParseErrorLocation(parseError.Location)
		result[i] = checker.Diagnostic{
			Kind:    checker.Error,
			Code:    checker.DiagnosticCodeParseError,
			Message: parseError.Message,
			Title:   "Parse error",
			Primary: checker.DiagnosticLabel{
				Span:    checker.SourceSpan{FilePath: filePath, Location: location},
				Message: parseError.Message,
			},
		}
	}
	return result
}

func RenderParseErrors(w io.Writer, filePath string, parseErrors []parse.ParseError) error {
	return Render(w, ParseErrors(filePath, parseErrors), FileSourceProvider())
}

func normalizeParseErrorLocation(location parse.Location) parse.Location {
	if location.Start.Row < 1 {
		location.Start.Row = 1
	}
	if location.Start.Col < 1 {
		location.Start.Col = 1
	}
	if location.End.Row < 1 {
		location.End.Row = location.Start.Row
	}
	if location.End.Col < 1 {
		location.End.Col = location.Start.Col
	}
	return location
}
