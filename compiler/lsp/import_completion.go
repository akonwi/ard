package lsp

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"go.lsp.dev/protocol"
)

func importCompletionPrefix(linePrefix string) (string, bool) {
	trimmed := strings.TrimLeft(linePrefix, " \t")
	if !strings.HasPrefix(trimmed, "use ") {
		return "", false
	}
	pathPrefix := strings.TrimSpace(strings.TrimPrefix(trimmed, "use "))
	if strings.Contains(pathPrefix, " ") || strings.Contains(pathPrefix, "\t") {
		return "", false
	}
	return pathPrefix, true
}

func importPathCompletionItems(pathPrefix string, filePath string) []protocol.CompletionItem {
	workingDir := filepath.Dir(filePath)
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		return nil
	}
	project := resolver.GetProjectInfo()
	if project == nil {
		return nil
	}

	segments := strings.Split(pathPrefix, "/")
	if pathPrefix == "" {
		segments = []string{""}
	}
	current := segments[len(segments)-1]
	base := strings.Join(segments[:len(segments)-1], "/")

	candidates := map[string]protocol.CompletionItem{}
	add := func(label string) {
		if label == "" || !strings.HasPrefix(label, current) {
			return
		}
		candidates[label] = protocol.CompletionItem{Label: label, Kind: protocol.CompletionItemKindModule, InsertText: label}
	}

	if base == "" {
		add("ard")
		add(project.ProjectName)
		for alias := range project.Dependencies {
			add(alias)
		}
	} else if base == "ard" {
		for _, entry := range listArdStdlibImportChildren("") {
			add(entry)
		}
	} else if strings.HasPrefix(base, "ard/") {
		for _, entry := range listArdStdlibImportChildren(strings.TrimPrefix(base, "ard/")) {
			add(entry)
		}
	} else {
		root := ""
		rootName := strings.Split(base, "/")[0]
		if rootName == project.ProjectName {
			root = project.RootPath
		} else if dep, ok := project.Dependencies[rootName]; ok {
			root = dep.VendorPath
		}
		if root != "" {
			rel := strings.TrimPrefix(base, rootName)
			rel = strings.TrimPrefix(rel, "/")
			for _, entry := range listProjectImportChildren(root, rel) {
				add(entry)
			}
		}
	}

	items := make([]protocol.CompletionItem, 0, len(candidates))
	for _, item := range candidates {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Label < items[j].Label })
	return items
}

func listArdStdlibImportChildren(rel string) []string {
	root := filepath.Dir(stdLibSourcePath("ard/io"))
	return listImportChildren(root, rel)
}

func listProjectImportChildren(root string, rel string) []string {
	return listImportChildren(root, rel)
}

func listImportChildren(root string, rel string) []string {
	dir := filepath.Join(root, filepath.FromSlash(rel))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "ard-out" || name == ".ard" {
			continue
		}
		if entry.IsDir() {
			seen[name] = true
			continue
		}
		if strings.HasSuffix(name, ".ard") {
			seen[strings.TrimSuffix(name, ".ard")] = true
		}
	}
	children := make([]string, 0, len(seen))
	for child := range seen {
		children = append(children, child)
	}
	sort.Strings(children)
	return children
}
