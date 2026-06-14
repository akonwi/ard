package gotarget

//go:generate go run generate_ard_module_files.go

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	stdlibffi "github.com/akonwi/ard/std_lib/ffi"
	"github.com/akonwi/ard/version"
)

type Options struct {
	PackageName  string
	ProjectInfo  *checker.ProjectInfo
	SuppressMain bool
	IncludeTests bool
}

type TestCase struct {
	Name        string
	DisplayName string
	Function    air.FunctionID
}

type TestOutcome struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

func GenerateSources(program *air.Program, options Options) (map[string][]byte, error) {
	generated, err := lowerProgram(program, options)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(generated))
	for name, file := range generated {
		source, err := renderFile(file)
		if err != nil {
			return nil, err
		}
		out[name] = source
	}
	return out, nil
}

func RunProgram(program *air.Program, args []string, projectInfo ...*checker.ProjectInfo) error {
	workspaceDir, err := artifactWorkspace(inputPathFromCLIArgs(args), "run")
	if err != nil {
		return err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: optionalProjectInfo(projectInfo)}); err != nil {
		return err
	}
	binaryPath := runBinaryPath(workspaceDir, optionalProjectInfo(projectInfo))
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return err
	}
	if err := buildGeneratedProgram(workspaceDir, binaryPath); err != nil {
		return err
	}
	cmd := exec.Command(binaryPath, programArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func BuildProgram(program *air.Program, outputPath string, projectInfo ...*checker.ProjectInfo) (string, error) {
	workspaceDir, err := artifactWorkspace(outputPath, "build")
	if err != nil {
		return "", err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: optionalProjectInfo(projectInfo)}); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(workspaceDir, absOutput); err != nil {
		return "", err
	}
	return absOutput, nil
}

func RunTests(program *air.Program, args []string, tests []TestCase, failFast bool, projectInfo ...*checker.ProjectInfo) ([]TestOutcome, error) {
	workspaceDir, err := artifactWorkspace(inputPathFromCLIArgs(args), "test")
	if err != nil {
		return nil, err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: optionalProjectInfo(projectInfo), SuppressMain: true, IncludeTests: true}); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "ard_tests.go"), []byte(renderTestRunner(program, tests, failFast)), 0o644); err != nil {
		return nil, err
	}
	binaryPath := filepath.Join(workspaceDir, "ard-tests")
	if err := buildGeneratedProgram(workspaceDir, binaryPath); err != nil {
		return nil, err
	}
	resultPath := filepath.Join(workspaceDir, "test-results.json")
	cmd := exec.Command(binaryPath, programArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "ARD_TEST_RESULTS="+resultPath)
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resultPath)
	if err != nil {
		return nil, err
	}
	var outcomes []TestOutcome
	if err := json.Unmarshal(data, &outcomes); err != nil {
		return nil, err
	}
	return outcomes, nil
}

