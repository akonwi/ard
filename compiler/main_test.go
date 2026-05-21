package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/backend"
	gotarget "github.com/akonwi/ard/go"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close stdout writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("failed to close stdout reader: %v", err)
	}
	return string(out)
}

func TestParseRunArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		target     string
		expectErr  bool
		errMessage string
	}{
		{
			name:   "input only",
			args:   []string{"samples/main.ard"},
			path:   "samples/main.ard",
			target: "",
		},
		{
			name:   "explicit target",
			args:   []string{"--target", "go", "samples/main.ard"},
			path:   "samples/main.ard",
			target: "go",
		},
		{
			name:       "missing target value",
			args:       []string{"--target"},
			expectErr:  true,
			errMessage: "--target requires a value",
		},
		{
			name:       "unknown target",
			args:       []string{"--target", "wasm", "samples/main.ard"},
			expectErr:  true,
			errMessage: "unknown target: wasm",
		},
		{
			name:       "unknown flag",
			args:       []string{"--watch", "samples/main.ard"},
			expectErr:  true,
			errMessage: "unknown flag: --watch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, target, err := parseRunArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}
}

func TestRunGoProgram(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "main.ard")
	source := `
		fn add(a: Int, b: Int) Int {
			a + b
		}

		fn main() Int {
			add(2, 3)
		}
	`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go target: %v", err)
	}
}

func TestRunGoTargetVariablesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "variables.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go variables sample: %v", err)
	}
}

func TestRunGoTargetNullablesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "nullables.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go nullables sample: %v", err)
	}
}

func TestRunGoTargetTraitsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "traits.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go traits sample: %v", err)
	}
}

func TestRunGoTargetConcurrentStressSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "concurrent_stress.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go concurrent stress sample: %v", err)
	}
}

func TestRunGoTargetTypeUnionsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "type-unions.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go type-unions sample: %v", err)
	}
}

func TestRunGoTargetTemperaturesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "temperatures.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go temperatures sample: %v", err)
	}
}

func TestRunGoTargetLightsSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "lights.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go lights sample: %v", err)
	}
}

func TestRunGoTargetSampleStdoutConformance(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		stdin  string
		stdout string
	}{
		{
			name: "fizzbuzz",
			path: filepath.Join("samples", "fizzbuzz.ard"),
			stdout: strings.Join([]string{
				"1", "2", "Fizz", "4", "Buzz", "Fizz", "7", "8", "Fizz", "Buzz", "",
			}, "\n"),
		},
		{
			name: "loops",
			path: filepath.Join("samples", "loops.ard"),
			stdout: strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "counting from 1 to 3", "1", "2", "3", "",
			}, "\n"),
		},
		{
			name: "collections",
			path: filepath.Join("samples", "collections.ard"),
			stdout: strings.Join([]string{
				"numbers.size = 0", "adding numbers from 0 to 10", "numbers.size = 11", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "7th element = 6", "",
			}, "\n"),
		},
		{
			name: "maps",
			path: filepath.Join("samples", "maps.ard"),
			stdout: strings.Join([]string{
				"size is 1", "entries:", "1 = one", "2 = 2", "3 = 3", "4 = 4", "5 = 5", "there is an entry for 2", "2 is not found", "entries:", "1 = one", "3 = 3", "4 = 4", "5 = 5", "",
			}, "\n"),
		},
		{
			name:   "word_frequency",
			path:   filepath.Join("samples", "word_frequency.ard"),
			stdin:  "ard ard lang\n",
			stdout: "Enter some text to analyze:\n\nWord Frequency Analysis:\n------------------------\nTotal words: 3\nUnique words: 2\n\nMost frequent words:\n1. ard: 2\n2. lang: 1\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := runGoSampleBinary(t, tc.path, tc.stdin)
			if got != tc.stdout {
				t.Fatalf("stdout mismatch for %s\ngot:\n%q\nwant:\n%q", tc.path, got, tc.stdout)
			}
		})
	}
}

func TestRunGoTargetServerSampleRoutes(t *testing.T) {
	sourcePath := filepath.Join("samples", "server.ard")
	outputPath := buildGoSampleBinary(t, sourcePath)

	port := freeTCPPort(t)
	cmd := exec.Command(outputPath)
	cmd.Env = append(os.Environ(), fmt.Sprintf("PORT=%d", port))
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server sample: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHTTPServer(t, baseURL)

	assertHTTPResponse(t, http.MethodGet, baseURL+"/", "", http.StatusOK, "Hello, World!")
	assertHTTPResponse(t, http.MethodGet, baseURL+"/me", "", http.StatusOK, "this is /me")
	assertHTTPResponse(t, http.MethodGet, baseURL+"/error", "", http.StatusBadRequest, "Bad request")
	assertHTTPResponse(t, http.MethodPost, baseURL+"/api/auth/sign-up", `{"email":"ard@example.com"}`, http.StatusCreated, "Created user with email ard@example.com")
	assertHTTPResponse(t, http.MethodPost, baseURL+"/api/auth/sign-up", "", http.StatusBadRequest, "Missing request body")
	assertHTTPResponse(t, http.MethodPost, baseURL+"/api/auth/sign-up", `{"name":"Ard"}`, http.StatusBadRequest, `Missing email: email: got Missing field "email", expected Field`)
}

