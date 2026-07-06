package runtime

import "embed"

// SourceFiles embeds the runtime support files copied into generated programs.
// Keep SourceFileNames in sync with this directive.
//
//go:embed maybe.go result.go unsafe.go
var SourceFiles embed.FS

var SourceFileNames = []string{
	"maybe.go",
	"result.go",
	"unsafe.go",
}