func renderTestRunner(program *air.Program, tests []TestCase, failFast bool) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	b.WriteString("\t\"encoding/json\"\n")
	b.WriteString("\t\"fmt\"\n")
	b.WriteString("\t\"os\"\n")
	b.WriteString("\truntime \"github.com/akonwi/ard/runtime\"\n")
	b.WriteString(")\n\n")
	b.WriteString("type ardTestOutcome struct {\n")
	b.WriteString("\tName string `json:\"name\"`\n")
	b.WriteString("\tDisplayName string `json:\"displayName\"`\n")
	b.WriteString("\tStatus string `json:\"status\"`\n")
	b.WriteString("\tMessage string `json:\"message,omitempty\"`\n")
	b.WriteString("}\n\n")
	b.WriteString("func ardRunTest(name string, displayName string, fn func() runtime.Result[runtime.Void, string]) (out ardTestOutcome) {\n")
	b.WriteString("\tout = ardTestOutcome{Name: name, DisplayName: displayName, Status: \"panic\"}\n")
	b.WriteString("\tdefer func() { if recovered := recover(); recovered != nil { out.Status = \"panic\"; out.Message = fmt.Sprint(recovered) } }()\n")
	b.WriteString("\tresult := fn()\n")
	b.WriteString("\tif result.Ok { out.Status = \"pass\"; out.Message = \"\" } else { out.Status = \"fail\"; out.Message = result.Err }\n")
	b.WriteString("\treturn out\n")
	b.WriteString("}\n\n")
	b.WriteString("func main() {\n")
	b.WriteString("\toutcomes := []ardTestOutcome{}\n")
	for _, test := range tests {
		if test.Function < 0 || int(test.Function) >= len(program.Functions) {
			continue
		}
		fn := program.Functions[test.Function]
		fmt.Fprintf(&b, "\toutcomes = append(outcomes, ardRunTest(%s, %s, %s))\n", strconv.Quote(test.Name), strconv.Quote(test.DisplayName), functionName(program, fn))
		if failFast {
			b.WriteString("\tif outcomes[len(outcomes)-1].Status != \"pass\" { goto done }\n")
		}
	}
	if failFast {
		b.WriteString("done:\n")
	}
	b.WriteString("\tdata, err := json.Marshal(outcomes)\n")
	b.WriteString("\tif err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }\n")
	b.WriteString("\tif path := os.Getenv(\"ARD_TEST_RESULTS\"); path != \"\" {\n")
	b.WriteString("\t\tif err := os.WriteFile(path, data, 0o644); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }\n")
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")
	b.WriteString("\t_, _ = os.Stdout.Write(data)\n")
	b.WriteString("}\n")
	return b.String()
}

func writeProgram(dir string, program *air.Program, options Options) error {
	sources, err := GenerateSources(program, options)
	if err != nil {
		return err
	}
	for name, source := range sources {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, source, 0o644); err != nil {
			return err
		}
	}
	if err := writeProjectFFICompanions(dir, program, options.ProjectInfo); err != nil {
		return err
	}
	if err := writeDependencyFFICompanions(dir, program, options.ProjectInfo); err != nil {
		return err
	}
	goMod, err := generatedGoMod(dir, program, options.ProjectInfo)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return err
	}
	if err := mergeGoSum(dir, program, options.ProjectInfo); err != nil {
		return err
	}
	return nil
}

func generatedGoMod(dir string, program *air.Program, projectInfo *checker.ProjectInfo) (string, error) {
	goMod := "module generated\n\ngo 1.26.0\n"
	ardRequirement, err := writeArdModuleDependency(dir)
	if err != nil {
		return "", err
	}
	goMod += ardRequirement

	requireSeen := requireKeys(goMod)
	requires := make([]string, 0)
	addDependencyGoModRequirements(&requires, requireSeen, projectInfo)
	if programUsesProjectFFI(program, projectInfo) {
		addProjectGoModRequirements(&requires, requireSeen, projectInfo)
	}
	addGoModRequirementsFromFile(&requires, requireSeen, filepath.Join(dir, "go.mod"))
	goMod += formatRequireBlock(requires)

	replaceSeen := replaceKeys(goMod)
	replaces := make([]string, 0)
	if programUsesProjectFFI(program, projectInfo) {
		addProjectGoModReplaces(&replaces, replaceSeen, projectInfo)
	}
	addGoModReplacesFromFile(&replaces, replaceSeen, filepath.Join(dir, "go.mod"), dir)
	goMod += formatReplaceBlock(replaces)
	return goMod, nil
}

func addDependencyGoModRequirements(out *[]string, seen map[string]bool, projectInfo *checker.ProjectInfo) {
	if projectInfo == nil || len(projectInfo.Dependencies) == 0 {
		return
	}
	for _, dep := range projectInfo.Dependencies {
		if root := dependencyRootPath(dep); root != "" {
			addGoModRequirementsFromFile(out, seen, filepath.Join(root, "go.mod"))
		}
	}
}

