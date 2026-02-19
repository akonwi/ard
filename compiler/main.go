package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/formatter"
	"github.com/akonwi/ard/parse"
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
			if _, err := bytecodevm.New(program).Run("main"); err != nil {
				fmt.Println(err)
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
	case "format":
		{
			inputPath, checkOnly, err := parseFormatArgs(os.Args[2:])
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			changed, err := formatFile(inputPath, checkOnly)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			if checkOnly {
				if changed {
					fmt.Printf("%s is not formatted\n", inputPath)
					os.Exit(1)
				}
				fmt.Printf("%s is formatted\n", inputPath)
				os.Exit(0)
			}
			if changed {
				fmt.Printf("Formatted %s\n", inputPath)
				os.Exit(0)
			}
			fmt.Printf("%s is already formatted\n", inputPath)
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

func formatFile(inputPath string, checkOnly bool) (bool, error) {
	sourceCode, err := os.ReadFile(inputPath)
	if err != nil {
		return false, fmt.Errorf("error reading file %s - %w", inputPath, err)
	}

	formatted := formatter.Format(sourceCode)
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

func maybeRunEmbedded() bool {
	if len(os.Args) > 1 && os.Args[1] != "run-embedded" {
		return false
	}
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
	if _, err := bytecodevm.New(program).Run("main"); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
