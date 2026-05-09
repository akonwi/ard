package javascript

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/frontend"
)

type ffiArtifacts struct {
	useStdlib  bool
	useProject bool
}

func Build(inputPath, outputPath, target string) (string, error) {
	program, projectInfo, err := loadAIRProgram(inputPath, target)
	if err != nil {
		return "", err
	}
	return BuildProgram(program, outputPath, target, projectInfo)
}

func Run(inputPath, target string, args []string) error {
	if target == backend.TargetJSBrowser {
		return fmt.Errorf("js-browser cannot be run directly; build and serve the emitted module instead")
	}
	program, projectInfo, err := loadAIRProgram(inputPath, target)
	if err != nil {
		return err
	}
	return RunProgram(program, target, args, projectInfo)
}

func loadAIRProgram(inputPath, target string) (*air.Program, *checker.ProjectInfo, error) {
	loaded, err := frontend.LoadModule(inputPath, target)
	if err != nil {
		return nil, nil, err
	}
	program, err := air.Lower(loaded.Module)
	if err != nil {
		return nil, nil, err
	}
	return program, loaded.ProjectInfo, nil
}

func compilerJSSourcePath(parts ...string) (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("unable to resolve javascript.go path")
	}
	compilerDir := filepath.Dir(filepath.Dir(currentFile))
	all := append([]string{compilerDir}, parts...)
	return filepath.Join(all...), nil
}

func stdlibFFISourcePath(target string) (string, error) {
	switch target {
	case backend.TargetJSServer:
		return compilerJSSourcePath("std_lib", "ffi.js-server.mjs")
	case backend.TargetJSBrowser:
		return compilerJSSourcePath("std_lib", "ffi.js-browser.mjs")
	default:
		return "", fmt.Errorf("unsupported JS ffi target: %s", target)
	}
}

func preludeSourcePath() (string, error) {
	return compilerJSSourcePath("javascript", "ard.prelude.mjs")
}

func writeFFICompanions(outputDir string, target string, projectInfo *checker.ProjectInfo, ffi ffiArtifacts) error {
	if target != backend.TargetJSServer && target != backend.TargetJSBrowser {
		return nil
	}
	preludePath, err := preludeSourcePath()
	if err != nil {
		return err
	}
	preludeContent, err := os.ReadFile(preludePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "ard.prelude.mjs"), preludeContent, 0o644); err != nil {
		return err
	}
	if ffi.useStdlib {
		sourcePath, err := stdlibFFISourcePath(target)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "ffi.stdlib."+target+".mjs"), content, 0o644); err != nil {
			return err
		}
	}
	if ffi.useProject && projectInfo != nil {
		sourcePath := filepath.Join(projectInfo.RootPath, "ffi."+target+".mjs")
		content, err := os.ReadFile(sourcePath)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(outputDir, "ffi.project."+target+".mjs"), content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func moduleAlias(path string) string {
	replacer := strings.NewReplacer("/", "_", "-", "_", ".", "_")
	return jsName(replacer.Replace(path))
}

func moduleOutputPath(path string) string {
	return filepath.ToSlash(path) + ".mjs"
}

func relativeJSImport(fromOutputPath string, toOutputPath string) string {
	fromDir := filepath.Dir(filepath.FromSlash(fromOutputPath))
	toPath := filepath.FromSlash(toOutputPath)
	rel, err := filepath.Rel(fromDir, toPath)
	if err != nil {
		return "./" + filepath.ToSlash(toOutputPath)
	}
	out := filepath.ToSlash(rel)
	if !strings.HasPrefix(out, "./") && !strings.HasPrefix(out, "../") {
		out = "./" + out
	}
	return out
}

func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if i == 0 {
			if r != '$' && r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '$' && r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func jsName(name string) string {
	replacer := strings.NewReplacer("::", "__", "-", "_", ".", "_")
	out := replacer.Replace(name)
	switch out {
	case "break", "case", "catch", "class", "const", "continue", "debugger", "default", "delete", "do", "else", "export", "extends", "finally", "for", "function", "if", "import", "in", "instanceof", "new", "return", "super", "switch", "this", "throw", "try", "typeof", "var", "void", "while", "with", "yield", "let", "static", "enum", "await", "implements", "package", "protected", "interface", "private", "public", "null", "true", "false":
		return out + "_"
	default:
		return out
	}
}

func escapeTemplateLiteral(raw string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "`", "\\`", "${", "\\${")
	return replacer.Replace(raw)
}
