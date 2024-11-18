package main

import (
	"flag"
	"fmt"
	"os"

	tree_sitter_kon "github.com/akonwi/tree-sitter-kon/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func main() {
	buildCmd := flag.NewFlagSet("build", flag.ExitOnError)

	if len(os.Args) < 2 {
		fmt.Println("Please provide a command")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "build":
		buildCmd.Parse(os.Args[2:])

		if buildCmd.NArg() < 1 {
			fmt.Println("Expected filepath argument")
			os.Exit(1)
		}

		filepath := buildCmd.Arg(0)
		sourceCode, err := os.ReadFile(filepath)
		if err != nil {
			fmt.Printf("Error reading file %s - %v\n", filepath, err)
			os.Exit(1)
		}

		language := tree_sitter.NewLanguage(tree_sitter_kon.Language())
		if language == nil {
			fmt.Println("Error loading Kon grammar")
			os.Exit(1)
		}
		parser := tree_sitter.NewParser()
		parser.SetLanguage(language)
		parser.Parse(sourceCode, nil)

		fmt.Println("Successfully parsed the file")

	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
