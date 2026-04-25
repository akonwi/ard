package go_backend

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func BuildBinary(inputPath, outputPath string) (string, error) {
	module, project, err := loadModule(inputPath)
	if err != nil {
		return "", err
	}

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := writeGeneratedProject(generatedDir, project, module); err != nil {
		return "", err
	}

	resolvedOutputPath, err := filepath.Abs(outputPath)
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-mod=mod", "-o", resolvedOutputPath, ".")
	configureGoCommand(cmd)
	cmd.Dir = generatedDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	return outputPath, nil
}

func Run(inputPath string, args []string) error {
	module, project, err := loadModule(inputPath)
	if err != nil {
		return err
	}

	generatedDir := filepath.Join(project.RootPath, "generated")
	if err := writeGeneratedProject(generatedDir, project, module); err != nil {
		return err
	}

	normalizedArgs, err := json.Marshal(normalizeCLIArgs(args))
	if err != nil {
		return err
	}

	cmd := exec.Command("go", "run", "-mod=mod", ".")
	configureGoCommand(cmd)
	cmd.Dir = generatedDir
	cmd.Env = append(cmd.Env, osArgsEnvVar+"="+string(normalizedArgs))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func configureGoCommand(cmd *exec.Cmd) {
	cmd.Env = append([]string{}, os.Environ()...)
	const requiredFlag = "-tags=goexperiment.jsonv2"
	goFlags := os.Getenv("GOFLAGS")
	if strings.Contains(goFlags, requiredFlag) {
		return
	}
	if strings.TrimSpace(goFlags) == "" {
		cmd.Env = append(cmd.Env, "GOFLAGS="+requiredFlag)
		return
	}
	cmd.Env = append(cmd.Env, "GOFLAGS="+goFlags+" "+requiredFlag)
}

func normalizeCLIArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--target" {
			i++
			continue
		}
		normalized = append(normalized, args[i])
	}
	return normalized
}