func addProjectGoModRequirements(out *[]string, seen map[string]bool, projectInfo *checker.ProjectInfo) {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return
	}
	addGoModRequirementsFromFile(out, seen, filepath.Join(projectInfo.RootPath, "go.mod"))
}

func addGoModRequirementsFromFile(out *[]string, seen map[string]bool, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	addGoModRequirements(out, seen, string(data))
}

func addGoModRequirements(out *[]string, seen map[string]bool, goMod string) {
	for _, req := range extractRequireLines(goMod) {
		key := requirementKey(req)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		*out = append(*out, req)
	}
}

func requireKeys(goMod string) map[string]bool {
	seen := map[string]bool{}
	for _, req := range extractRequireLines(goMod) {
		if key := requirementKey(req); key != "" {
			seen[key] = true
		}
	}
	return seen
}

func requirementKey(req string) string {
	fields := strings.Fields(req)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func formatRequireBlock(requires []string) string {
	if len(requires) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\nrequire (\n")
	for _, req := range requires {
		out.WriteString("\t" + req + "\n")
	}
	out.WriteString(")\n")
	return out.String()
}

func extractRequireLines(goMod string) []string {
	lines := []string{}
	inBlock := false
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			lines = append(lines, trimmed)
			continue
		}
		if trimmed == "require (" {
			inBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "require ") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(trimmed, "require ")))
		}
	}
	return lines
}

func addProjectGoModReplaces(out *[]string, seen map[string]bool, projectInfo *checker.ProjectInfo) {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return
	}
	addGoModReplacesFromFile(out, seen, filepath.Join(projectInfo.RootPath, "go.mod"), projectInfo.RootPath)
}

func addGoModReplacesFromFile(out *[]string, seen map[string]bool, path string, baseDir string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, replace := range extractReplaceLines(string(data)) {
		normalized, ok := normalizeReplaceLine(replace, baseDir)
		if !ok {
			continue
		}
		key := replaceKey(normalized)
		if key == "" || replacedModulePath(key) == "github.com/akonwi/ard" || seen[key] {
			continue
		}
		seen[key] = true
		*out = append(*out, normalized)
	}
}

func replaceKeys(goMod string) map[string]bool {
	seen := map[string]bool{}
	for _, replace := range extractReplaceLines(goMod) {
		if key := replaceKey(replace); key != "" {
			seen[key] = true
		}
	}
	return seen
}

func extractReplaceLines(goMod string) []string {
	lines := []string{}
	inBlock := false
	for _, line := range strings.Split(goMod, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		if inBlock {
			if trimmed == ")" {
				inBlock = false
				continue
			}
			lines = append(lines, trimmed)
			continue
		}
		if trimmed == "replace (" {
			inBlock = true
			continue
		}
		if strings.HasPrefix(trimmed, "replace ") {
			lines = append(lines, strings.TrimSpace(strings.TrimPrefix(trimmed, "replace ")))
		}
	}
	return lines
}

func normalizeReplaceLine(line string, baseDir string) (string, bool) {
	parts := strings.SplitN(line, "=>", 2)
	if len(parts) != 2 {
		return "", false
	}
	lhs := strings.TrimSpace(parts[0])
	rhs := strings.TrimSpace(parts[1])
	rhsWithoutComment := strings.TrimSpace(strings.SplitN(rhs, "//", 2)[0])
	fields := strings.Fields(rhsWithoutComment)
	if len(fields) == 1 && baseDir != "" && isLocalReplacePath(fields[0]) {
		path := fields[0]
		if !filepath.IsAbs(path) {
			path = filepath.Join(baseDir, path)
		}
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
		rhs = filepath.Clean(path)
	}
	if lhs == "" || rhs == "" {
		return "", false
	}
	return lhs + " => " + rhs, true
}

