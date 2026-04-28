package go_backend

import backendir "github.com/akonwi/ard/go_backend/ir"

func emitGoFileFromBackendIRWithImports(module *backendir.Module, imports map[string]string, entrypoint bool) (goFileIR, error) {
	if module != nil {
		module.Imports = imports
	}
	return emitGoFileFromBackendIR(module, entrypoint)
}
