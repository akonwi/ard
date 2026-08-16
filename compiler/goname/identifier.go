// Package goname contains the Ard-to-Go identifier mapping shared by the
// checker and Go backend. Keeping the conversion target-neutral prevents the
// checker from accepting declarations that the backend cannot emit.
package goname

import (
	"go/token"
	"strings"
	"unicode"
)

// NaturalIdentifier converts an Ard identifier to its natural Go spelling.
func NaturalIdentifier(raw string, exported bool) string {
	parts := identifierParts(raw)
	if len(parts) == 0 {
		if exported {
			return "Exported"
		}
		return "name"
	}
	var b strings.Builder
	for i, part := range parts {
		if i == 0 && !exported {
			b.WriteString(lowerFirst(part))
			continue
		}
		b.WriteString(upperFirst(part))
	}
	name := b.String()
	if name == "" {
		if exported {
			return "Exported"
		}
		return "name"
	}
	if !exported && token.Lookup(name) != token.IDENT {
		name += "_"
	}
	return name
}

func identifierParts(raw string) []string {
	sanitized := sanitizeIdentifier(raw)
	if sanitized == "" {
		return nil
	}
	chunks := strings.Split(sanitized, "_")
	parts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			parts = append(parts, chunk)
		}
	}
	return parts
}

func sanitizeIdentifier(raw string) string {
	if raw == "" {
		return ""
	}
	var out []rune
	lastUnderscore := false
	for _, r := range raw {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			r = '_'
		}
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		out = append(out, r)
	}
	name := string(out)
	if name == "" {
		return ""
	}
	if unicode.IsDigit([]rune(name)[0]) {
		name = "_" + name
	}
	return name
}

func upperFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}
