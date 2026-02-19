package formatter

import "strings"

// Format applies Ard source formatting rules.
func Format(input []byte) []byte {
	if len(input) == 0 {
		return input
	}

	source := strings.ReplaceAll(string(input), "\r\n", "\n")
	source = strings.ReplaceAll(source, "\r", "\n")

	lines := strings.Split(source, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}

	formatted := strings.Join(lines, "\n")
	if formatted != "" && !strings.HasSuffix(formatted, "\n") {
		formatted += "\n"
	}

	return []byte(formatted)
}
