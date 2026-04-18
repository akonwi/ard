//go:build integration

package transpile

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/akonwi/ard/bytecode"
	bytecodevm "github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/runtime"
)

type sampleRunResult struct {
	stdout string
	stderr string
	err    error
}

type sampleSpec struct {
	name  string
	stdin string
}

func TestBuildBinaryRunsSampleSmoke(t *testing.T) {
	sampleRoot := copySamplesProject(t)
	samples := []sampleSpec{
		{name: "maps.ard"},
	}

	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
			result := runGoSample(t, sampleRoot, sample)
			if result.err != nil {
				t.Fatalf("go backend sample run failed: %v\nstdout:\n%s\nstderr:\n%s", result.err, result.stdout, result.stderr)
			}
		})
	}
}

func TestBuildBinaryMatchesVMSampleParity(t *testing.T) {
	sampleRoot := copySamplesProject(t)
	samples := []sampleSpec{
		{name: "collections.ard"},
		{name: "concurrent_stress.ard"},
		{name: "escape-sequences.ard"},
		{name: "fibonacci.ard"},
		{name: "fizzbuzz.ard"},
		{name: "grades.ard"},
		{name: "guess.ard", stdin: "10\n50\n47\n"},
		{name: "lights.ard"},
		{name: "loops.ard"},
		{name: "maps.ard"},
		{name: "modules.ard"},
		{name: "nullables.ard"},
		{name: "temperatures.ard"},
		{name: "tic-tac-toe.ard", stdin: "1\n4\n2\n5\n3\n"},
		{name: "todo-list.ard", stdin: "Write tests\n\n"},
		{name: "traits.ard"},
		{name: "type-unions.ard"},
		{name: "variables.ard"},
		{name: "word_frequency.ard", stdin: "go ard go\n"},
	}
	for _, sample := range samples {
		t.Run(sample.name, func(t *testing.T) {
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

func TestBuildBinaryMatchesVMServerSampleParity(t *testing.T) {
	ardPath := requireIntegrationArdBinary(t)

	vmRoot := copySamplesProject(t)
	vmPort := reserveLocalPort(t)
	vmBaseURL := fmt.Sprintf("http://127.0.0.1:%d", vmPort)
	rewriteServerSamplePort(t, vmRoot, vmPort)
	vmRun := startArdSampleProcess(t, ardPath, vmRoot, "run", "server.ard")
	waitForSampleServer(t, vmBaseURL)
	vmSnapshot := captureServerSampleSnapshot(t, vmBaseURL)
	vmResult := vmRun.stop()

	goRoot := copySamplesProject(t)
	goPort := reserveLocalPort(t)
	goBaseURL := fmt.Sprintf("http://127.0.0.1:%d", goPort)
	rewriteServerSamplePort(t, goRoot, goPort)
	goRun := startArdSampleProcess(t, ardPath, goRoot, "run", "--target", "go", "server.ard")
	waitForSampleServer(t, goBaseURL)
	goSnapshot := captureServerSampleSnapshot(t, goBaseURL)
	goResult := goRun.stop()

	if vmSnapshot != goSnapshot {
		t.Fatalf("server sample mismatch\nvm:\n%s\ngo:\n%s\nvm stderr:\n%s\ngo stderr:\n%s", vmSnapshot, goSnapshot, vmResult.stderr, goResult.stderr)
	}
}

func TestBuildBinaryMatchesVMPokemonSampleParity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v2/pokemon":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"count": 2, "results": [{"name": "bulbasaur", "url": "https://example.com/1"}, {"name": "ivysaur", "url": "https://example.com/2"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/post":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	sampleRoot := copySamplesProject(t)
	rewritePokemonSampleURLs(t, sampleRoot, server.URL)
	sample := sampleSpec{name: "pokemon.ard"}

	vmResult := runVMSample(t, sampleRoot, sample)
	if vmResult.err != nil {
		t.Fatalf("vm pokemon sample run failed: %v\nstdout:\n%s\nstderr:\n%s", vmResult.err, vmResult.stdout, vmResult.stderr)
	}

	goResult := runGoSample(t, sampleRoot, sample)
	if goResult.err != nil {
		t.Fatalf("go pokemon sample run failed: %v\nstdout:\n%s\nstderr:\n%s", goResult.err, goResult.stdout, goResult.stderr)
	}

	if vmResult.stdout != goResult.stdout {
		t.Fatalf("pokemon sample stdout mismatch\nvm:\n%s\ngo:\n%s\nvm stderr:\n%s\ngo stderr:\n%s", vmResult.stdout, goResult.stdout, vmResult.stderr, goResult.stderr)
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

func runVMSample(t *testing.T, sampleRoot string, sample sampleSpec) sampleRunResult {
	t.Helper()
	samplePath := filepath.Join(sampleRoot, sample.name)
	stdout, stderr, err := captureOutput(sample.stdin, func() error {
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

func runGoSample(t *testing.T, sampleRoot string, sample sampleSpec) sampleRunResult {
	t.Helper()
	result := runArdCLI(t, requireIntegrationArdBinary(t), sampleRoot, nil, sample.stdin, "run", "--target", "go", sample.name)
	return sampleRunResult{stdout: result.stdout, stderr: result.stderr, err: result.err}
}

func captureOutput(stdin string, fn func() error) (string, string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	oldStdin := os.Stdin
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
	if stdin != "" {
		stdinReader, stdinWriter, pipeErr := os.Pipe()
		if pipeErr != nil {
			stdoutReader.Close()
			stdoutWriter.Close()
			stderrReader.Close()
			stderrWriter.Close()
			return "", "", pipeErr
		}
		if _, pipeErr = stdinWriter.WriteString(stdin); pipeErr != nil {
			stdinReader.Close()
			stdinWriter.Close()
			stdoutReader.Close()
			stdoutWriter.Close()
			stderrReader.Close()
			stderrWriter.Close()
			return "", "", pipeErr
		}
		stdinWriter.Close()
		os.Stdin = stdinReader
		defer stdinReader.Close()
	}
	defer func() {
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		os.Stdin = oldStdin
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

type sampleProcess struct {
	stop func() sampleRunResult
}

func rewriteServerSamplePort(t *testing.T, sampleRoot string, port int) {
	t.Helper()
	path := filepath.Join(sampleRoot, "server.ard")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read server sample: %v", err)
	}
	updated := strings.Replace(string(content), "http::serve(8000, routes)", fmt.Sprintf("http::serve(%d, routes)", port), 1)
	if updated == string(content) {
		t.Fatalf("failed to rewrite server sample port")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("failed to rewrite server sample: %v", err)
	}
}

func rewritePokemonSampleURLs(t *testing.T, sampleRoot, baseURL string) {
	t.Helper()
	path := filepath.Join(sampleRoot, "pokemon.ard")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read pokemon sample: %v", err)
	}
	updated := string(content)
	updated = strings.ReplaceAll(updated, "https://pokeapi.co/api/v2/pokemon", baseURL+"/api/v2/pokemon")
	updated = strings.ReplaceAll(updated, "https://postman-echo.com/post", baseURL+"/post")
	if updated == string(content) {
		t.Fatalf("failed to rewrite pokemon sample URLs")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("failed to rewrite pokemon sample: %v", err)
	}
}

func startArdSampleProcess(t *testing.T, ardPath, dir string, args ...string) sampleProcess {
	t.Helper()
	cmd := exec.Command(ardPath, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sample process: %v", err)
	}
	return sampleProcess{stop: func() sampleRunResult {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		done := make(chan error, 1)
		go func() {
			done <- cmd.Wait()
		}()
		var err error
		select {
		case err = <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			err = <-done
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == -1 || strings.Contains(exitErr.Error(), "signal: killed") {
				err = nil
			}
		}
		return sampleRunResult{stdout: normalizeOutput(stdout.String()), stderr: normalizeOutput(stderr.String()), err: err}
	}}
}

func waitForSampleServer(t *testing.T, baseURL string) {
	t.Helper()
	client := &http.Client{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := client.Get(baseURL + "/")
		if err == nil {
			_, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("server did not become ready: %s", baseURL)
}

func captureServerSampleSnapshot(t *testing.T, baseURL string) string {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	requests := []struct {
		method      string
		path        string
		body        string
		contentType string
	}{
		{method: http.MethodGet, path: "/"},
		{method: http.MethodGet, path: "/me"},
		{method: http.MethodGet, path: "/error"},
		{method: http.MethodPost, path: "/api/auth/sign-up"},
		{method: http.MethodPost, path: "/api/auth/sign-up", body: "{not-json", contentType: "application/json"},
		{method: http.MethodPost, path: "/api/auth/sign-up", body: "{}", contentType: "application/json"},
		{method: http.MethodPost, path: "/api/auth/sign-up", body: "{\"email\":\"kit@example.com\"}", contentType: "application/json"},
	}

	var snapshot strings.Builder
	for _, tc := range requests {
		var bodyReader io.Reader
		if tc.body != "" {
			bodyReader = strings.NewReader(tc.body)
		}
		req, err := http.NewRequest(tc.method, baseURL+tc.path, bodyReader)
		if err != nil {
			t.Fatalf("failed to build request %s %s: %v", tc.method, tc.path, err)
		}
		if tc.contentType != "" {
			req.Header.Set("Content-Type", tc.contentType)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed %s %s: %v", tc.method, tc.path, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("failed to read response body %s %s: %v", tc.method, tc.path, err)
		}
		fmt.Fprintf(&snapshot, "%s %s\n%d\n%s\n", tc.method, tc.path, resp.StatusCode, string(body))
	}
	return snapshot.String()
}

func normalizeOutput(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}

func formatSampleFailure(sample string, vmResult, goResult sampleRunResult) string {
	return fmt.Sprintf("sample %s mismatch\nvm stdout:\n%s\nvm stderr:\n%s\ngo stdout:\n%s\ngo stderr:\n%s", sample, vmResult.stdout, vmResult.stderr, goResult.stdout, goResult.stderr)
}
