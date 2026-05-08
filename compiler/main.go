package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/frontend"
	gotarget "github.com/akonwi/ard/go"
	"github.com/akonwi/ard/javascript"
	"github.com/akonwi/ard/version"
	vm "github.com/akonwi/ard/vm"
)

const vmFooterMarker = "ARDVMv001"

func main() {
	if maybeRunEmbedded() {
		return
	}
	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println(version.Get())
		os.Exit(0)
	case "check":
		{
			if len(os.Args) < 3 {
				fmt.Println("Expected filepath argument")
				os.Exit(1)
			}

			inputPath := os.Args[2]
			if !check(inputPath) {
				os.Exit(1)
			}

			fmt.Println("✅ No errors found")
			os.Exit(0)
		}
	case "run":
		{
			inputPath, requestedTarget, err := parseRunArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			target, err := resolveTarget(inputPath, requestedTarget)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			switch target {
			case backend.TargetVM:
				profile := newPipelineProfile("run vm")
				defer profile.Print()
				var module checker.Module
				if err := profile.Time("frontend.load_module", func() error {
					var loadErr error
					module, loadErr = loadModule(inputPath, target)
					return loadErr
				}); err != nil {
					os.Exit(1)
				}
				var program *air.Program
				if err := profile.Time("air.lower", func() error {
					var lowerErr error
					program, lowerErr = air.Lower(module)
					return lowerErr
				}); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := runVMProgramProfile(program, os.Args, profile); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case backend.TargetGo:
				profile := newPipelineProfile("run go")
				defer profile.Print()
				var module checker.Module
				if err := profile.Time("frontend.load_module", func() error {
					var loadErr error
					module, loadErr = loadModule(inputPath, target)
					return loadErr
				}); err != nil {
					os.Exit(1)
				}
				var program *air.Program
				if err := profile.Time("air.lower", func() error {
					var lowerErr error
					program, lowerErr = air.Lower(module)
					return lowerErr
				}); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
				if err := gotarget.RunProgram(program, os.Args); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case backend.TargetJSBrowser, backend.TargetJSServer:
				if err := runJSProgram(inputPath, target, os.Args); err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			default:
				fmt.Printf("unknown target: %s\n", target)
				os.Exit(1)
			}
		}
	case "build":
		{
			inputPath, outputPath, requestedTarget, err := parseBuildArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			target, err := resolveTarget(inputPath, requestedTarget)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			var builtPath string
			switch target {
			case backend.TargetVM:
				builtPath, err = buildVMBinary(inputPath, outputPath)
			case backend.TargetGo:
				builtPath, err = buildGoBinary(inputPath, outputPath, target)
			case backend.TargetJSBrowser, backend.TargetJSServer:
				builtPath, err = buildJSProgram(inputPath, outputPath, target)
			default:
				err = fmt.Errorf("unknown target: %s", target)
			}
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			fmt.Printf("Built %s\n", builtPath)
		}
	case "test":
		{
			inputPath, filter, failFast, err := parseTestArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if !runTests(inputPath, filter, failFast) {
				os.Exit(1)
			}
			os.Exit(0)
		}
	case "format":
		{
			inputPath, checkOnly, err := parseFormatArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			changedPaths, err := formatPath(inputPath, checkOnly)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if checkOnly {
				if len(changedPaths) > 0 {
					fmt.Println("files with format errors:")
					for _, changedPath := range changedPaths {
						fmt.Println(changedPath)
					}
					os.Exit(1)
				}
				os.Exit(0)
			}
			if len(changedPaths) > 0 {
				os.Exit(0)
			}
			os.Exit(0)
		}
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func check(inputPath string) bool {
	_, err := loadModule(inputPath, "")
	return err == nil
}

func loadModule(inputPath string, target string) (checker.Module, error) {
	result, err := frontend.LoadModule(inputPath, target)
	if err != nil {
		return nil, err
	}
	return result.Module, nil
}

func parseRunArgs(args []string) (string, string, error) {
	inputPath := ""
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--target" {
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--target requires a value")
			}
			parsedTarget, err := backend.ParseTarget(args[i+1])
			if err != nil {
				return "", "", err
			}
			target = parsedTarget
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
	}
	if inputPath == "" {
		return "", "", fmt.Errorf("expected filepath argument")
	}
	return inputPath, target, nil
}

