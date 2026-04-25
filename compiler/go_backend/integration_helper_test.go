//go:build integration

package go_backend

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var (
	integrationArdPath string
	integrationArdErr  error
)

func init() {
	compilerRoot, err := compilerModuleRoot()
	if err != nil {
		integrationArdErr = fmt.Errorf("determine compiler root: %w", err)
		return
	}
	tmpDir, err := os.MkdirTemp("", "ard-integration-*")
	if err != nil {
		integrationArdErr = fmt.Errorf("create temp dir: %w", err)
		return
	}
	integrationArdPath = filepath.Join(tmpDir, "ard")
	cmd := exec.Command("go", "build", "-o", integrationArdPath, ".")
	configureGoCommand(cmd)
	cmd.Dir = compilerRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		integrationArdErr = fmt.Errorf("build ard CLI: %w\n%s", err, string(output))
		return
	}
}

func requireIntegrationArdBinary(t *testing.T) string {
	t.Helper()
	if integrationArdErr != nil {
		t.Fatalf("failed to prepare integration ard binary: %v", integrationArdErr)
	}
	return integrationArdPath
}

func assertGoTargetRunSucceeds(t *testing.T, dir, entrypoint string) cliRunResult {
	t.Helper()
	result := runArdCLI(t, requireIntegrationArdBinary(t), dir, nil, "", "run", "--target", "go", entrypoint)
	if result.err != nil {
		t.Fatalf("did not expect go target run to fail: %s", formatCLIRunFailure(result))
	}
	return result
}

func assertGoTargetRunOutput(t *testing.T, dir, entrypoint, expected string) {
	t.Helper()
	result := assertGoTargetRunSucceeds(t, dir, entrypoint)
	if result.stdout != expected {
		t.Fatalf("unexpected stdout\nexpected:\n%s\nactual:\n%s\nstderr:\n%s", expected, result.stdout, result.stderr)
	}
}
