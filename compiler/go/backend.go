package gotarget

import (
	"encoding/json"
	"fmt"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/stdlibgo"
	"github.com/akonwi/ard/version"
	"golang.org/x/mod/modfile"
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
	info := optionalProjectInfo(projectInfo)
	workspaceDir, err := artifactWorkspace(inputPathFromCLIArgs(args), "run")
	if err != nil {
		return err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: info}); err != nil {
		return err
	}
	binaryPath := runBinaryPath(workspaceDir, info)
	if err := os.MkdirAll(filepath.Dir(binaryPath), 0o755); err != nil {
		return err
	}
	if err := buildGeneratedProgram(workspaceDir, binaryPath, goBuildTags(info)...); err != nil {
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
	info := optionalProjectInfo(projectInfo)
	workspaceDir, err := artifactWorkspace(outputPath, "build")
	if err != nil {
		return "", err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: info}); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(workspaceDir, absOutput, goBuildTags(info)...); err != nil {
		return "", err
	}
	return absOutput, nil
}

func RunTests(program *air.Program, args []string, tests []TestCase, failFast bool, projectInfo ...*checker.ProjectInfo) ([]TestOutcome, error) {
	info := optionalProjectInfo(projectInfo)
	workspaceDir, err := artifactWorkspace(inputPathFromCLIArgs(args), "test")
	if err != nil {
		return nil, err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: info, SuppressMain: true, IncludeTests: true}); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(workspaceDir, "ard_tests.go"), []byte(renderTestRunner(program, tests, failFast, info)), 0o644); err != nil {
		return nil, err
	}
	binaryPath := filepath.Join(workspaceDir, "ard-tests")
	if err := buildGeneratedProgram(workspaceDir, binaryPath, goBuildTags(info)...); err != nil {
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

func writeImportSpec(b *strings.Builder, alias string, defaultAlias string, importPath string) {
	if alias == "" {
		alias = defaultAlias
	}
	if alias == defaultAlias {
		fmt.Fprintf(b, "\t%q\n", importPath)
		return
	}
	fmt.Fprintf(b, "\t%s %q\n", alias, importPath)
}

type testRunnerImports struct {
	std     map[string]string
	modules map[air.ModuleID]string
}

func testRunnerImportAliases(program *air.Program, tests []TestCase) testRunnerImports {
	imports := testRunnerImports{std: map[string]string{}, modules: map[air.ModuleID]string{}}
	used := testRunnerReservedTopLevelNames(program)
	for _, base := range []string{"json", "fmt", "os"} {
		alias := base
		for i := 1; used[alias]; i++ {
			alias = fmt.Sprintf("%s_%d", base, i)
		}
		imports.std[base] = alias
		used[alias] = true
	}
	if program == nil {
		return imports
	}
	for _, test := range tests {
		if test.Function < 0 || int(test.Function) >= len(program.Functions) {
			continue
		}
		// Every module is its own package now, so the test runner (the sole
		// `package main`) imports and qualifies all test functions (ADR 0031).
		moduleID := program.Functions[test.Function].Module
		if _, ok := imports.modules[moduleID]; ok {
			continue
		}
		base := modulePackageName(program, moduleID)
		alias := base
		for i := 1; used[alias]; i++ {
			alias = fmt.Sprintf("%s_%d", base, i)
		}
		imports.modules[moduleID] = alias
		used[alias] = true
	}
	return imports
}

func testRunnerReservedTopLevelNames(program *air.Program) map[string]bool {
	reserved := map[string]bool{"main": true, "ardRunTest": true, "ardTestOutcome": true}
	if program == nil {
		return reserved
	}
	traitLowerer := &lowerer{program: program}
	for _, typ := range program.Types {
		reserved[typeName(program, typ)] = true
		for _, variant := range typ.Variants {
			reserved[enumVariantName(program, typ, variant)] = true
		}
	}
	for _, trait := range program.Traits {
		reserved[traitLowerer.traitInterfaceTypeName(trait)] = true
	}
	for _, global := range program.Globals {
		reserved[globalName(program, global)] = true
	}
	for _, fn := range program.Functions {
		reserved[functionName(program, fn)] = true
	}
	return reserved
}

func renderTestRunner(program *air.Program, tests []TestCase, failFast bool, projectInfo *checker.ProjectInfo) string {
	imports := testRunnerImportAliases(program, tests)
	aliases := imports.std
	var b strings.Builder
	b.WriteString("package main\n\n")
	b.WriteString("import (\n")
	writeImportSpec(&b, aliases["json"], "json", "encoding/json")
	writeImportSpec(&b, aliases["fmt"], "fmt", "fmt")
	writeImportSpec(&b, aliases["os"], "os", "os")
	moduleIDs := make([]int, 0, len(imports.modules))
	for moduleID := range imports.modules {
		moduleIDs = append(moduleIDs, int(moduleID))
	}
	sort.Ints(moduleIDs)
	for _, moduleID := range moduleIDs {
		id := air.ModuleID(moduleID)
		projectName := ""
		if projectInfo != nil {
			projectName = projectInfo.ProjectName
		}
		writeImportSpec(&b, imports.modules[id], modulePackageName(program, id), moduleImportPathForProject(program, id, generatedModulePath(projectInfo), projectName))
	}
	b.WriteString(")\n\n")
	b.WriteString("type ardTestOutcome struct {\n")
	b.WriteString("\tName string `json:\"name\"`\n")
	b.WriteString("\tDisplayName string `json:\"displayName\"`\n")
	b.WriteString("\tStatus string `json:\"status\"`\n")
	b.WriteString("\tMessage string `json:\"message,omitempty\"`\n")
	b.WriteString("}\n\n")
	b.WriteString("func ardRunTest(name string, displayName string, fn func() error) (out ardTestOutcome) {\n")
	b.WriteString("\tout = ardTestOutcome{Name: name, DisplayName: displayName, Status: \"panic\"}\n")
	fmt.Fprintf(&b, "\tdefer func() { if recovered := recover(); recovered != nil { out.Status = \"panic\"; out.Message = %s.Sprint(recovered) } }()\n", aliases["fmt"])
	b.WriteString("\terr := fn()\n")
	b.WriteString("\tif err == nil { out.Status = \"pass\"; out.Message = \"\" } else { out.Status = \"fail\"; out.Message = err.Error() }\n")
	b.WriteString("\treturn out\n")
	b.WriteString("}\n\n")
	b.WriteString("func main() {\n")
	b.WriteString("\toutcomes := []ardTestOutcome{}\n")
	for _, test := range tests {
		if test.Function < 0 || int(test.Function) >= len(program.Functions) {
			continue
		}
		fn := program.Functions[test.Function]
		fnName := functionName(program, fn)
		if alias := imports.modules[fn.Module]; alias != "" {
			fnName = alias + "." + fnName
		}
		fmt.Fprintf(&b, "\toutcomes = append(outcomes, ardRunTest(%s, %s, %s))\n", strconv.Quote(test.Name), strconv.Quote(test.DisplayName), fnName)
		if failFast {
			b.WriteString("\tif outcomes[len(outcomes)-1].Status != \"pass\" { goto done }\n")
		}
	}
	if failFast {
		b.WriteString("done:\n")
	}
	fmt.Fprintf(&b, "\tdata, err := %s.Marshal(outcomes)\n", aliases["json"])
	fmt.Fprintf(&b, "\tif err != nil { %s.Fprintln(%s.Stderr, err); %s.Exit(1) }\n", aliases["fmt"], aliases["os"], aliases["os"])
	fmt.Fprintf(&b, "\tif path := %s.Getenv(\"ARD_TEST_RESULTS\"); path != \"\" {\n", aliases["os"])
	fmt.Fprintf(&b, "\t\tif err := %s.WriteFile(path, data, 0o644); err != nil { %s.Fprintln(%s.Stderr, err); %s.Exit(1) }\n", aliases["os"], aliases["fmt"], aliases["os"], aliases["os"])
	b.WriteString("\t\treturn\n")
	b.WriteString("\t}\n")
	fmt.Fprintf(&b, "\t_, _ = %s.Stdout.Write(data)\n", aliases["os"])
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
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, source, 0o644); err != nil {
			return err
		}
	}
	if err := copyProjectFFIDir(dir, options.ProjectInfo); err != nil {
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
	goMod, err := generatedGoModBase(projectInfo)
	if err != nil {
		return "", err
	}
	ardRequirement, err := writeArdModuleDependency(dir)
	if err != nil {
		return "", err
	}
	requireSeen := requireKeys(goMod)
	requires := make([]string, 0)
	addGoModRequirements(&requires, requireSeen, ardRequirement)
	addDependencyGoModRequirements(&requires, requireSeen, program, projectInfo)
	goMod += formatRequireBlock(requires)

	replaceSeen := replaceKeys(goMod)
	replaces := make([]string, 0)
	addGoModReplaces(&replaces, replaceSeen, ardRequirement)
	addDependencyGoModReplaces(&replaces, replaceSeen, program, projectInfo)
	goMod += formatReplaceBlock(replaces)
	return goMod, nil
}

