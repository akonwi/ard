package transpile

import "strings"

func optimizeGoFileIR(fileIR goFileIR) goFileIR {
	optimized := goFileIR{PackageName: fileIR.PackageName}
	if len(fileIR.Imports) > 0 {
		optimized.Imports = make([]goImportIR, 0, len(fileIR.Imports))
		seenImports := make(map[string]struct{}, len(fileIR.Imports))
		for _, imp := range fileIR.Imports {
			key := imp.Alias + "\x00" + imp.Path
			if _, ok := seenImports[key]; ok {
				continue
			}
			seenImports[key] = struct{}{}
			optimized.Imports = append(optimized.Imports, imp)
		}
	}
	if len(fileIR.Decls) > 0 {
		optimized.Decls = make([]goDeclIR, 0, len(fileIR.Decls))
		for _, decl := range fileIR.Decls {
			trimmed := strings.TrimSpace(decl.Source)
			if trimmed == "" {
				continue
			}
			optimized.Decls = append(optimized.Decls, goDeclIR{Source: trimmed})
		}
	}
	return optimized
}
