package lsp

import (
	"fmt"

	"go.lsp.dev/protocol"
)

func applyDocumentChanges(text string, changes []documentContentChange) (string, error) {
	updated := text
	for _, change := range changes {
		if change.Range == nil {
			updated = change.Text
			continue
		}
		start, err := lspPositionByteOffset(updated, change.Range.Start)
		if err != nil {
			return "", fmt.Errorf("change start: %w", err)
		}
		end, err := lspPositionByteOffset(updated, change.Range.End)
		if err != nil {
			return "", fmt.Errorf("change end: %w", err)
		}
		if end < start {
			return "", fmt.Errorf("range end precedes start")
		}
		updated = updated[:start] + change.Text + updated[end:]
	}
	return updated, nil
}

func lspPositionByteOffset(text string, position protocol.Position) (int, error) {
	line := uint32(0)
	character := uint32(0)
	for offset, r := range text {
		if line == position.Line && character >= position.Character {
			return offset, nil
		}
		if r == '\n' {
			if line == position.Line {
				return offset, nil
			}
			line++
			character = 0
			continue
		}
		if line == position.Line {
			character += lspUTF16Width(r)
		}
	}
	return len(text), nil
}

func lspUTF16Width(r rune) uint32 {
	if r > 0xffff {
		return 2
	}
	return 1
}
