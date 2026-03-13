package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/ffi"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/parse"
	"github.com/akonwi/ard/runtime"
	"github.com/akonwi/ard/version"
)

const bytecodeFooterMarker = "ARDBYTECODEv1"

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
			inputPath, err := parseRunArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			module, err := loadModule(inputPath)
			if err != nil {
				os.Exit(1)
			}
			program, err := bytecode.NewEmitter().EmitProgram(module)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if err := bytecode.VerifyProgram(program); err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			ffi.SetOSArgs(os.Args)
			_, runErr := bytecodevm.New(program).Run("main")
			ffi.SetOSArgs(nil)
			if runErr != nil {
				fmt.Println(runErr)
				os.Exit(1)
			}
		}
	case "build":
		{
			inputPath, outputPath, err := parseBuildArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			builtPath, err := buildBytecodeBinary(inputPath, outputPath)
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
	_, err := loadModule(inputPath)
	return err == nil
}

func loadModule(inputPath string) (checker.Module, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		fmt.Printf("Error reading file %s - %v\n", inputPath, err)
		return nil, err
	}

	result := parse.Parse(sourceCode, inputPath)
	if len(result.Errors) > 0 {
		result.PrintErrors()
		return nil, fmt.Errorf("parse errors")
	}
	program := result.Program

	workingDir := filepath.Dir(inputPath)
	moduleResolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		log.Fatalf("Error initializing module resolver: %v\n", err)
	}

	// Get relative path for diagnostics
	relPath, err := filepath.Rel(workingDir, inputPath)
	if err != nil {
		relPath = inputPath // fallback to absolute path
	}

	c := checker.New(relPath, program, moduleResolver)
	c.Check()
	if c.HasErrors() {
		for _, diagnostic := range c.Diagnostics() {
			fmt.Println(diagnostic)
		}
		return nil, fmt.Errorf("type errors")
	}

	return c.Module(), nil
}

func parseRunArgs(args []string) (string, error) {
	inputPath := ""
	for i := range args {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			return "", fmt.Errorf("unknown flag: %s", arg)
		}
		if inputPath == "" {
			inputPath = arg
			continue
		}
	}
	if inputPath == "" {
		return "", fmt.Errorf("expected filepath argument")
	}
	return inputPath, nil
}

func parseBuildArgs(args []string) (string, string, error) {
	inputPath := ""
	outputPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--out" {
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--out requires a path")
			}
			outputPath = args[i+1]
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
	if outputPath == "" {
		outputPath = strings.TrimSuffix(inputPath, filepath.Ext(inputPath))
	}
	return inputPath, outputPath, nil
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
	testPass  testStatus = "PASS"
	testFail  testStatus = "FAIL"
	testPanic testStatus = "PANIC"
)

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
		module, err := loadModule(path)
		if err != nil {
			return false
		}
		tests := collectTests(module, path, filter)
		if len(tests) == 0 {
			continue
		}

		program, err := bytecode.NewEmitter().EmitProgram(module)
		if err != nil {
			fmt.Println(err)
			return false
		}
		if err := bytecode.VerifyProgram(program); err != nil {
			fmt.Println(err)
			return false
		}

		for _, test := range tests {
			outcome := runCompiledTest(program, test)
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

func runCompiledTest(program bytecode.Program, test discoveredTest) testOutcome {
	ffi.SetOSArgs(os.Args)
	defer ffi.SetOSArgs(nil)

	res, err := bytecodevm.New(program).Run(test.name)
	if err != nil {
		return testOutcome{test: test, status: testPanic, message: err.Error()}
	}
	if res == nil {
		return testOutcome{test: test, status: testPanic, message: "test returned no result"}
	}
	if !res.IsResult() {
		return testOutcome{test: test, status: testPanic, message: "test did not return a Result"}
	}
	if res.IsErr() {
		return testOutcome{test: test, status: testFail, message: resultMessage(res)}
	}
	return testOutcome{test: test, status: testPass}
}

func resultMessage(res *runtime.Object) string {
	if res == nil {
		return ""
	}
	unwrapped := res.UnwrapResult()
	if msg, ok := unwrapped.IsStr(); ok {
		return msg
	}
	return fmt.Sprintf("%v", unwrapped.GoValue())
}

func reportTestOutcome(outcome testOutcome) {
	fmt.Printf("%s  %s\n", outcome.status, outcome.test.displayName())
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

func buildBytecodeBinary(inputPath string, outputPath string) (string, error) {
	module, err := loadModule(inputPath)
	if err != nil {
		return "", err
	}
	program, err := bytecode.NewEmitter().EmitProgram(module)
	if err != nil {
		return "", err
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		return "", err
	}
	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		return "", err
	}
	selfPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	if err := writeEmbeddedBinary(selfPath, outputPath, data); err != nil {
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
	data, err := readEmbeddedBytecode()
	if err != nil || data == nil {
		return false
	}
	program, err := bytecode.DeserializeProgram(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to deserialize bytecode:", err)
		os.Exit(1)
	}
	if err := bytecode.VerifyProgram(program); err != nil {
		fmt.Fprintln(os.Stderr, "Invalid bytecode:", err)
		os.Exit(1)
	}
	ffi.SetOSArgs(argsForEmbeddedProgram(os.Args))
	_, runErr := bytecodevm.New(program).Run("main")
	ffi.SetOSArgs(nil)
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
	return true
}

func writeEmbeddedBinary(srcPath string, dstPath string, data []byte) error {
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
	if err := writeFooter(dst, uint64(len(data))); err != nil {
		return err
	}
	return dst.Chmod(0o755)
}

func writeFooter(w io.Writer, length uint64) error {
	if _, err := w.Write([]byte(bytecodeFooterMarker)); err != nil {
		return err
	}
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, length)
	_, err := w.Write(buf)
	return err
}

func readEmbeddedBytecode() ([]byte, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	footerSize := int64(len(bytecodeFooterMarker) + 8)
	if info.Size() < footerSize {
		return nil, nil
	}
	_, err = file.Seek(info.Size()-footerSize, io.SeekStart)
	if err != nil {
		return nil, err
	}
	footer := make([]byte, footerSize)
	if _, err := io.ReadFull(file, footer); err != nil {
		return nil, err
	}
	marker := string(footer[:len(bytecodeFooterMarker)])
	if marker != bytecodeFooterMarker {
		return nil, nil
	}
	length := binary.LittleEndian.Uint64(footer[len(bytecodeFooterMarker):])
	dataOffset := info.Size() - footerSize - int64(length)
	if dataOffset < 0 {
		return nil, fmt.Errorf("invalid embedded bytecode length")
	}
	if _, err := file.Seek(dataOffset, io.SeekStart); err != nil {
		return nil, err
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(file, data); err != nil {
		return nil, err
	}
	return data, nil
}