func buildGoSampleBinary(t *testing.T, sourcePath string) string {
	t.Helper()
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module %s: %v", sourcePath, err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR %s: %v", sourcePath, err)
	}
	outputPath := filepath.Join(t.TempDir(), filepath.Base(strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath))))
	if _, err := gotarget.BuildProgram(program, outputPath); err != nil {
		t.Fatalf("build go sample %s: %v", sourcePath, err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("stat built sample binary %s: %v", outputPath, err)
	}
	return outputPath
}

func runGoSampleBinary(t *testing.T, sourcePath, stdin string) string {
	t.Helper()
	binaryPath := buildGoSampleBinary(t, sourcePath)
	cmd := exec.Command(binaryPath)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run go sample %s: %v\nstderr:\n%s", sourcePath, err, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Fatalf("run go sample %s wrote stderr:\n%s", sourcePath, stderr.String())
	}
	return stdout.String()
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen on free TCP port: %v", err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func waitForHTTPServer(t *testing.T, baseURL string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("server at %s did not become ready", baseURL)
}

func assertHTTPResponse(t *testing.T, method, url, body string, wantStatus int, wantBody string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	gotBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	gotBody := strings.TrimRight(string(gotBodyBytes), "\n")
	if resp.StatusCode != wantStatus || gotBody != wantBody {
		t.Fatalf("%s %s = (%d, %q), want (%d, %q)", method, url, resp.StatusCode, gotBody, wantStatus, wantBody)
	}
}

func TestRunGoTargetModulesSample(t *testing.T) {
	sourcePath := filepath.Join("samples", "modules.ard")
	module, err := loadModule(sourcePath, backend.TargetGo)
	if err != nil {
		t.Fatalf("load module: %v", err)
	}
	program, err := air.Lower(module)
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	if err := gotarget.RunProgram(program, []string{"ard", "run", "--target", "go", sourcePath}); err != nil {
		t.Fatalf("run go modules sample: %v", err)
	}
}

func TestBuildRejectsInvalidMainEntrypointSignature(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		wantErr string
	}{
		{
			name: "main with args",
			source: `fn main(name: Str) Void {
			}`,
			wantErr: "main entrypoint cannot have parameters",
		},
		{
			name: "main with non-Void return",
			source: `fn main() Int {
			  1
			}`,
			wantErr: "main entrypoint must return Void, got Int",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			sourcePath := filepath.Join(tempDir, "main.ard")
			if err := os.WriteFile(sourcePath, []byte(tt.source), 0o644); err != nil {
				t.Fatalf("write source: %v", err)
			}
			_, err := buildGoBinary(sourcePath, filepath.Join(tempDir, "main-bin"), backend.TargetGo)
			if err == nil {
				t.Fatalf("buildGoBinary succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildGoBinary(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "main.ard")
	outputPath := filepath.Join(tempDir, "main-bin")
	source := `
		fn main() Void {
			()
		}
	`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	builtPath, err := buildGoBinary(sourcePath, outputPath, backend.TargetGo)
	if err != nil {
		t.Fatalf("build go target: %v", err)
	}
	if builtPath != outputPath {
		t.Fatalf("built path = %q, want %q", builtPath, outputPath)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("stat built binary: %v", err)
	}
}

func TestParseTestArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		filter     string
		failFast   bool
		expectErr  bool
		errMessage string
	}{
		{
			name:     "defaults to current directory",
			args:     []string{},
			path:     ".",
			filter:   "",
			failFast: false,
		},
		{
			name:     "path and flags",
			args:     []string{"samples", "--filter", "math", "--fail-fast"},
			path:     "samples",
			filter:   "math",
			failFast: true,
		},
		{
			name:       "missing filter value",
			args:       []string{"--filter"},
			expectErr:  true,
			errMessage: "--filter requires a value",
		},
		{
			name:       "unknown flag",
			args:       []string{"--list"},
			expectErr:  true,
			errMessage: "unknown flag: --list",
		},
		{
			name:       "unexpected extra argument",
			args:       []string{"a.ard", "b.ard"},
			expectErr:  true,
			errMessage: "unexpected argument: b.ard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, filter, failFast, _, err := parseTestArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if filter != tt.filter {
				t.Fatalf("expected filter %q, got %q", tt.filter, filter)
			}
			if failFast != tt.failFast {
				t.Fatalf("expected failFast %t, got %t", tt.failFast, failFast)
			}
		})
	}
}

func TestParseBuildArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		out        string
		target     string
		expectErr  bool
		errMessage string
	}{
		{
			name:   "input only",
			args:   []string{"demo.ard"},
			path:   "demo.ard",
			out:    "demo",
			target: "",
		},
		{
			name:   "nested input defaults to file basename",
			args:   []string{"samples/main.ard"},
			path:   "samples/main.ard",
			out:    "main",
			target: "",
		},
		{
			name:   "explicit output and target",
			args:   []string{"samples/main.ard", "--out", "demo", "--target", "go"},
			path:   "samples/main.ard",
			out:    "demo",
			target: "go",
		},
		{
			name:       "missing target value",
			args:       []string{"samples/main.ard", "--target"},
			expectErr:  true,
			errMessage: "--target requires a value",
		},
		{
			name:       "unknown target",
			args:       []string{"samples/main.ard", "--target", "wasm"},
			expectErr:  true,
			errMessage: "unknown target: wasm",
		},
		{
			name:       "unknown flag",
			args:       []string{"samples/main.ard", "--wat"},
			expectErr:  true,
			errMessage: "unknown flag: --wat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, out, target, err := parseBuildArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if out != tt.out {
				t.Fatalf("expected output %q, got %q", tt.out, out)
			}
			if target != tt.target {
				t.Fatalf("expected target %q, got %q", tt.target, target)
			}
		})
	}
}

func TestBuildJSProgramDefaultWritesArdOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "main.ard")
	if err := os.WriteFile(sourcePath, []byte(`fn main() { () }`), 0o644); err != nil {
		t.Fatal(err)
	}

	builtPath, err := buildJSProgram(sourcePath, "main", backend.TargetJSBrowser)
	if err != nil {
		t.Fatalf("buildJSProgram error = %v", err)
	}
	want := filepath.Join(dir, "ard-out", backend.TargetJSBrowser, "main.mjs")
	if builtPath != want {
		t.Fatalf("built path = %q, want %q", builtPath, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected generated root module: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(want), "ard.prelude.mjs")); err != nil {
		t.Fatalf("expected generated prelude next to root module: %v", err)
	}
	out, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("read generated root module: %v", err)
	}
	if strings.Contains(string(out), "await main();") || strings.Contains(string(out), "await __ard_script();") {
		t.Fatalf("expected build output to be importable without auto-invoking root, got:\n%s", string(out))
	}
}

func TestParseFormatArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		path       string
		checkOnly  bool
		expectErr  bool
		errMessage string
	}{
		{
			name:      "single file path",
			args:      []string{"samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: false,
		},
		{
			name:      "check mode",
			args:      []string{"--check", "samples/hello.ard"},
			path:      "samples/hello.ard",
			checkOnly: true,
		},
		{
			name:       "unknown flag",
			args:       []string{"--watch", "samples/hello.ard"},
			expectErr:  true,
			errMessage: "unknown flag: --watch",
		},
		{
			name:       "missing filepath",
			args:       []string{"--check"},
			expectErr:  true,
			errMessage: "expected filepath argument",
		},
		{
			name:       "unexpected extra argument",
			args:       []string{"a.ard", "b.ard"},
			expectErr:  true,
			errMessage: "unexpected argument: b.ard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, checkOnly, err := parseFormatArgs(tt.args)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.errMessage)
				}
				if err.Error() != tt.errMessage {
					t.Fatalf("expected error %q, got %q", tt.errMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("did not expect error: %v", err)
			}
			if path != tt.path {
				t.Fatalf("expected path %q, got %q", tt.path, path)
			}
			if checkOnly != tt.checkOnly {
				t.Fatalf("expected checkOnly %t, got %t", tt.checkOnly, checkOnly)
			}
		})
	}
}

func TestFormatFile(t *testing.T) {
	t.Run("writes formatted source", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		if err := os.WriteFile(path, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected file to change")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read formatted file: %v", err)
		}
		if string(out) != "let x = 1\n" {
			t.Fatalf("expected formatted content, got %q", string(out))
		}
	})

	t.Run("check mode does not write file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "example.ard")
		original := "let x = 1  \n"
		if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
			t.Fatalf("failed to seed test file: %v", err)
		}

		changed, err := formatFile(path, true)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if !changed {
			t.Fatalf("expected check mode to report changes")
		}

		out, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("failed to read source file: %v", err)
		}
		if string(out) != original {
			t.Fatalf("expected file to stay unchanged, got %q", string(out))
		}
	})
}