func parseBuildArgs(args []string) (string, string, string, error) {
	inputPath := ""
	outputPath := ""
	target := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--out" {
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--out requires a path")
			}
			outputPath = args[i+1]
			i++
			continue
		}
		if arg == "--target" {
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--target requires a value")
			}
			parsedTarget, err := backend.ParseTarget(args[i+1])
			if err != nil {
				return "", "", "", err
			}
			target = parsedTarget
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", "", "", fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
	}
	if inputPath == "" {
		return "", "", "", fmt.Errorf("expected filepath argument")
	}
	if outputPath == "" {
		outputPath = filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath)))
		if outputPath == "" || outputPath == "." || outputPath == string(filepath.Separator) {
			outputPath = "main"
		}
	}
	return inputPath, outputPath, target, nil
}

func resolveTarget(inputPath, requestedTarget string) (string, error) {
	return frontend.ResolveTarget(inputPath, requestedTarget)
}

func parseFormatArgs(args []string) (string, bool, error) {
	inputPath := ""
	checkOnly := false
	for i := range args {
		arg := args[i]
		if arg == "--check" {
			checkOnly = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return "", false, fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
		return "", false, fmt.Errorf("unexpected argument: %s", arg)
	}
	if inputPath == "" {
		return "", false, fmt.Errorf("expected filepath argument")
	}
	return inputPath, checkOnly, nil
}

func parseTestArgs(args []string) (string, string, bool, error) {
	inputPath := "."
	filter := ""
	failFast := false
	seenPath := false

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--fail-fast":
			failFast = true
		case "--filter":
			if i+1 >= len(args) {
				return "", "", false, fmt.Errorf("--filter requires a value")
			}
			filter = args[i+1]
			i++
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", false, fmt.Errorf("unknown flag: %s", arg)
			}
			if seenPath {
				return "", "", false, fmt.Errorf("unexpected argument: %s", arg)
			}
			inputPath = arg
			seenPath = true
		}
	}

	return inputPath, filter, failFast, nil
}

func formatPath(inputPath string, checkOnly bool) ([]string, error) {
	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading path %s - %w", inputPath, err)
	}

	if !fileInfo.IsDir() {
		changed, err := formatFile(inputPath, checkOnly)
		if err != nil {
			return nil, err
		}
		if changed {
			return []string{inputPath}, nil
		}
		return nil, nil
	}

	ardFiles := make([]string, 0)
	err = filepath.WalkDir(inputPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) == ".ard" {
			ardFiles = append(ardFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error walking directory %s - %w", inputPath, err)
	}

	changedPaths := make([]string, 0)
	for _, filePath := range ardFiles {
		changed, fileErr := formatFile(filePath, checkOnly)
		if fileErr != nil {
			return nil, fileErr
		}
		if changed {
			changedPaths = append(changedPaths, filePath)
		}
	}
	return changedPaths, nil
}

func formatFile(inputPath string, checkOnly bool) (bool, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		return false, fmt.Errorf("error reading file %s - %w", inputPath, err)
	}

	formatted, err := formatter.Format(sourceCode, inputPath)
	if err != nil {
		return false, fmt.Errorf("error formatting file %s - %w", inputPath, err)
	}
	changed := !bytes.Equal(sourceCode, formatted)
	if !changed || checkOnly {
		return changed, nil
	}

	fileInfo, err := os.Stat(inputPath)
	if err != nil {
		return false, fmt.Errorf("error reading file info %s - %w", inputPath, err)
	}

	if err := os.WriteFile(inputPath, formatted, fileInfo.Mode()); err != nil {
		return false, fmt.Errorf("error writing file %s - %w", inputPath, err)
	}

	return true, nil
}

type discoveredTest struct {
	filePath    string
	displayPath string
	name        string
}

type testStatus string

const (
	testPass  testStatus = "pass"
	testFail  testStatus = "fail"
	testPanic testStatus = "panic"
)

func (s testStatus) symbol() string {
	switch s {
	case testPass:
		return "✓"
	case testFail:
		return "✗"
	case testPanic:
		return "💥"
	default:
		return "?"
	}
}

type testOutcome struct {
	test    discoveredTest
	status  testStatus
	message string
}

