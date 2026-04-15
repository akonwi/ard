package transpile

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/runtime"
)

type sampleRunResult struct {
	stdout string
	stderr string
	err    error
}

func TestBuildBinaryRunsSampleSmoke(t *testing.T) {
	sampleRoot := copySamplesProject(t)
	samples := []string{
		"maps.ard",
	}

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			result := runGoSample(t, sampleRoot, sample)
			if result.err != nil {
				t.Fatalf("go backend sample run failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
			}
		})
	}
}

func TestBuildBinaryMatchesVMSampleParity(t *testing.T) {
	sampleRoot := copySamplesProject(t)
	samples := []string{
		"collections.ard",
		"escape-sequences.ard",
		"fibonacci.ard",
		"fizzbuzz.ard",
		"grades.ard",
		"lights.ard",
		"loops.ard",
		"maps.ard",
		"modules.ard",
		"nullables.ard",
		"temperatures.ard",
		"traits.ard",
		"type-unions.ard",
		"variables.ard",
	}

	for _, sample := range samples {
		t.Run(sample, func(t *testing.T) {
			vmResult := runVMSample(t, sampleRoot, sample)
			if vmResult.err != nil {
				t.Fatalf("vm sample run failed: %v\nstdout:\n%s\nstderr:\n%s", vmResult.err, vmResult.stdout, vmResult.stderr)
			}

			goResult := runGoSample(t, sampleRoot, sample)
			if goResult.err != nil {
				t.Fatalf("go backend sample run failed: %v\nstdout:\n%s\nstderr:\n%s", goResult.err, goResult.stdout, goResult.stderr)
			}

			if vmResult.stdout != goResult.stdout {
				t.Fatalf("sample stdout mismatch\nvm:\n%s\ngo:\n%s\nvm stderr:\n%s\ngo stderr:\n%s", vmResult.stdout, goResult.stdout, vmResult.stderr, goResult.stderr)
			}
		})
	}
}

func copySamplesProject(t *testing.T) string {
	t.Helper()
	compilerRoot, err := compilerModuleRoot()
	if err != nil {
		t.Fatalf("failed to determine compiler root: %v", err)
	}
	sourceRoot := filepath.Join(compilerRoot, "samples")
	targetRoot := filepath.Join(t.TempDir(), "samples")
	if err := copyDir(sourceRoot, targetRoot); err != nil {
		t.Fatalf("failed to copy samples project: %v", err)
	}
	return targetRoot
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
}

func runVMSample(t *testing.T, sampleRoot, sample string) sampleRunResult {
	t.Helper()
	samplePath := filepath.Join(sampleRoot, sample)
	stdout, stderr, err := captureOutput(func() error {
		module, _, err := loadModule(samplePath)
		if err != nil {
			return err
		}
		program, err := bytecode.NewEmitter().EmitProgram(module)
		if err != nil {
			return err
		}
		if err := bytecode.VerifyProgram(program); err != nil {
			return err
		}
		runtime.SetOSArgs([]string{samplePath})
		defer runtime.SetOSArgs(nil)
		_, runErr := bytecodevm.New(program).Run("main")
		return runErr
	})
	return sampleRunResult{stdout: normalizeOutput(stdout), stderr: normalizeOutput(stderr), err: err}
}

func runGoSample(t *testing.T, sampleRoot, sample string) sampleRunResult {
	t.Helper()
	samplePath := filepath.Join(sampleRoot, sample)
	outputPath := filepath.Join(sampleRoot, strings.TrimSuffix(sample, filepath.Ext(sample))+"-bin")
	if _, err := BuildBinary(samplePath, outputPath); err != nil {
		return sampleRunResult{err: err}
	}
	cmd := exec.Command(outputPath)
	cmd.Dir = sampleRoot
	stdout, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
		return sampleRunResult{stdout: normalizeOutput(string(stdout)), stderr: normalizeOutput(string(stderr)), err: err}
	}
	return sampleRunResult{stdout: normalizeOutput(string(stdout))}
}

func captureOutput(fn func() error) (string, string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		stdoutReader.Close()
		stdoutWriter.Close()
		return "", "", err
	}
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
	}()

	stdoutCh := make(chan string, 1)
	stderrCh := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(stdoutReader)
		stdoutCh <- string(data)
	}()
	go func() {
		data, _ := io.ReadAll(stderrReader)
		stderrCh <- string(data)
	}()

	runErr := fn()
	stdoutWriter.Close()
	stderrWriter.Close()
	stdout := <-stdoutCh
	stderr := <-stderrCh
	stdoutReader.Close()
	stderrReader.Close()
	return stdout, stderr, runErr
}

func normalizeOutput(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func formatSampleFailure(sample string, vmResult, goResult sampleRunResult) string {
	return fmt.Sprintf("sample %s mismatch\nvm stdout:\n%s\nvm stderr:\n%s\ngo stdout:\n%s\ngo stderr:\n%s", sample, vmResult.stdout, vmResult.stderr, goResult.stdout, goResult.stderr)
}
