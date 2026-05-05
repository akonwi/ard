package go_backend

import "fmt"

func Run(inputPath string, args []string) error {
	_ = inputPath
	_ = args
	return fmt.Errorf("go target rewrite in progress")
}

func BuildBinary(inputPath string, outputPath string) (string, error) {
	_ = inputPath
	_ = outputPath
	return "", fmt.Errorf("go target rewrite in progress")
}
