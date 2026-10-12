package formatter

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
)

// Format applies Ard formatting rules.
func Format(input []byte, fileName string) ([]byte, error) {
	normalized := normalizeLineEndings(string(input))
	if strings.TrimSpace(normalized) == "" {
		return []byte(normalizeWhitespace(normalized)), nil
	}

	result := parse.Parse([]byte(normalized), fileName)
	if len(result.Errors) > 0 {
		lines := make([]string, 0, len(result.Errors))
		for _, err := range result.Errors {
			lines = append(lines, fmt.Sprintf("%s %s", err.Location.Start, err.Message))
		}
		return nil, fmt.Errorf("cannot format invalid Ard source:\n%s", strings.Join(lines, "\n"))
	}

	removeUnusedImports(result.Program)

	printer := newPrinter(100)
	formatted := []byte(ensureFinalNewline(printer.program(result.Program)))
	if err := checkFormattedOutputParses(formatted, fileName); err != nil {
		return nil, err
	}
	return formatted, nil
}

// checkFormattedOutputParses rejects formatter output that no longer parses,
// so a printer bug fails the format instead of corrupting valid source (#546).
func checkFormattedOutputParses(formatted []byte, fileName string) error {
	result := parse.Parse(formatted, fileName)
	if len(result.Errors) == 0 {
		return nil
	}
	first := result.Errors[0]
	return fmt.Errorf("refusing to format %s: the formatted source would not parse (%s %s); the file was left unchanged. This is a formatter bug, please report it", fileName, first.Location.Start, first.Message)
}

func normalizeLineEndings(source string) string {
	source = strings.ReplaceAll(source, "\r\n", "\n")
	return strings.ReplaceAll(source, "\r", "\n")
}

func ensureFinalNewline(source string) string {
	if source != "" && !strings.HasSuffix(source, "\n") {
		return source + "\n"
	}
	return source
}

func normalizeWhitespace(source string) string {
	if source == "" {
		return ""
	}

	source = normalizeLineEndings(source)

	lines := strings.Split(source, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}

	normalized := strings.Join(lines, "\n")
	if normalized != "" && !strings.HasSuffix(normalized, "\n") {
		normalized += "\n"
	}
	return normalized
}