func replaceKey(replace string) string {
	parts := strings.SplitN(replace, "=>", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func replacedModulePath(key string) string {
	fields := strings.Fields(key)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func isLocalReplacePath(path string) bool {
	return filepath.IsAbs(path) || strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../")
}

func formatReplaceBlock(replaces []string) string {
	if len(replaces) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString("\nreplace (\n")
	for _, replace := range replaces {
		out.WriteString("\t" + replace + "\n")
	}
	out.WriteString(")\n")
	return out.String()
}

func mergeGoSum(dir string, program *air.Program, projectInfo *checker.ProjectInfo) error {
	goSumPath := filepath.Join(dir, "go.sum")
	lines := make([]string, 0)
	seen := map[string]bool{}
	addGoSumLines(&lines, seen, goSumPath)
	if programUsesProjectFFI(program, projectInfo) && projectInfo != nil && strings.TrimSpace(projectInfo.RootPath) != "" {
		addGoSumLines(&lines, seen, filepath.Join(projectInfo.RootPath, "go.sum"))
	}
	if len(lines) == 0 {
		return nil
	}
	return os.WriteFile(goSumPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

func addGoSumLines(out *[]string, seen map[string]bool, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		*out = append(*out, trimmed)
	}
}

func optionalProjectInfo(projectInfo []*checker.ProjectInfo) *checker.ProjectInfo {
	if len(projectInfo) == 0 {
		return nil
	}
	return projectInfo[0]
}

func dependencyRootPath(dep checker.DependencyInfo) string {
	if dep.RootPath != "" {
		return dep.RootPath
	}
	if dep.SourcePath != "" {
		return dep.SourcePath
	}
	return dep.VendorPath
}

func runBinaryPath(workspaceDir string, projectInfo *checker.ProjectInfo) string {
	return filepath.Join(workspaceDir, ".bin", runBinaryName(projectInfo))
}

func runBinaryName(projectInfo *checker.ProjectInfo) string {
	const fallback = "ard-program"
	if projectInfo == nil {
		return fallback
	}
	name := sanitizeRunBinaryName(projectInfo.ProjectName)
	if name == "" {
		return fallback
	}
	if isWindowsReservedFileName(name) {
		return "ard-" + name
	}
	return name
}

func sanitizeRunBinaryName(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var out strings.Builder
	hasNameChar := false
	for _, r := range raw {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\\|?*`, r) {
			out.WriteByte('_')
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			hasNameChar = true
		}
		out.WriteRune(r)
	}
	name := strings.Trim(out.String(), " .")
	if !hasNameChar || name == "" {
		return ""
	}
	return name
}

func isWindowsReservedFileName(name string) bool {
	base := strings.TrimRight(name, " .")
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	if len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
		return true
	}
	return false
}

func inputPathFromCLIArgs(args []string) string {
	if len(args) >= 3 && strings.TrimSpace(args[2]) != "" {
		return args[2]
	}
	return "."
}

func artifactWorkspace(pathHint string, purpose string) (string, error) {
	rootDir, err := artifactRootDir(pathHint)
	if err != nil {
		return "", err
	}
	base := filepath.Join(rootDir, "ard-out", "go", purpose)
	preserved, err := readGoModuleFiles(base)
	if err != nil {
		return "", err
	}
	if err := os.RemoveAll(base); err != nil {
		return "", err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}
	if err := preserved.write(base); err != nil {
		return "", err
	}
	return base, nil
}

type goModuleFiles struct {
	goMod []byte
	goSum []byte
}

func readGoModuleFiles(dir string) (goModuleFiles, error) {
	var files goModuleFiles
	var err error
	files.goMod, err = readOptionalFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return files, err
	}
	files.goSum, err = readOptionalFile(filepath.Join(dir, "go.sum"))
	if err != nil {
		return files, err
	}
	return files, nil
}

func readOptionalFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return data, nil
	}
	if os.IsNotExist(err) {
		return nil, nil
	}
	return nil, err
}

func (files goModuleFiles) write(dir string) error {
	if files.goMod != nil {
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), files.goMod, 0o644); err != nil {
			return err
		}
	}
	if files.goSum != nil {
		if err := os.WriteFile(filepath.Join(dir, "go.sum"), files.goSum, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func artifactRootDir(pathHint string) (string, error) {
	if strings.TrimSpace(pathHint) == "" {
		return os.Getwd()
	}
	pathHint = filepath.Clean(pathHint)
	absPath, err := filepath.Abs(pathHint)
	if err != nil {
		return "", err
	}
	candidate := absPath
	if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
		candidate = filepath.Dir(absPath)
	} else if statErr != nil {
		candidate = filepath.Dir(absPath)
	}
	if project, err := checker.FindProjectRoot(candidate); err == nil && strings.TrimSpace(project.RootPath) != "" {
		return project.RootPath, nil
	}
	return candidate, nil
}

func writeProjectFFICompanions(dir string, program *air.Program, projectInfo *checker.ProjectInfo) error {
	if !programUsesProjectFFI(program, projectInfo) {
		return nil
	}
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return fmt.Errorf("go target uses project externs but project information is unavailable")
	}
	rootFile := filepath.Join(projectInfo.RootPath, "ffi.go")
	dirMatches, err := filepath.Glob(filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	if err != nil {
		return err
	}
	rootExists := fileExists(rootFile)
	if rootExists && len(dirMatches) > 0 {
		return fmt.Errorf("project Go FFI must use either %s or %s, not both", rootFile, filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	}
	ffiDir := filepath.Join(dir, projectFFIPackageAlias(projectInfo))
	if rootExists {
		if err := copyProjectFFIFile(rootFile, filepath.Join(ffiDir, filepath.Base(rootFile))); err != nil {
			return err
		}
		return nil
	}
	if len(dirMatches) > 0 {
		for _, sourcePath := range dirMatches {
			if err := copyProjectFFIFile(sourcePath, filepath.Join(ffiDir, filepath.Base(sourcePath))); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("go target uses project externs but no project Go FFI companion was found at %s or %s", rootFile, filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
}

func copyProjectFFIFile(sourcePath, destPath string) error {
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read project Go FFI companion %s: %w", sourcePath, err)
	}
	file, err := parser.ParseFile(token.NewFileSet(), sourcePath, content, parser.PackageClauseOnly)
	if err != nil {
		return fmt.Errorf("parse project Go FFI companion %s: %w", sourcePath, err)
	}
	if file.Name == nil || file.Name.Name != "ffi" {
		pkg := ""
		if file.Name != nil {
			pkg = file.Name.Name
		}
		return fmt.Errorf("project Go FFI companion %s must use package ffi, got package %s", sourcePath, pkg)
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destPath, content, 0o644); err != nil {
		return err
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeDependencyFFICompanions(dir string, program *air.Program, projectInfo *checker.ProjectInfo) error {
	if projectInfo == nil || len(projectInfo.Dependencies) == 0 {
		return nil
	}
	used := dependencyFFIAliases(program, projectInfo)
	for alias := range used {
		dep, ok := projectInfo.Dependencies[alias]
		if !ok {
			continue
		}
		root := dependencyRootPath(dep)
		matches, err := filepath.Glob(filepath.Join(root, "ffi", "*.go"))
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("go target uses dependency externs from %s but no Go FFI companion was found at %s", alias, filepath.Join(root, "ffi", "*.go"))
		}
		ffiDir := filepath.Join(dir, "depffi", sanitizeName(alias))
		for _, sourcePath := range matches {
			if err := copyProjectFFIFile(sourcePath, filepath.Join(ffiDir, filepath.Base(sourcePath))); err != nil {
				return err
			}
		}
	}
	return nil
}

func dependencyFFIAliases(program *air.Program, projectInfo *checker.ProjectInfo) map[string]bool {
	used := map[string]bool{}
	if program == nil || projectInfo == nil {
		return used
	}
	for _, ext := range program.Externs {
		if alias, ok := dependencyAliasForModulePath(modulePathForExtern(program, ext), projectInfo); ok {
			used[alias] = true
		}
	}
	return used
}

func modulePathForExtern(program *air.Program, ext air.Extern) string {
	if program != nil && int(ext.Module) >= 0 && int(ext.Module) < len(program.Modules) {
		return program.Modules[ext.Module].Path
	}
	return ""
}

func dependencyAliasForModulePath(modulePath string, projectInfo *checker.ProjectInfo) (string, bool) {
	if projectInfo == nil || modulePath == "" {
		return "", false
	}
	first := strings.Split(modulePath, "/")[0]
	_, ok := projectInfo.Dependencies[first]
	return first, ok
}

func programUsesProjectFFI(program *air.Program, projectInfo *checker.ProjectInfo) bool {
	if program == nil {
		return false
	}
	for _, ext := range program.Externs {
		if externModuleIsStdlib(program, ext) {
			continue
		}
		if _, ok := dependencyAliasForModulePath(modulePathForExtern(program, ext), projectInfo); ok {
			continue
		}
		return true
	}
	for _, typ := range program.Types {
		if typ.Kind != air.TypeExtern || strings.HasPrefix(typ.ModulePath, "ard/") {
			continue
		}
		if _, ok := dependencyAliasForModulePath(typ.ModulePath, projectInfo); ok {
			continue
		}
		if externBindingUsesProjectFFIType(typ.ExternBinding, projectInfo) {
			return true
		}
	}
	return false
}

func externBindingUsesProjectFFIType(binding string, projectInfo *checker.ProjectInfo) bool {
	expr, err := parser.ParseExpr(binding)
	if err != nil {
		return false
	}
	return exprUsesProjectFFIType(expr, projectFFIPackageAlias(projectInfo))
}

func exprUsesProjectFFIType(expr ast.Expr, projectAlias string) bool {
	switch node := expr.(type) {
	case *ast.Ident:
		return false
	case *ast.StarExpr:
		return exprUsesProjectFFIType(node.X, projectAlias)
	case *ast.ArrayType:
		return exprUsesProjectFFIType(node.Elt, projectAlias)
	case *ast.MapType:
		return exprUsesProjectFFIType(node.Key, projectAlias) || exprUsesProjectFFIType(node.Value, projectAlias)
	case *ast.IndexExpr:
		return exprUsesProjectFFIType(node.X, projectAlias) || exprUsesProjectFFIType(node.Index, projectAlias)
	case *ast.IndexListExpr:
		if exprUsesProjectFFIType(node.X, projectAlias) {
			return true
		}
		for _, index := range node.Indices {
			if exprUsesProjectFFIType(index, projectAlias) {
				return true
			}
		}
		return false
	case *ast.ParenExpr:
		return exprUsesProjectFFIType(node.X, projectAlias)
	case *ast.SelectorExpr:
		ident, ok := node.X.(*ast.Ident)
		return ok && ident.Name == projectAlias
	default:
		return false
	}
}

func projectFFIPackageAlias(projectInfo *checker.ProjectInfo) string {
	name := "project"
	if projectInfo != nil {
		name = sanitizeName(projectInfo.ProjectName)
	}
	if !token.IsIdentifier(name) {
		return "project"
	}
	return name
}

func projectFFIImportPath(projectInfo *checker.ProjectInfo) string {
	return "generated/" + projectFFIPackageAlias(projectInfo)
}

func registerProjectFFIImports(imports map[string]string, projectInfo *checker.ProjectInfo) {
	imports[projectFFIPackageAlias(projectInfo)] = projectFFIImportPath(projectInfo)
}

func projectHasFFICompanions(projectInfo *checker.ProjectInfo) bool {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return false
	}
	if fileExists(filepath.Join(projectInfo.RootPath, "ffi.go")) {
		return true
	}
	matches, err := filepath.Glob(filepath.Join(projectInfo.RootPath, "ffi", "*.go"))
	return err == nil && len(matches) > 0
}

func externModuleIsStdlib(program *air.Program, ext air.Extern) bool {
	if program != nil && int(ext.Module) >= 0 && int(ext.Module) < len(program.Modules) && strings.HasPrefix(program.Modules[ext.Module].Path, "ard/") {
		return true
	}
	return stdlibGoBinding(goExternBinding(ext))
}

func goExternBinding(ext air.Extern) string {
	if binding := ext.Bindings["go"]; binding != "" {
		return binding
	}
	return ext.Name
}

var stdlibGoBindings = func() map[string]struct{} {
	bindings := map[string]struct{}{}
	for binding := range stdlibffi.HostFunctions {
		bindings[binding] = struct{}{}
	}
	return bindings
}()

func stdlibGoBinding(binding string) bool {
	if _, ok := stdlibGoBindings[binding]; ok {
		return true
	}
	_, ok := generatedStdlibExternLowerings[binding]
	return ok
}

func writeArdModuleDependency(dir string) (string, error) {
	if releaseVersion := strings.TrimSpace(version.Get()); releaseVersion != "" && releaseVersion != "dev" {
		moduleDir := filepath.Join(dir, ".ard", "ard-module")
		if err := writeEmbeddedArdModule(moduleDir); err != nil {
			return "", err
		}
		return "\nrequire github.com/akonwi/ard v0.0.0\nreplace github.com/akonwi/ard => ./.ard/ard-module\n", nil
	}
	if moduleRoot, ok := compilerModuleRoot(); ok {
		return fmt.Sprintf("\nrequire github.com/akonwi/ard v0.0.0\nreplace github.com/akonwi/ard => %s\n", moduleRoot), nil
	}
	return "", nil
}

func writeEmbeddedArdModule(dir string) error {
	for rel, content := range embeddedArdModuleFiles {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func compilerModuleRoot() (string, bool) {
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		return "", false
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
	if strings.TrimSpace(root) == "" {
		return "", false
	}
	return root, true
}

func buildGeneratedProgram(dir string, outputPath string) error {
	cmd := exec.Command("go", "build", "-tags=goexperiment.jsonv2", "-mod=mod", "-o", outputPath, ".")
	cmd.Dir = dir
	cmd.Env = appendGoExperimentJSONv2(os.Environ())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func appendGoExperimentJSONv2(env []string) []string {
	for i, entry := range env {
		if !strings.HasPrefix(entry, "GOEXPERIMENT=") {
			continue
		}
		current := strings.TrimPrefix(entry, "GOEXPERIMENT=")
		if current == "" {
			env[i] = "GOEXPERIMENT=jsonv2"
			return env
		}
		for _, experiment := range strings.Split(current, ",") {
			if experiment == "jsonv2" {
				return env
			}
		}
		env[i] = entry + ",jsonv2"
		return env
	}
	return append(env, "GOEXPERIMENT=jsonv2")
}

func programArgs(args []string) []string {
	if len(args) <= 3 {
		return nil
	}
	return append([]string(nil), args[3:]...)
}

func defaultPackageName(name string) string {
	if name == "" {
		return "main"
	}
	return name
}

func rootFunction(program *air.Program) (air.FunctionID, error) {
	if rootID, ok := findRootFunction(program); ok {
		return rootID, nil
	}
	return air.NoFunction, fmt.Errorf("AIR program has no entry or script function")
}

func findRootFunction(program *air.Program) (air.FunctionID, bool) {
	if program == nil {
		return air.NoFunction, false
	}
	if program.Entry != air.NoFunction {
		return program.Entry, true
	}
	if program.Script != air.NoFunction {
		return program.Script, true
	}
	return air.NoFunction, false
}
