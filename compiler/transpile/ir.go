package transpile

type goFileIR struct {
	PackageName string
	Imports     []goImportIR
	Decls       []goDeclIR
}

type goDeclIR struct {
	Source string
}

type goImportIR struct {
	Alias string
	Path  string
}

func lowerGoFileIR(packageName string, imports map[string]string) goFileIR {
	file := goFileIR{PackageName: packageName}
	paths := sortedImportPaths(imports)
	file.Imports = make([]goImportIR, 0, len(paths))
	for _, importPath := range paths {
		file.Imports = append(file.Imports, goImportIR{
			Alias: imports[importPath],
			Path:  importPath,
		})
	}
	return file
}
