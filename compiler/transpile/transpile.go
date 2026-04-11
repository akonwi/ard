package transpile

import "fmt"

func BuildBinary(inputPath, outputPath string) (string, error) {
	_ = inputPath
	_ = outputPath
	return "", fmt.Errorf("go target not implemented yet")
}

func Run(inputPath string, args []string) error {
	_ = inputPath
	_ = args
	return fmt.Errorf("go target not implemented yet")
}
