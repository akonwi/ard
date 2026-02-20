package formatter

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/parse"
)

// Format applies Ard formatting rules.
func Format(input []byte, fileName string) ([]byte, error) {
	normalized := normalizeWhitespace(string(input))
	if strings.TrimSpace(normalized) == "" {
		return []byte(normalized), nil
	}

	result := parse.Parse([]byte(normalized), fileName)
	if len(result.Errors) > 0 {
		lines := make([]string, 0, len(result.Errors))
		for _, err := range result.Errors {
			lines = append(lines, fmt.Sprintf("%s %s", err.Location.Start, err.Message))
		}
		return nil, fmt.Errorf("cannot format invalid Ard source:\n%s", strings.Join(lines, "\n"))
	}

	printer := newPrinter(100)
	formatted := printer.program(result.Program)
	return []byte(normalizeWhitespace(formatted)), nil
}

func normalizeWhitespace(source string) string {
	if source == "" {
		return ""
	}

	source = strings.ReplaceAll(source, "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

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
