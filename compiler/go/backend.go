package gotarget

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"go/token"
	goversion "go/version"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	runtimesrc "github.com/akonwi/ard/runtime"
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

type StageObserver func(name string, duration time.Duration, err error)

func observeStage(observer StageObserver, name string, fn func() error) error {
	if observer == nil {
		return fn()
	}
	started := time.Now()
	err := fn()
	observer(name, time.Since(started), err)
	return err
}

type artifactPurpose string

const (
	artifactPurposeRun   artifactPurpose = "run"
	artifactPurposeBuild artifactPurpose = "build"
	artifactPurposeTest  artifactPurpose = "test"
)

func GenerateSources(program *air.Program, options Options) (map[string][]byte, error) {
	generated, err := lowerProgram(program, options)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(generated)+1)
	for name, file := range generated {
		source, err := renderFile(file)
		if err != nil {
			return nil, err
		}
		out[name] = source
	}
	if len(program.EmbeddedBlobs) > 0 {
		source, err := generateEmbeddedResourceSource(program)
		if err != nil {
			return nil, err
		}
		out["internal/ardembed/embed.go"] = source
	}
	return out, nil
}

func generateEmbeddedResourceSource(program *air.Program) ([]byte, error) {
	var source strings.Builder
	source.WriteString("package ardembed\n\n")
	if len(program.EmbeddedSets) > 0 {
		source.WriteString("import (\n\t\"embed\"\n\t\"fmt\"\n\t\"io/fs\"\n\t\"unicode/utf8\"\n)\n\n")
	} else {
		source.WriteString("import _ \"embed\"\n\n")
	}
	for _, blob := range program.EmbeddedBlobs {
		if !blob.Direct {
			continue
		}
		fmt.Fprintf(&source, "//go:embed data/%s\n", blob.Digest)
		fmt.Fprintf(&source, "var blob%s string\n\n", blob.Digest)
		fmt.Fprintf(&source, "func Text%s() string { return blob%s }\n\n", blob.Digest, blob.Digest)
		fmt.Fprintf(&source, "func Bytes%s() []byte { return []byte(blob%s) }\n\n", blob.Digest, blob.Digest)
	}
	for _, set := range program.EmbeddedSets {
		fmt.Fprintf(&source, "//go:embed all:sets/%s\n", set.Digest)
		fmt.Fprintf(&source, "var raw%s embed.FS\n\n", set.Digest)
		fmt.Fprintf(&source, "var set%s fs.FS = mustSub(raw%s, \"sets/%s\")\n\n", set.Digest, set.Digest, set.Digest)
		fmt.Fprintf(&source, "func FS%s() fs.FS { return set%s }\n\n", set.Digest, set.Digest)
	}
	if len(program.EmbeddedSets) > 0 {
		source.WriteString(`func mustSub(root fs.FS, path string) fs.FS {
	value, err := fs.Sub(root, path)
	if err != nil { panic(err) }
	return value
}

func ReadFile(root fs.FS, path string) ([]byte, error) {
	data, err := fs.ReadFile(root, path)
	if err != nil { return nil, err }
	return append([]byte(nil), data...), nil
}

func ReadText(root fs.FS, path string) (string, error) {
	data, err := fs.ReadFile(root, path)
	if err != nil { return "", err }
	if !utf8.Valid(data) { return "", &fs.PathError{Op: "read_text", Path: path, Err: fs.ErrInvalid} }
	return string(data), nil
}

func Sub(root fs.FS, path string) (fs.FS, error) {
	info, err := fs.Stat(root, path)
	if err != nil { return nil, err }
	if !info.IsDir() { return nil, &fs.PathError{Op: "sub", Path: path, Err: fmt.Errorf("%w: not a directory", fs.ErrInvalid)} }
	return fs.Sub(root, path)
}

`)
	}
	formatted, err := format.Source([]byte(source.String()))
	if err != nil {
		return nil, fmt.Errorf("format embedded resource package: %w", err)
	}
	return formatted, nil
}

func RunProgram(program *air.Program, args []string, projectInfo ...*checker.ProjectInfo) error {
	info := optionalProjectInfo(projectInfo)
	pathHint := artifactPathHint(info, inputPathFromCLIArgs(args))
	workspaceDir, err := artifactWorkspace(pathHint, artifactPurposeRun)
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
	return cmd.Run()
}

