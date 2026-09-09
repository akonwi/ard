package checker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
	"golang.org/x/mod/module"
)

const (
	EmbedModulePath             = "ard/embed"
	MaxEmbeddedFileBytes        = 16 * 1024 * 1024
	MaxEmbeddedProgramBytes     = 128 * 1024 * 1024
	MaxEmbeddedProgramFileCount = 10_000
)

type EmbedPkg struct{}

func (EmbedPkg) Path() string      { return EmbedModulePath }
func (EmbedPkg) Program() *Program { return nil }

func (EmbedPkg) Get(name string) Symbol {
	var returnType Type
	switch name {
	case "text":
		returnType = Str
	case "bytes":
		returnType = MakeList(Byte)
	default:
		return Symbol{}
	}
	return Symbol{Name: name, Type: &FunctionDef{
		Name:       name,
		Parameters: []Parameter{{Name: "path", Type: Str}},
		ReturnType: returnType,
	}}
}

func (c *Checker) checkEmbedExactCall(s *parse.StaticFunction, name string) Expression {
	qualified := "embed::" + name
	if name != "text" && name != "bytes" {
		return nil
	}
	if len(s.Function.TypeArgs) != 0 {
		c.addInvalidFunctionTypeArguments(qualified, 0, len(s.Function.TypeArgs), false, s.GetLocation(), qualified+" does not accept type arguments")
		return nil
	}
	if c.rejectSpreadForFixedCall(s.Function.Args) {
		return nil
	}
	if len(s.Function.Args) != 1 {
		c.addArgumentCount("1", len(s.Function.Args), s.GetLocation(), "")
		return nil
	}
	arg := s.Function.Args[0]
	if arg.Name != "" {
		c.addNamedArgumentsUnsupported("embed constructor", arg.GetLocation())
		return nil
	}
	literal, ok := arg.Value.(*parse.StrLiteral)
	if !ok {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedStaticArgument, "Embedded resource path must be a static string", "use a non-interpolated string literal selected during checking", arg.Value.GetLocation())
		return nil
	}
	if c.moduleResolver == nil {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedResource, "Cannot resolve embedded resource", "embedding requires project package context", literal.GetLocation())
		return nil
	}
	resource, err := c.moduleResolver.resolveEmbeddedExactFile(c.modulePath, literal.Value)
	if err != nil {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedResource, fmt.Sprintf("Cannot embed %q", literal.Value), err.Error(), literal.GetLocation())
		return nil
	}
	if name == "text" {
		if !utf8.Valid(resource.Data) {
			c.addEmbedDiagnostic(DiagnosticCodeEmbedTextUTF8, fmt.Sprintf("Cannot embed %q as text", literal.Value), "the file is not valid UTF-8; use embed::bytes instead", literal.GetLocation())
			return nil
		}
		return &EmbeddedText{Resource: resource}
	}
	return &EmbeddedBytes{Resource: resource}
}

func (c *Checker) addEmbedDiagnostic(code DiagnosticCode, title string, text string, location parse.Location) {
	diagnostic := newLabeledDiagnostic(
		Error,
		title+": "+text,
		title,
		text,
		DiagnosticLabel{Span: c.sourceSpan(location), Message: title},
	)
	diagnostic.Code = code
	c.addDiagnostic(diagnostic)
}