func generatedModulePath(projectInfo *checker.ProjectInfo) string {
	if module := projectGoModuleName(projectInfo); module != "" {
		return module
	}
	if projectInfo != nil && strings.TrimSpace(projectInfo.ProjectName) != "" {
		return projectInfo.ProjectName
	}
	return "generated"
}

func generatedGoModBase(projectInfo *checker.ProjectInfo) (string, error) {
	if projectInfo != nil && strings.TrimSpace(projectInfo.RootPath) != "" {
		goModPath := filepath.Join(projectInfo.RootPath, "go.mod")
		data, err := os.ReadFile(goModPath)
		if err == nil {
			rewritten, err := rewriteRelativeReplaces(data, projectInfo.RootPath)
			if err != nil {
				return "", err
			}
			return string(rewritten), nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if strings.TrimSpace(projectInfo.ProjectName) != "" {
			return fmt.Sprintf("module %s\n\ngo 1.26.0\n", projectInfo.ProjectName), nil
		}
	}
	return "module generated\n\ngo 1.26.0\n", nil
}

func rewriteRelativeReplaces(data []byte, projectRoot string) ([]byte, error) {
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	type replacementRewrite struct {
		oldPath    string
		oldVersion string
		newPath    string
	}
	rewrites := []replacementRewrite{}
	for _, replace := range file.Replace {
		if replace.New.Version != "" || !isRelativeLocalReplacePath(replace.New.Path) {
			continue
		}
		abs, err := filepath.Abs(filepath.Join(projectRoot, replace.New.Path))
		if err != nil {
			return nil, err
		}
		rewrites = append(rewrites, replacementRewrite{oldPath: replace.Old.Path, oldVersion: replace.Old.Version, newPath: abs})
	}
	for _, rewrite := range rewrites {
		if err := file.DropReplace(rewrite.oldPath, rewrite.oldVersion); err != nil {
			return nil, err
		}
		if err := file.AddReplace(rewrite.oldPath, rewrite.oldVersion, rewrite.newPath, ""); err != nil {
			return nil, err
		}
	}
	return file.Format()
}

func isRelativeLocalReplacePath(path string) bool {
	return strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") || path == "." || path == ".."
}

func addDependencyGoModRequirements(out *[]string, seen map[string]bool, program *air.Program, projectInfo *checker.ProjectInfo) {
	for _, root := range dependencyGoModPackages(program, projectInfo) {
		addGoModRequirementsFromFile(out, seen, filepath.Join(root, "go.mod"))
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

// projectGoModuleName returns the module path declared in the project's go.mod,
// or "" if the project has no Go module. This is the module that owns the
func projectGoModuleName(projectInfo *checker.ProjectInfo) string {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(projectInfo.RootPath, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func addDependencyGoModReplaces(out *[]string, seen map[string]bool, program *air.Program, projectInfo *checker.ProjectInfo) {
	for _, root := range dependencyGoModPackages(program, projectInfo) {
		addGoModReplacesFromFile(out, seen, filepath.Join(root, "go.mod"), root)
	}
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
		addGoModReplace(out, seen, normalized)
	}
}

func addGoModReplaces(out *[]string, seen map[string]bool, goMod string) {
	for _, replace := range extractReplaceLines(goMod) {
		addGoModReplace(out, seen, replace)
	}
}

func addGoModReplace(out *[]string, seen map[string]bool, replace string) {
	key := replaceKey(replace)
	if key == "" || seen[key] {
		return
	}
	seen[key] = true
	*out = append(*out, replace)
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
	if projectInfo != nil && strings.TrimSpace(projectInfo.RootPath) != "" {
		addGoSumLines(&lines, seen, filepath.Join(projectInfo.RootPath, "go.sum"))
	}
	for _, root := range dependencyGoModPackages(program, projectInfo) {
		addGoSumLines(&lines, seen, filepath.Join(root, "go.sum"))
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
	return dep.SourcePath
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dependencyGoModPackages(program *air.Program, projectInfo *checker.ProjectInfo) map[string]string {
	return map[string]string{}
}

func dependencyAliasForModulePath(modulePath string, projectInfo *checker.ProjectInfo) (string, bool) {
	key, _, ok := dependencyPackageForModulePath(modulePath, projectInfo)
	return key, ok
}

func dependencyPackageForModulePath(modulePath string, projectInfo *checker.ProjectInfo) (string, string, bool) {
	if projectInfo == nil || modulePath == "" {
		return "", "", false
	}
	first := strings.Split(modulePath, "/")[0]
	for _, dep := range projectInfo.Dependencies {
		packageID := dep.PackageID
		if packageID == "" {
			packageID = dep.Alias
		}
		key := checker.PackageModulePrefix(packageID)
		if first == key || first == dep.Alias {
			return key, dependencyRootPath(dep), true
		}
	}
	for packageID, pkg := range projectInfo.Packages {
		if packageID == projectInfo.RootPackageID || packageID == "" {
			continue
		}
		key := checker.PackageModulePrefix(packageID)
		if first == key {
			return key, pkg.RootPath, true
		}
	}
	return "", "", false
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

func copyProjectFFIDir(outputDir string, projectInfo *checker.ProjectInfo) error {
	if projectInfo == nil || strings.TrimSpace(projectInfo.RootPath) == "" {
		return nil
	}
	source := filepath.Join(projectInfo.RootPath, "ffi")
	info, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("project ffi path is not a directory: %s", source)
	}
	return copyDir(source, filepath.Join(outputDir, "ffi"))
}

func copyDir(source string, dest string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dest, rel), 0o755)
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func writeEmbeddedArdModule(dir string) error {
	for rel, content := range stdlibgo.Files {
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

func buildGeneratedProgram(dir string, outputPath string, buildTags ...string) error {
	// The generated output imports encoding/json/v2 (union marshalling), so
	// the jsonv2 experiment tag is part of the output contract and always
	// applied here, regardless of caller or environment. The checker's
	// go/packages resolution applies the same tag (checker.JSONV2BuildTag)
	// so both sides see one build configuration.
	tags := []string{checker.JSONV2BuildTag}
	for _, tag := range buildTags {
		if tag != checker.JSONV2BuildTag {
			tags = append(tags, tag)
		}
	}
	args := []string{"build", "-mod=mod", "-o", outputPath, "-tags=" + strings.Join(tags, ",")}
	args = append(args, ".")
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func goBuildTags(projectInfo *checker.ProjectInfo) []string {
	if projectInfo == nil || len(projectInfo.Go.BuildTags) == 0 {
		return nil
	}
	return append([]string(nil), projectInfo.Go.BuildTags...)
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
	sanitized := sanitizeGoIdentifier(name)
	if sanitized == "" || sanitized == "_" {
		return "main"
	}
	if token.Lookup(sanitized) != token.IDENT {
		return sanitized + "_"
	}
	return sanitized
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