func BuildProgram(program *air.Program, outputPath string, projectInfo ...*checker.ProjectInfo) (string, error) {
	info := optionalProjectInfo(projectInfo)
	if outputPath == "" {
		outputPath = "main"
	}
	absOutput, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}
	pathHint := artifactPathHint(info, outputPath)
	if err := rejectManagedBuildOutput(pathHint, absOutput); err != nil {
		return "", err
	}
	workspaceDir, err := artifactWorkspace(pathHint, artifactPurposeBuild)
	if err != nil {
		return "", err
	}
	if err := writeProgram(workspaceDir, program, Options{PackageName: "main", ProjectInfo: info}); err != nil {
		return "", err
	}
	if err := buildGeneratedProgram(workspaceDir, absOutput, goBuildTags(info)...); err != nil {
		return "", err
	}
	return absOutput, nil
}

func RunTests(program *air.Program, args []string, tests []TestCase, failFast bool, projectInfo ...*checker.ProjectInfo) ([]TestOutcome, error) {
	return RunTestsWithStageObserver(program, args, tests, failFast, nil, projectInfo...)
}

func RunTestsWithStageObserver(program *air.Program, args []string, tests []TestCase, failFast bool, observer StageObserver, projectInfo ...*checker.ProjectInfo) ([]TestOutcome, error) {
	info := optionalProjectInfo(projectInfo)
	pathHint := artifactPathHint(info, inputPathFromCLIArgs(args))
	var workspaceDir string
	if err := observeStage(observer, "go.prepare_workspace", func() error {
		var workspaceErr error
		workspaceDir, workspaceErr = artifactWorkspace(pathHint, artifactPurposeTest)
		return workspaceErr
	}); err != nil {
		return nil, err
	}
	if err := writeProgramWithStageObserver(workspaceDir, program, Options{PackageName: "main", ProjectInfo: info, SuppressMain: true, IncludeTests: true}, observer); err != nil {
		return nil, err
	}
	if err := observeStage(observer, "go.render_write_test_runner", func() error {
		return os.WriteFile(filepath.Join(workspaceDir, "ard_tests.go"), []byte(renderTestRunner(program, tests, failFast, info)), 0o644)
	}); err != nil {
		return nil, err
	}
	binaryPath := filepath.Join(workspaceDir, "ard-tests")
	if err := observeStage(observer, "go.compile_link", func() error {
		return buildGeneratedProgram(workspaceDir, binaryPath, goBuildTags(info)...)
	}); err != nil {
		return nil, err
	}
	resultPath := filepath.Join(workspaceDir, "test-results.json")
	cmd := exec.Command(binaryPath, programArgs(args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = append(os.Environ(), "ARD_TEST_RESULTS="+resultPath)
	if err := observeStage(observer, "test.execute", cmd.Run); err != nil {
		return nil, err
	}
	var outcomes []TestOutcome
	if err := observeStage(observer, "test.read_decode_results", func() error {
		data, err := os.ReadFile(resultPath)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, &outcomes)
	}); err != nil {
		return nil, err
	}
	return outcomes, nil
}

func rejectManagedBuildOutput(pathHint string, outputPath string) error {
	rootDir, err := artifactRootDir(pathHint)
	if err != nil {
		return err
	}
	ardOutRoot := filepath.Join(rootDir, "ard-out")
	managedPaths := []string{
		filepath.Join(ardOutRoot, "go"),
		filepath.Join(ardOutRoot, ".go-module-cache"),
	}
	resolvedOutput, err := resolveExistingPathAncestors(outputPath)
	if err != nil {
		return err
	}
	for _, managedPath := range managedPaths {
		resolvedManagedPath, err := resolveExistingPathAncestors(managedPath)
		if err != nil {
			return err
		}
		if pathWithin(resolvedOutput, resolvedManagedPath) {
			return fmt.Errorf("build output %q is inside backend-managed path %q", outputPath, managedPath)
		}
	}
	return nil
}

func resolveExistingPathAncestors(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	current := filepath.Clean(absolute)
	missing := []string{}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func pathWithin(path string, root string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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
	// The runner is a synthetic package main. Ard declarations live in imported
	// module packages, so they cannot collide with imports here. Reserve only the
	// runner's own declarations and locals plus the predeclared identifiers its
	// generated code uses; a package alias matching one of these would shadow or
	// be shadowed at its use site.
	used := map[string]bool{
		"append":         true,
		"ardRunTest":     true,
		"ardTestOutcome": true,
		"data":           true,
		"displayName":    true,
		"err":            true,
		"error":          true,
		"fn":             true,
		"len":            true,
		"main":           true,
		"name":           true,
		"nil":            true,
		"out":            true,
		"outcomes":       true,
		"path":           true,
		"recover":        true,
		"recovered":      true,
		"string":         true,
	}
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
	return writeProgramWithStageObserver(dir, program, options, nil)
}

func writeProgramWithStageObserver(dir string, program *air.Program, options Options, observer StageObserver) error {
	var sources map[string][]byte
	if err := observeStage(observer, "go.validate_lower_render", func() error {
		var generateErr error
		sources, generateErr = GenerateSources(program, options)
		return generateErr
	}); err != nil {
		return err
	}
	if err := observeStage(observer, "go.write_sources", func() error {
		for name, source := range sources {
			path := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(path, source, 0o644); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	if err := observeStage(observer, "go.write_embedded_resources", func() error {
		return writeEmbeddedResources(dir, program)
	}); err != nil {
		return err
	}
	if err := observeStage(observer, "go.copy_ffi", func() error {
		return copyProjectFFIDir(dir, options.ProjectInfo)
	}); err != nil {
		return err
	}
	if err := observeStage(observer, "go.write_runtime", func() error {
		return writeGeneratedRuntimePackage(dir)
	}); err != nil {
		return err
	}
	if err := observeStage(observer, "go.write_module", func() error {
		goMod, err := generatedGoMod(dir, program, options.ProjectInfo)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
			return err
		}
		return mergeGoSum(dir, program, options.ProjectInfo)
	}); err != nil {
		return err
	}
	return nil
}

func generatedGoMod(dir string, program *air.Program, projectInfo *checker.ProjectInfo) (string, error) {
	goMod, err := generatedGoModBase(projectInfo)
	if err != nil {
		return "", err
	}
	dependencies := sortedDependencyGoModPackages(program, projectInfo)
	goMod, err = dropSelectedDependencyReplaces(goMod, dependencies)
	if err != nil {
		return "", err
	}
	requireSeen := requireKeys(goMod)
	requires := make([]string, 0)
	addDependencyGoModRequirements(&requires, requireSeen, program, projectInfo)
	sort.Strings(requires)
	goMod += formatRequireBlock(requires)

	replaceSeen := replaceKeys(goMod)
	replaces := make([]string, 0)
	addDependencyGoModRootReplaces(&replaces, replaceSeen, program, projectInfo)
	addDependencyGoModReplaces(&replaces, replaceSeen, program, projectInfo)
	sort.Strings(replaces)
	goMod += formatReplaceBlock(replaces)
	return goMod, nil
}

func dropSelectedDependencyReplaces(goMod string, dependencies []dependencyGoModPackage) (string, error) {
	if len(dependencies) == 0 {
		return goMod, nil
	}
	file, err := modfile.Parse("go.mod", []byte(goMod), nil)
	if err != nil {
		return "", err
	}
	selected := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		selected[dependency.modulePath] = true
	}
	changed := false
	for _, replace := range append([]*modfile.Replace(nil), file.Replace...) {
		if !selected[replace.Old.Path] {
			continue
		}
		if err := file.DropReplace(replace.Old.Path, replace.Old.Version); err != nil {
			return "", err
		}
		changed = true
	}
	if !changed {
		return goMod, nil
	}
	file.Cleanup()
	formatted, err := file.Format()
	if err != nil {
		return "", err
	}
	return string(formatted), nil
}

// addDependencyGoModRootReplaces redirects each dependency's Go module to the
// root backing its Ard source: a locked checkout for Git dependencies (#353)
// or the declared source root for path dependencies (#437). This mirrors the
// checker's resolution, keeping type-checking and the build in agreement.
func addDependencyGoModRootReplaces(out *[]string, seen map[string]bool, program *air.Program, projectInfo *checker.ProjectInfo) {
	for _, dependency := range sortedDependencyGoModPackages(program, projectInfo) {
		abs, err := filepath.Abs(dependency.root)
		if err != nil {
			continue
		}
		addGoModReplace(out, seen, fmt.Sprintf("%s => %s", dependency.modulePath, modfile.AutoQuote(abs)))
	}
}

func generatedModulePath(projectInfo *checker.ProjectInfo) string {
	if module := projectGoModuleName(projectInfo); module != "" {
		return module
	}
	preferred := "generated"
	if projectInfo != nil && strings.TrimSpace(projectInfo.ProjectName) != "" {
		preferred = projectInfo.ProjectName
	}
	dependencyPaths := generatedDependencyModulePaths(projectInfo)
	conflicts := func(candidate string) bool {
		for modulePath := range dependencyPaths {
			if checker.GoModulePathsOverlap(candidate, modulePath) {
				return true
			}
		}
		return false
	}
	if !conflicts(preferred) {
		return preferred
	}
	for suffix := 0; ; suffix++ {
		root := "ard-generated"
		if suffix > 0 {
			root += fmt.Sprintf("-%d", suffix)
		}
		candidate := root + ".invalid/project"
		if !conflicts(candidate) {
			return candidate
		}
	}
}

func generatedDependencyModulePaths(projectInfo *checker.ProjectInfo) map[string]bool {
	roots := checker.DependencyGoModuleRoots(projectInfo)
	paths := make(map[string]bool, len(roots))
	for modulePath, root := range roots {
		paths[modulePath] = true
		data, err := os.ReadFile(filepath.Join(root, "go.mod"))
		if err != nil {
			continue
		}
		file, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			continue
		}
		for _, require := range file.Require {
			paths[require.Mod.Path] = true
		}
	}
	return paths
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
		if !os.IsNotExist(err) {
			return "", err
		}
	}
	return fmt.Sprintf("module %s\n\ngo 1.27.0\n", generatedModulePath(projectInfo)), nil
}

func rewriteRelativeReplaces(data []byte, projectRoot string) ([]byte, error) {
	file, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}
	if file.Go == nil || goversion.Compare("go"+file.Go.Version, "go1.27.0") < 0 {
		if err := file.AddGoStmt("1.27.0"); err != nil {
			return nil, err
		}
	}
	if file.Toolchain != nil && file.Toolchain.Name != "default" && goversion.Compare(file.Toolchain.Name, "go1.27.0") < 0 {
		if err := file.AddToolchainStmt("go1.27.0"); err != nil {
			return nil, err
		}
	}
	type replacementRewrite struct {
		oldPath    string
		oldVersion string
		newPath    string
	}
	rewrites := []replacementRewrite{}
	for _, replace := range file.Replace {
		if replace.New.Version != "" || !modfile.IsDirectoryPath(replace.New.Path) || filepath.IsAbs(filepath.FromSlash(replace.New.Path)) {
			continue
		}
		abs, err := checker.ResolveLocalGoModulePath(projectRoot, replace.New.Path)
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

func addDependencyGoModRequirements(out *[]string, seen map[string]bool, program *air.Program, projectInfo *checker.ProjectInfo) {
	dependencies := sortedDependencyGoModPackages(program, projectInfo)
	for _, dependency := range dependencies {
		addGoModRequirementsFromFile(out, seen, filepath.Join(dependency.root, "go.mod"))
	}
	for _, dependency := range dependencies {
		if !seen[dependency.modulePath] {
			seen[dependency.modulePath] = true
			*out = append(*out, dependency.modulePath+" "+checker.DependencyGoModuleVersion(dependency.modulePath))
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
	dependencies := sortedDependencyGoModPackages(program, projectInfo)
	selected := make(map[string]bool, len(dependencies))
	for _, dependency := range dependencies {
		selected[dependency.modulePath] = true
	}
	wildcardReplaced := map[string]bool{}
	for key := range seen {
		if fields := strings.Fields(key); len(fields) == 1 {
			wildcardReplaced[fields[0]] = true
		}
	}
	for _, dependency := range dependencies {
		data, err := os.ReadFile(filepath.Join(dependency.root, "go.mod"))
		if err != nil {
			continue
		}
		file, err := modfile.Parse("go.mod", data, nil)
		if err != nil {
			continue
		}
		sourceWildcards := map[string]bool{}
		for _, replace := range file.Replace {
			if selected[replace.Old.Path] || wildcardReplaced[replace.Old.Path] {
				continue
			}
			normalized, err := formatDependencyGoModReplace(replace, dependency.root)
			if err != nil {
				continue
			}
			addGoModReplace(out, seen, normalized)
			if replace.Old.Version == "" {
				sourceWildcards[replace.Old.Path] = true
			}
		}
		for modulePath := range sourceWildcards {
			wildcardReplaced[modulePath] = true
		}
	}
}

func formatDependencyGoModReplace(replace *modfile.Replace, baseDir string) (string, error) {
	newPath := replace.New.Path
	if replace.New.Version == "" && modfile.IsDirectoryPath(newPath) {
		var err error
		newPath, err = checker.ResolveLocalGoModulePath(baseDir, newPath)
		if err != nil {
			return "", err
		}
	}
	oldSide := modfile.AutoQuote(replace.Old.Path)
	if replace.Old.Version != "" {
		oldSide += " " + replace.Old.Version
	}
	newSide := modfile.AutoQuote(newPath)
	if replace.New.Version != "" {
		newSide += " " + replace.New.Version
	}
	return oldSide + " => " + newSide, nil
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

func replaceKey(replace string) string {
	parts := strings.SplitN(replace, "=>", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[0])
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

// mergeGoSum retains checksums verified in prior generated builds and supplied
// by the consumer module. Dependency go.sum files are not main-module trust
// roots; missing checksums are verified by Go in the generated module.
func mergeGoSum(dir string, program *air.Program, projectInfo *checker.ProjectInfo) error {
	goSumPath := filepath.Join(dir, "go.sum")
	lines := make([]string, 0)
	seen := map[string]bool{}
	addGoSumLines(&lines, seen, goSumPath)
	if projectInfo != nil && strings.TrimSpace(projectInfo.RootPath) != "" {
		addGoSumLines(&lines, seen, filepath.Join(projectInfo.RootPath, "go.sum"))
	}
	if len(lines) == 0 {
		return nil
	}
	sort.Strings(lines)
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

func artifactPathHint(projectInfo *checker.ProjectInfo, fallback string) string {
	if projectInfo != nil && strings.TrimSpace(projectInfo.RootPath) != "" {
		return projectInfo.RootPath
	}
	return fallback
}

func artifactWorkspace(pathHint string, purpose artifactPurpose) (string, error) {
	rootDir, err := artifactRootDir(pathHint)
	if err != nil {
		return "", err
	}
	ardOutRoot := filepath.Join(rootDir, "ard-out")
	artifactRoot := filepath.Join(ardOutRoot, "go")
	moduleCacheRoot := filepath.Join(ardOutRoot, ".go-module-cache")
	workspace := filepath.Join(artifactRoot, string(purpose))
	if err := cacheArtifactGoModuleFiles(artifactRoot, moduleCacheRoot); err != nil {
		return "", err
	}
	preserved, err := readGoModuleFiles(workspace)
	if err != nil {
		return "", err
	}
	cached, err := readGoModuleFiles(filepath.Join(moduleCacheRoot, string(purpose)))
	if err != nil {
		return "", err
	}
	preserved.fillMissing(cached)
	if err := os.RemoveAll(artifactRoot); err != nil {
		return "", err
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return "", err
	}
	if err := preserved.write(workspace); err != nil {
		return "", err
	}
	return workspace, nil
}

func cacheArtifactGoModuleFiles(artifactRoot string, cacheRoot string) error {
	for _, purpose := range []artifactPurpose{artifactPurposeRun, artifactPurposeBuild, artifactPurposeTest} {
		files, err := readGoModuleFiles(filepath.Join(artifactRoot, string(purpose)))
		if err != nil {
			return err
		}
		if files.goMod == nil && files.goSum == nil {
			continue
		}
		cacheDir := filepath.Join(cacheRoot, string(purpose))
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return err
		}
		if err := files.write(cacheDir); err != nil {
			return err
		}
	}
	return nil
}

type goModuleFiles struct {
	goMod []byte
	goSum []byte
}

func (files *goModuleFiles) fillMissing(cached goModuleFiles) {
	if files.goMod == nil {
		files.goMod = cached.goMod
	}
	if files.goSum == nil {
		files.goSum = cached.goSum
	}
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
	return checker.DependencyGoModuleRoots(projectInfo)
}

type dependencyGoModPackage struct {
	modulePath string
	root       string
}

func sortedDependencyGoModPackages(program *air.Program, projectInfo *checker.ProjectInfo) []dependencyGoModPackage {
	packages := dependencyGoModPackages(program, projectInfo)
	ordered := make([]dependencyGoModPackage, 0, len(packages))
	for modulePath, root := range packages {
		ordered = append(ordered, dependencyGoModPackage{modulePath: modulePath, root: root})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].modulePath == ordered[j].modulePath {
			return ordered[i].root < ordered[j].root
		}
		return ordered[i].modulePath < ordered[j].modulePath
	})
	return ordered
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
	dependencyAliases := make([]string, 0, len(projectInfo.Dependencies))
	for alias := range projectInfo.Dependencies {
		dependencyAliases = append(dependencyAliases, alias)
	}
	sort.Strings(dependencyAliases)
	for _, alias := range dependencyAliases {
		dep := projectInfo.Dependencies[alias]
		packageID := dep.PackageID
		if packageID == "" {
			packageID = dep.Alias
		}
		key := checker.PackageModulePrefix(packageID)
		if first == key || first == dep.Alias {
			return key, dependencyRootPath(dep), true
		}
	}
	packageIDs := make([]string, 0, len(projectInfo.Packages))
	for packageID := range projectInfo.Packages {
		packageIDs = append(packageIDs, packageID)
	}
	sort.Strings(packageIDs)
	for _, packageID := range packageIDs {
		if packageID == projectInfo.RootPackageID || packageID == "" {
			continue
		}
		key := checker.PackageModulePrefix(packageID)
		if first == key {
			return key, projectInfo.Packages[packageID].RootPath, true
		}
	}
	return "", "", false
}

func writeEmbeddedResources(outputDir string, program *air.Program) error {
	if program == nil {
		return nil
	}
	dataDir := filepath.Join(outputDir, "internal", "ardembed", "data")
	for _, blob := range program.EmbeddedBlobs {
		if !blob.Direct {
			continue
		}
		if err := os.MkdirAll(dataDir, 0o755); err != nil {
			return fmt.Errorf("create embedded resource directory: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dataDir, blob.Digest), blob.Data, 0o644); err != nil {
			return fmt.Errorf("write embedded resource %s: %w", blob.Digest, err)
		}
	}
	for _, set := range program.EmbeddedSets {
		setRoot := filepath.Join(outputDir, "internal", "ardembed", "sets", set.Digest)
		for _, entry := range set.Entries {
			if entry.Blob < 0 || int(entry.Blob) >= len(program.EmbeddedBlobs) {
				return fmt.Errorf("embedded set %s references invalid blob %d", set.Digest, entry.Blob)
			}
			resourcePath := filepath.Join(setRoot, filepath.FromSlash(entry.Path))
			rel, err := filepath.Rel(setRoot, resourcePath)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
				return fmt.Errorf("embedded set resource escapes staging root: %q", entry.Path)
			}
			if err := os.MkdirAll(filepath.Dir(resourcePath), 0o755); err != nil {
				return fmt.Errorf("create embedded set directory: %w", err)
			}
			if err := os.WriteFile(resourcePath, program.EmbeddedBlobs[entry.Blob].Data, 0o644); err != nil {
				return fmt.Errorf("write embedded set resource %s: %w", entry.Path, err)
			}
		}
	}
	return nil
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

func writeGeneratedRuntimePackage(dir string) error {
	for _, name := range runtimesrc.SourceFileNames {
		content, err := runtimesrc.SourceFiles.ReadFile(name)
		if err != nil {
			return err
		}
		content = bytes.Replace(content, []byte("package runtime"), []byte("package ard"), 1)
		path := filepath.Join(dir, "internal", "ard", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func buildGeneratedProgram(dir string, outputPath string, buildTags ...string) error {
	args := []string{"build", "-mod=mod", "-o", outputPath}
	if len(buildTags) > 0 {
		args = append(args, "-tags="+strings.Join(buildTags, ","))
	}
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