func TestFormatPath(t *testing.T) {
	t.Run("formats directories recursively", func(t *testing.T) {
		dir := t.TempDir()
		nestedDir := filepath.Join(dir, "nested")
		if err := os.MkdirAll(nestedDir, 0o755); err != nil {
			t.Fatalf("failed to create nested dir: %v", err)
		}

		first := filepath.Join(dir, "first.ard")
		second := filepath.Join(nestedDir, "second.ard")
		if err := os.WriteFile(first, []byte("let x = 1  \n"), 0o644); err != nil {
			t.Fatalf("failed to seed first file: %v", err)
		}
		if err := os.WriteFile(second, []byte("let y = 2\n"), 0o644); err != nil {
			t.Fatalf("failed to seed second file: %v", err)
		}

		changedPaths, err := formatPath(dir, false)
		if err != nil {
			t.Fatalf("did not expect error: %v", err)
		}
		if len(changedPaths) != 1 {
			t.Fatalf("expected one changed path, got %d", len(changedPaths))
		}
		if changedPaths[0] != first && changedPaths[0] != second {
			t.Fatalf("unexpected changed path %q", changedPaths[0])
		}
	})
}

func TestTestCommand(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	mainSource := `use ard/testing

test fn passes() Void!Str {
  try testing::assert(true, "true should pass")
  try testing::assert(1 + 1 == 2, "math should hold")
  testing::pass()
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.ard"), []byte(mainSource), 0o644); err != nil {
		t.Fatalf("failed to write main source: %v", err)
	}
	failureSource := `use ard/testing

test fn fails() Void!Str {
  testing::fail("nope")
}

test fn panics() Void!Str {
  panic("boom")
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "test", "failures.ard"), []byte(failureSource), 0o644); err != nil {
		t.Fatalf("failed to write test source: %v", err)
	}

	t.Run("passing filter", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "passes", false)
		})
		if !ok {
			t.Fatalf("expected tests to pass\n%s", output)
		}
		if !strings.Contains(output, "✓") || !strings.Contains(output, "1 passed; 0 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail and panic classification", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "failures", false)
		})
		if ok {
			t.Fatalf("expected failing test command behavior\n%s", output)
		}
		if !strings.Contains(output, "✗") || !strings.Contains(output, "💥") || !strings.Contains(output, "0 passed; 1 failed; 1 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})

	t.Run("fail fast stops after first failure", func(t *testing.T) {
		var ok bool
		output := captureStdout(t, func() {
			ok = runTests(projectDir, "failures", true)
		})
		if ok {
			t.Fatalf("expected failing test command behavior\n%s", output)
		}
		if strings.Contains(output, "💥") || !strings.Contains(output, "0 passed; 1 failed; 0 panicked") {
			t.Fatalf("unexpected output:\n%s", output)
		}
	})
}

func TestTestCommandGoTargetSupportsProjectFFI(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "main.ard"), []byte(`use ard/testing

extern fn lookup() Str = "Lookup"

test fn ffi_passes() Void!Str {
  testing::assert(lookup() == "ok", "project ffi should run on go target")
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ffi.go"), []byte(`package ffi

func Lookup() string { return "ok" }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var ok bool
	output := captureStdout(t, func() {
		ok = runTests(projectDir, "", false, backend.TargetGo)
	})
	if !ok {
		t.Fatalf("expected go target tests to pass\n%s", output)
	}
	if !strings.Contains(output, "✓") || !strings.Contains(output, "1 passed; 0 failed; 0 panicked") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}

func TestTestCommandRespectsPrivateAccessInTestDir(t *testing.T) {
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "test"), 0o755); err != nil {
		t.Fatalf("failed to create project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}
	utilsSource := `private fn private_helper() Int {
  42
}

fn public_helper() Int {
  7
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "utils.ard"), []byte(utilsSource), 0o644); err != nil {
		t.Fatalf("failed to write utils source: %v", err)
	}
	privateAccessSource := `use demo/utils

test fn private_access() Void!Str {
  utils::private_helper()
  Result::ok(())
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "test", "private_access.ard"), []byte(privateAccessSource), 0o644); err != nil {
		t.Fatalf("failed to write private access test: %v", err)
	}

	var ok bool
	output := captureStdout(t, func() {
		ok = runTests(projectDir, "", false)
	})
	if ok {
		t.Fatalf("expected private access test behavior to fail\n%s", output)
	}
	if !strings.Contains(output, "Undefined: utils::private_helper") {
		t.Fatalf("unexpected output:\n%s", output)
	}
}
