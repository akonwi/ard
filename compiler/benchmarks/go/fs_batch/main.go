package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func filePath(dir, name string) string {
	return filepath.Join(dir, name)
}

func cleanupDir(dir string) {
	if _, err := os.Stat(dir); err == nil {
		entries, err := os.ReadDir(dir)
		if err != nil {
			panic(err)
		}
		for _, entry := range entries {
			if err := os.Remove(filePath(dir, entry.Name())); err != nil {
				panic(err)
			}
		}
		if err := os.Remove(dir); err != nil {
			panic(err)
		}
	} else if !os.IsNotExist(err) {
		panic(err)
	}
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	dir := ".ard-bench-fs"
	cleanupDir(dir)
	must(os.Mkdir(dir, 0o755))

	checksum := 0

	for i := 0; i <= 400; i++ {
		baseName := fmt.Sprintf("item-%d.txt", i)
		copyName := fmt.Sprintf("copy-%d.txt", i)
		finalName := fmt.Sprintf("final-%d.txt", i)
		basePath := filePath(dir, baseName)
		copyPath := filePath(dir, copyName)
		finalPath := filePath(dir, finalName)
		content := fmt.Sprintf("record:%d:%d:%d:%d", i, i%17, i*13%29, i*i%97)

		must(os.WriteFile(basePath, []byte(content), 0o644))
		must(copyFile(basePath, copyPath))
		must(os.Rename(copyPath, finalPath))

		text, err := os.ReadFile(finalPath)
		must(err)
		checksum += len(string(text))

		if _, err := os.Stat(basePath); err == nil {
			checksum++
		} else if !os.IsNotExist(err) {
			panic(err)
		}

		info, err := os.Stat(finalPath)
		must(err)
		if !info.IsDir() {
			checksum += 2
		}
	}

	entries, err := os.ReadDir(dir)
	must(err)
	checksum += len(entries)

	for _, entry := range entries {
		checksum += len(entry.Name())
		must(os.Remove(filePath(dir, entry.Name())))
	}

	must(os.Remove(dir))
	fmt.Print(checksum)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