func runTests(inputPath, filter string, failFast bool) bool {
	files, err := discoverTestFiles(inputPath)
	if err != nil {
		fmt.Println(err)
		return false
	}

	outcomes := make([]testOutcome, 0)
	for _, path := range files {
		module, err := loadModule(path, "")
		if err != nil {
			return false
		}
		tests := collectTests(module, path, filter)
		if len(tests) == 0 {
			continue
		}

		program, err := air.Lower(module)
		if err != nil {
			fmt.Println(err)
			return false
		}
		if err := air.Validate(program); err != nil {
			fmt.Println(err)
			return false
		}
		vm, err := vm.NewWithOptions(program, vm.Options{Args: os.Args})
		if err != nil {
			fmt.Println(err)
			return false
		}

		for _, test := range tests {
			outcome := runCompiledTest(vm, test)
			outcomes = append(outcomes, outcome)
			reportTestOutcome(outcome)
			if failFast && outcome.status != testPass {
				reportTestSummary(outcomes)
				return false
			}
		}
	}

	if len(outcomes) == 0 {
		fmt.Println("No tests found")
		return true
	}

	reportTestSummary(outcomes)
	for _, outcome := range outcomes {
		if outcome.status != testPass {
			return false
		}
	}
	return true
}

func stdlibModulePathForTestFile(root, filePath string) (string, bool) {
	root = filepath.Clean(root)
	filePath = filepath.Clean(filePath)
	if filepath.Base(root) != "std_lib" || filepath.Ext(filePath) != ".ard" {
		return "", false
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(strings.TrimSuffix(rel, ".ard"))
	if rel == "" || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return "ard/" + rel, true
}

func discoverTestFiles(inputPath string) ([]string, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("error reading path %s - %w", inputPath, err)
	}

	if !info.IsDir() {
		if filepath.Ext(inputPath) != ".ard" {
			return nil, fmt.Errorf("expected an .ard file or directory: %s", inputPath)
		}
		return []string{filepath.Clean(inputPath)}, nil
	}

	files := make([]string, 0)
	seen := make(map[string]struct{})
	err = filepath.WalkDir(inputPath, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != inputPath {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".ard" {
			return nil
		}
		if modulePath, ok := stdlibModulePathForTestFile(inputPath, path); ok {
			if err := checker.ValidateStdlibImportTarget(modulePath, backend.DefaultTarget); err != nil {
				return nil
			}
		}
		cleaned := filepath.Clean(path)
		if _, ok := seen[cleaned]; ok {
			return nil
		}
		seen[cleaned] = struct{}{}
		files = append(files, cleaned)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func collectTests(module checker.Module, filePath string, filter string) []discoveredTest {
	program := module.Program()
	if program == nil {
		return nil
	}

	displayPath := strings.TrimSuffix(filepath.Clean(filePath), filepath.Ext(filePath))
	tests := make([]discoveredTest, 0)
	for _, stmt := range program.Statements {
		fn, ok := stmt.Expr.(*checker.FunctionDef)
		if !ok || !fn.IsTest {
			continue
		}
		test := discoveredTest{
			filePath:    filePath,
			displayPath: displayPath,
			name:        fn.Name,
		}
		if filter != "" && !strings.Contains(test.displayName(), filter) {
			continue
		}
		tests = append(tests, test)
	}
	return tests
}

func (t discoveredTest) displayName() string {
	return fmt.Sprintf("%s::%s", t.displayPath, t.name)
}

func runCompiledTest(machine *vm.VM, test discoveredTest) testOutcome {
	result := machine.RunNamedTest(test.name)
	outcome := testOutcome{test: test, message: result.Message}
	switch result.Status {
	case vm.TestPass:
		outcome.status = testPass
	case vm.TestFail:
		outcome.status = testFail
	case vm.TestPanic:
		outcome.status = testPanic
	default:
		outcome.status = testPanic
		if outcome.message == "" {
			outcome.message = fmt.Sprintf("unknown test status: %s", result.Status)
		}
	}
	return outcome
}

func reportTestOutcome(outcome testOutcome) {
	fmt.Printf("%s  %s\n", outcome.status.symbol(), outcome.test.displayName())
	if outcome.message != "" && outcome.status != testPass {
		fmt.Printf("  %s\n", outcome.message)
	}
}

func reportTestSummary(outcomes []testOutcome) {
	passed := 0
	failed := 0
	panicked := 0
	for _, outcome := range outcomes {
		switch outcome.status {
		case testPass:
			passed++
		case testFail:
			failed++
		case testPanic:
			panicked++
		}
	}
	fmt.Printf("\n%d passed; %d failed; %d panicked\n", passed, failed, panicked)
}

func runJSProgram(inputPath string, target string, args []string) error {
	profile := newPipelineProfile("run javascript")
	defer profile.Print()
	var loaded *frontend.LoadResult
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		loaded, loadErr = frontend.LoadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(loaded.Module)
		return lowerErr
	}); err != nil {
		return err
	}
	if err := profile.Time("javascript.run", func() error {
		return javascript.RunProgram(program, target, args, loaded.ProjectInfo)
	}); err != nil {
		return err
	}
	return nil
}

func buildJSProgram(inputPath string, outputPath string, target string) (string, error) {
	profile := newPipelineProfile("build javascript")
	defer profile.Print()
	var loaded *frontend.LoadResult
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		loaded, loadErr = frontend.LoadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(loaded.Module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	var builtPath string
	if err := profile.Time("javascript.build", func() error {
		var buildErr error
		builtPath, buildErr = javascript.BuildProgram(program, outputPath, target, loaded.ProjectInfo)
		return buildErr
	}); err != nil {
		return "", err
	}
	return builtPath, nil
}

func buildGoBinary(inputPath string, outputPath string, target string) (string, error) {
	profile := newPipelineProfile("build go")
	defer profile.Print()
	var module checker.Module
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		module, loadErr = loadModule(inputPath, target)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	if err := profile.Time("air.validate", func() error {
		return air.Validate(program)
	}); err != nil {
		return "", err
	}
	if outputPath == "" {
		outputPath = filepath.Base(strings.TrimSuffix(inputPath, filepath.Ext(inputPath)))
		if outputPath == "" || outputPath == "." || outputPath == string(filepath.Separator) {
			outputPath = "main"
		}
	}
	var builtPath string
	if err := profile.Time("go.build", func() error {
		var buildErr error
		builtPath, buildErr = gotarget.BuildProgram(program, outputPath)
		return buildErr
	}); err != nil {
		return "", err
	}
	return builtPath, nil
}

func buildVMBinary(inputPath string, outputPath string) (string, error) {
	profile := newPipelineProfile("build vm")
	defer profile.Print()
	var module checker.Module
	if err := profile.Time("frontend.load_module", func() error {
		var loadErr error
		module, loadErr = loadModule(inputPath, backend.TargetVM)
		return loadErr
	}); err != nil {
		return "", err
	}
	var program *air.Program
	if err := profile.Time("air.lower", func() error {
		var lowerErr error
		program, lowerErr = air.Lower(module)
		return lowerErr
	}); err != nil {
		return "", err
	}
	if err := profile.Time("air.validate", func() error {
		return air.Validate(program)
	}); err != nil {
		return "", err
	}
	if program.Entry == air.NoFunction {
		return "", fmt.Errorf("vm builds require fn main()")
	}
	var data []byte
	if err := profile.Time("air.serialize", func() error {
		var serializeErr error
		data, serializeErr = air.SerializeProgram(program)
		return serializeErr
	}); err != nil {
		return "", err
	}
	selfPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := profile.Time("embed.write_binary", func() error {
		return writeEmbeddedBinary(selfPath, outputPath, vmFooterMarker, data)
	}); err != nil {
		return "", err
	}
	return outputPath, nil
}

func argsForEmbeddedProgram(args []string) []string {
	if len(args) > 1 && args[1] == "run-embedded" {
		out := make([]string, 0, len(args)-1)
		out = append(out, args[0])
		out = append(out, args[2:]...)
		return out
	}
	return append([]string(nil), args...)
}

func maybeRunEmbedded() bool {
	marker, data, err := readEmbeddedPayload()
	if err != nil || data == nil {
		return false
	}
	switch marker {
	case vmFooterMarker:
		profile := newPipelineProfile("embedded vm")
		defer profile.Print()
		var program *air.Program
		if err := profile.Time("air.deserialize", func() error {
			var deserializeErr error
			program, deserializeErr = air.DeserializeProgram(data)
			return deserializeErr
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Failed to deserialize vm AIR:", err)
			os.Exit(1)
		}
		if err := profile.Time("air.validate", func() error {
			return air.Validate(program)
		}); err != nil {
			fmt.Fprintln(os.Stderr, "Invalid vm AIR:", err)
			os.Exit(1)
		}
		if runErr := runVMProgramProfile(program, argsForEmbeddedProgram(os.Args), profile); runErr != nil {
			fmt.Fprintln(os.Stderr, runErr)
			os.Exit(1)
		}
	default:
		return false
	}
	return true
}

func runVMProgram(program *air.Program, args []string) error {
	return runVMProgramProfile(program, args, nil)
}

func runVMProgramProfile(program *air.Program, args []string, profile *pipelineProfile) error {
	var machine *vm.VM
	if err := profile.Time("vm.init", func() error {
		var initErr error
		machine, initErr = vm.NewWithOptions(program, vm.Options{Args: args})
		return initErr
	}); err != nil {
		return err
	}
	var err error
	runErr := profile.Time("vm.run", func() error {
		if program.Entry != air.NoFunction {
			_, err = machine.RunEntry()
			return err
		}
		_, err = machine.RunScript()
		return err
	})
	if report := machine.ProfileReport(); report != "" {
		fmt.Fprintln(os.Stderr, report)
	}
	return runErr
}

func writeEmbeddedBinary(srcPath string, dstPath string, marker string, data []byte) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	if _, err := dst.Write(data); err != nil {
		return err
	}
	if err := writeFooter(dst, marker, uint64(len(data))); err != nil {
		return err
	}
	return dst.Chmod(0o755)
}

func writeFooter(w io.Writer, marker string, length uint64) error {
	if _, err := w.Write([]byte(marker)); err != nil {
		return err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, length)
	_, err := w.Write(buf)
	return err
}

func readEmbeddedPayload() (string, []byte, error) {
	path, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	return readEmbeddedPayloadFromPath(path)
}

func readEmbeddedPayloadFromPath(path string) (string, []byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", nil, err
	}
	footerSize := int64(len(vmFooterMarker) + 8)
	if info.Size() < footerSize {
		return "", nil, nil
	}
	_, err = file.Seek(info.Size()-footerSize, io.SeekStart)
	if err != nil {
		return "", nil, err
	}
	footer := make([]byte, footerSize)
	if _, err := io.ReadFull(file, footer); err != nil {
		return "", nil, err
	}
	marker := string(footer[:len(vmFooterMarker)])
	if marker != vmFooterMarker {
		return "", nil, nil
	}
	length := binary.LittleEndian.Uint64(footer[len(vmFooterMarker):])
	dataOffset := info.Size() - footerSize - int64(length)
	if dataOffset < 0 {
		return "", nil, fmt.Errorf("invalid embedded payload length")
	}
	if _, err := file.Seek(dataOffset, io.SeekStart); err != nil {
		return "", nil, err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(file, data); err != nil {
		return "", nil, err
	}
	return marker, data, nil
}

const pipelineProfileEnvVar = "ARD_PIPELINE_PROFILE"

type pipelineProfile struct {
	scope   string
	started time.Time
	stages  []pipelineProfileStage
}

type pipelineProfileStage struct {
	name string
	dur  time.Duration
}

func newPipelineProfile(scope string) *pipelineProfile {
	if !pipelineProfilingEnabled() {
		return nil
	}
	return &pipelineProfile{scope: scope, started: time.Now()}
}

func pipelineProfilingEnabled() bool {
	raw, ok := os.LookupEnv(pipelineProfileEnvVar)
	if !ok {
		return false
	}
	raw = strings.TrimSpace(strings.ToLower(raw))
	return raw != "" && raw != "0" && raw != "false" && raw != "off"
}

func (p *pipelineProfile) Time(name string, fn func() error) error {
	if p == nil {
		return fn()
	}
	started := time.Now()
	err := fn()
	p.stages = append(p.stages, pipelineProfileStage{name: name, dur: time.Since(started)})
	return err
}

func (p *pipelineProfile) Print() {
	if p == nil {
		return
	}
	fmt.Fprintln(os.Stderr, p.Report())
}

func (p *pipelineProfile) Report() string {
	if p == nil {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "[ard pipeline profile: %s]\n", p.scope)
	fmt.Fprintf(&out, "total=%s\n", time.Since(p.started).Round(time.Microsecond))
	for _, stage := range p.stages {
		fmt.Fprintf(&out, "%s=%s\n", stage.name, stage.dur.Round(time.Microsecond))
	}
	return strings.TrimRight(out.String(), "\n")
}