func (mr *ModuleResolver) resolveEmbeddedExactFile(importerModulePath string, logicalPath string) (EmbeddedResource, error) {
	if err := validateEmbeddedExactPath(logicalPath); err != nil {
		return EmbeddedResource{}, err
	}
	packageID := mr.packageIDForModule(importerModulePath)
	pkg := mr.packageInfo(packageID)
	if pkg.RootPath == "" {
		return EmbeddedResource{}, fmt.Errorf("owning Ard package has no source root")
	}

	cacheKey := packageID + "\x00" + logicalPath
	mr.embedMu.Lock()
	defer mr.embedMu.Unlock()
	if resource, ok := mr.embeddedResources[cacheKey]; ok {
		return resource, nil
	}

	root, err := os.OpenRoot(pkg.RootPath)
	if err != nil {
		return EmbeddedResource{}, fmt.Errorf("open owning Ard package root: %w", err)
	}
	defer root.Close()

	components := strings.Split(logicalPath, "/")
	current := ""
	for index, component := range components {
		current = filepath.Join(current, component)
		info, err := root.Lstat(current)
		if err != nil {
			return EmbeddedResource{}, fmt.Errorf("inspect embedded resource: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return EmbeddedResource{}, fmt.Errorf("embedded resource path must not contain a symlink: %s", logicalPath)
		}
		if index < len(components)-1 {
			if !info.IsDir() {
				return EmbeddedResource{}, fmt.Errorf("embedded resource path component is not a directory: %s", strings.Join(components[:index+1], "/"))
			}
			switch component {
			case ".bzr", ".git", ".hg", ".svn", "vendor":
				return EmbeddedResource{}, fmt.Errorf("embedded resources must not select reserved directory %q", component)
			}
			if _, err := root.Stat(filepath.Join(current, "ard.toml")); err == nil {
				return EmbeddedResource{}, fmt.Errorf("embedded resource crosses a nested Ard package boundary: %s", strings.Join(components[:index+1], "/"))
			} else if !os.IsNotExist(err) {
				return EmbeddedResource{}, fmt.Errorf("inspect nested Ard package boundary: %w", err)
			}
			if _, err := root.Stat(filepath.Join(current, "go.mod")); err == nil {
				return EmbeddedResource{}, fmt.Errorf("embedded resource crosses a nested Go module boundary: %s", strings.Join(components[:index+1], "/"))
			} else if !os.IsNotExist(err) {
				return EmbeddedResource{}, fmt.Errorf("inspect nested Go module boundary: %w", err)
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return EmbeddedResource{}, fmt.Errorf("embedded resource must be a regular file: %s", logicalPath)
		}
	}

	file, err := root.Open(filepath.FromSlash(logicalPath))
	if err != nil {
		return EmbeddedResource{}, fmt.Errorf("open embedded resource: %w", err)
	}
	info, statErr := file.Stat()
	if statErr != nil {
		_ = file.Close()
		return EmbeddedResource{}, fmt.Errorf("inspect opened embedded resource: %w", statErr)
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return EmbeddedResource{}, fmt.Errorf("embedded resource must be a regular file: %s", logicalPath)
	}
	if info.Size() > MaxEmbeddedFileBytes {
		_ = file.Close()
		return EmbeddedResource{}, fmt.Errorf("embedded resource is %d bytes; maximum file size is %d bytes", info.Size(), MaxEmbeddedFileBytes)
	}
	data, readErr := io.ReadAll(io.LimitReader(file, MaxEmbeddedFileBytes+1))
	closeErr := file.Close()
	if readErr != nil {
		return EmbeddedResource{}, fmt.Errorf("read embedded resource: %w", readErr)
	}
	if closeErr != nil {
		return EmbeddedResource{}, fmt.Errorf("close embedded resource: %w", closeErr)
	}
	if len(data) > MaxEmbeddedFileBytes {
		return EmbeddedResource{}, fmt.Errorf("embedded resource exceeds maximum file size of %d bytes", MaxEmbeddedFileBytes)
	}
	if len(mr.embeddedResources)+1 > MaxEmbeddedProgramFileCount {
		return EmbeddedResource{}, fmt.Errorf("embedded resources exceed program limit of %d files", MaxEmbeddedProgramFileCount)
	}
	if mr.embeddedResourceBytes+len(data) > MaxEmbeddedProgramBytes {
		return EmbeddedResource{}, fmt.Errorf("embedded resources exceed program limit of %d bytes", MaxEmbeddedProgramBytes)
	}

	resource := EmbeddedResource{OwnerPackageIdentity: packageID, LogicalPath: logicalPath, Data: data}
	mr.embeddedResources[cacheKey] = resource
	mr.embeddedResourceBytes += len(data)
	return resource, nil
}

func validateEmbeddedExactPath(path string) error {
	if path == "" {
		return fmt.Errorf("embedded resource path must not be empty")
	}
	if strings.Contains(path, `\`) {
		return fmt.Errorf("embedded resource paths use forward slashes")
	}
	if err := module.CheckFilePath(path); err != nil {
		return fmt.Errorf("invalid embedded resource path: %w", err)
	}
	for _, component := range strings.Split(path, "/") {
		if component == "go.mod" {
			return fmt.Errorf("embedded resources must not select a file named go.mod")
		}
	}
	return nil
}
