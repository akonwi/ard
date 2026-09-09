package checker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/akonwi/ard/parse"
	"golang.org/x/mod/module"
)

const (
	EmbedModulePath             = "ard/embed"
	MaxEmbeddedFileBytes        = 16 * 1024 * 1024
	MaxEmbeddedSetBytes         = 64 * 1024 * 1024
	MaxEmbeddedProgramBytes     = 128 * 1024 * 1024
	MaxEmbeddedProgramFileCount = 10_000
)

type embeddedFSType struct{}

var EmbeddedFS Type = &embeddedFSType{}

var (
	embeddedDirEntryType = &StructDef{Name: "DirEntry", ModulePath: EmbedModulePath, Fields: map[string]Type{"name": Str, "is_dir": Bool}}
	embeddedFileInfoType = &StructDef{Name: "FileInfo", ModulePath: EmbedModulePath, Fields: map[string]Type{"name": Str, "is_dir": Bool, "size": MakeMaybe(Int)}}
)

func (*embeddedFSType) String() string        { return "embed::FS" }
func (*embeddedFSType) equal(other Type) bool { _, ok := other.(*embeddedFSType); return ok }
func (*embeddedFSType) hasTrait(*Trait) bool  { return false }
func (*embeddedFSType) get(name string) Type {
	var returnType Type
	switch name {
	case "read_file":
		returnType = MakeResult(MakeList(Byte), BuiltinError)
	case "read_text":
		returnType = MakeResult(Str, BuiltinError)
	case "read_dir":
		returnType = MakeResult(MakeList(embeddedDirEntryType), BuiltinError)
	case "stat":
		returnType = MakeResult(embeddedFileInfoType, BuiltinError)
	case "sub":
		returnType = MakeResult(EmbeddedFS, BuiltinError)
	default:
		return nil
	}
	return &FunctionDef{Name: name, Parameters: []Parameter{{Name: "path", Type: Str}}, ReturnType: returnType}
}

type EmbedPkg struct{}

func (EmbedPkg) Path() string      { return EmbedModulePath }
func (EmbedPkg) Program() *Program { return nil }

func (EmbedPkg) Get(name string) Symbol {
	switch name {
	case "FS":
		return Symbol{Name: name, Type: EmbeddedFS, typeDeclaration: true}
	case "DirEntry":
		return Symbol{Name: name, Type: embeddedDirEntryType, typeDeclaration: true}
	case "FileInfo":
		return Symbol{Name: name, Type: embeddedFileInfoType, typeDeclaration: true}
	}
	var parameter Type = Str
	var returnType Type
	switch name {
	case "text":
		returnType = Str
	case "bytes":
		returnType = MakeList(Byte)
	case "fs":
		parameter = MakeList(Str)
		returnType = EmbeddedFS
	default:
		return Symbol{}
	}
	return Symbol{Name: name, Type: &FunctionDef{
		Name:       name,
		Parameters: []Parameter{{Name: "path", Type: parameter}},
		ReturnType: returnType,
	}}
}

func (c *Checker) checkEmbedFSCall(s *parse.StaticFunction) Expression {
	if len(s.Function.TypeArgs) != 0 {
		c.addInvalidFunctionTypeArguments("embed::fs", 0, len(s.Function.TypeArgs), false, s.GetLocation(), "embed::fs does not accept type arguments")
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
	list, ok := arg.Value.(*parse.ListLiteral)
	if !ok || len(list.Items) == 0 {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedStaticArgument, "Embed patterns must be a non-empty static list of strings", "files are selected while the package is checked", arg.Value.GetLocation())
		return nil
	}
	patterns := make([]string, len(list.Items))
	for index, item := range list.Items {
		literal, ok := item.(*parse.StrLiteral)
		if !ok {
			c.addEmbedDiagnostic(DiagnosticCodeEmbedStaticArgument, "Embed patterns must be a static list of strings", "use non-interpolated string literals", item.GetLocation())
			return nil
		}
		patterns[index] = literal.Value
	}
	if c.moduleResolver == nil {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedResource, "Cannot resolve embedded filesystem", "embedding requires project package context", list.GetLocation())
		return nil
	}
	set, err := c.moduleResolver.resolveEmbeddedFileSet(c.modulePath, patterns)
	if err != nil {
		c.addEmbedDiagnostic(DiagnosticCodeEmbedResource, "Cannot construct embedded filesystem", err.Error(), list.GetLocation())
		return nil
	}
	return &EmbeddedFSValue{Set: set}
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
	if err == nil {
		err = c.moduleResolver.accountEmbeddedExact(resource)
	}
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

func (c *Checker) createEmbeddedFSMethod(subject Expression, name string, args []Expression, declaration *FunctionDef) Expression {
	var kind EmbeddedFSMethodKind
	switch name {
	case "read_file":
		kind = EmbeddedFSReadFile
	case "read_text":
		kind = EmbeddedFSReadText
	case "read_dir":
		kind = EmbeddedFSReadDir
	case "stat":
		kind = EmbeddedFSStat
	case "sub":
		kind = EmbeddedFSSub
	default:
		return nil
	}
	return &EmbeddedFSMethod{Subject: subject, Kind: kind, Args: args, ReturnType: declaration.ReturnType}
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
	if err := ValidateEmbeddedLogicalFilePath(logicalPath); err != nil {
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
			case ".bzr", ".git", ".hg", ".svn", "ard-out", "vendor":
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
	resource := EmbeddedResource{OwnerPackageIdentity: embeddedOwnerIdentity(pkg), LogicalPath: logicalPath, Data: data}
	mr.embeddedResources[cacheKey] = resource
	return resource, nil
}

func embeddedOwnerIdentity(pkg PackageInfo) string {
	if pkg.Git != "" {
		return pkg.ID
	}
	return pkg.Name
}

func (mr *ModuleResolver) accountEmbeddedExact(resource EmbeddedResource) error {
	sum := sha256.Sum256(resource.Data)
	key := hex.EncodeToString(sum[:])
	mr.embedMu.Lock()
	defer mr.embedMu.Unlock()
	if mr.embeddedExactReferences[key] {
		return nil
	}
	if mr.embeddedProgramFileCount+1 > MaxEmbeddedProgramFileCount {
		return fmt.Errorf("embedded resources exceed program limit of %d files", MaxEmbeddedProgramFileCount)
	}
	if mr.embeddedProgramBytes+len(resource.Data) > MaxEmbeddedProgramBytes {
		return fmt.Errorf("embedded resources exceed program limit of %d bytes", MaxEmbeddedProgramBytes)
	}
	mr.embeddedExactReferences[key] = true
	mr.embeddedProgramFileCount++
	mr.embeddedProgramBytes += len(resource.Data)
	return nil
}

func (mr *ModuleResolver) resolveEmbeddedFileSet(importerModulePath string, patterns []string) (EmbeddedFileSet, error) {
	packageID := mr.packageIDForModule(importerModulePath)
	pkg := mr.packageInfo(packageID)
	if pkg.RootPath == "" {
		return EmbeddedFileSet{}, fmt.Errorf("owning Ard package has no source root")
	}
	patternCacheKey := packageID + "\x00" + strings.Join(patterns, "\x00")
	mr.embedMu.Lock()
	if cached, ok := mr.embeddedPatternSets[patternCacheKey]; ok {
		mr.embedMu.Unlock()
		return cached, nil
	}
	mr.embedMu.Unlock()

	type patternSpec struct {
		value      string
		includeAll bool
	}
	specs := make([]patternSpec, len(patterns))
	for index, original := range patterns {
		value := original
		includeAll := strings.HasPrefix(value, "all:")
		if includeAll {
			value = strings.TrimPrefix(value, "all:")
		}
		if value == "" || value == "." || !iofs.ValidPath(value) || strings.Contains(value, `\`) {
			return EmbeddedFileSet{}, fmt.Errorf("invalid embed pattern %q", original)
		}
		if _, err := path.Match(value, ""); err != nil {
			return EmbeddedFileSet{}, fmt.Errorf("invalid embed pattern %q: %w", original, err)
		}
		specs[index] = patternSpec{value: value, includeAll: includeAll}
	}

	selected := map[string]bool{}
	matchedPatterns := make([]bool, len(specs))
	scanRoots := map[string]bool{}
	for _, spec := range specs {
		scanRoot := embedPatternScanRoot(spec.value)
		scanRoots[filepath.Join(pkg.RootPath, filepath.FromSlash(scanRoot))] = true
	}
	orderedRoots := make([]string, 0, len(scanRoots))
	for scanRoot := range scanRoots {
		orderedRoots = append(orderedRoots, scanRoot)
	}
	sort.Strings(orderedRoots)
	walk := func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == pkg.RootPath {
			return nil
		}
		rel, err := filepath.Rel(pkg.RootPath, filePath)
		if err != nil {
			return err
		}
		logical := filepath.ToSlash(rel)
		if entry.Type()&os.ModeSymlink != 0 {
			for index, spec := range specs {
				if embedPatternSelectsFile(spec.value, spec.includeAll, logical) {
					return fmt.Errorf("embed pattern %q selects symbolic link %q", patterns[index], logical)
				}
			}
			return nil
		}
		if entry.IsDir() {
			base := entry.Name()
			switch base {
			case ".bzr", ".git", ".hg", ".svn", "ard-out", "vendor":
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(filePath, "ard.toml")); err == nil {
				return filepath.SkipDir
			} else if !os.IsNotExist(err) {
				return err
			}
			if _, err := os.Stat(filepath.Join(filePath, "go.mod")); err == nil {
				return filepath.SkipDir
			} else if !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		for index, spec := range specs {
			if !embedPatternSelectsFile(spec.value, spec.includeAll, logical) {
				continue
			}
			matchedPatterns[index] = true
			selected[logical] = true
			if len(selected) > MaxEmbeddedProgramFileCount {
				return fmt.Errorf("embedded filesystem exceeds program limit of %d files", MaxEmbeddedProgramFileCount)
			}
		}
		return nil
	}
	for _, scanRoot := range orderedRoots {
		if err := filepath.WalkDir(scanRoot, walk); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return EmbeddedFileSet{}, fmt.Errorf("scan embedded resources: %w", err)
		}
	}
	for index, matched := range matchedPatterns {
		if !matched {
			return EmbeddedFileSet{}, fmt.Errorf("embed pattern %q matched no files", patterns[index])
		}
	}

	paths := make([]string, 0, len(selected))
	for file := range selected {
		paths = append(paths, file)
	}
	sort.Strings(paths)
	if len(paths) > MaxEmbeddedProgramFileCount {
		return EmbeddedFileSet{}, fmt.Errorf("embedded filesystem exceeds program limit of %d files", MaxEmbeddedProgramFileCount)
	}
	set := EmbeddedFileSet{OwnerPackageIdentity: embeddedOwnerIdentity(pkg), Entries: make([]EmbeddedSetEntry, 0, len(paths))}
	totalBytes := 0
	for _, logicalPath := range paths {
		for _, component := range strings.Split(logicalPath, "/") {
			switch component {
			case ".bzr", ".git", ".hg", ".svn":
				return EmbeddedFileSet{}, fmt.Errorf("embedded filesystems cannot contain reserved path %q", component)
			}
		}
		resource, err := mr.resolveEmbeddedExactFile(importerModulePath, logicalPath)
		if err != nil {
			return EmbeddedFileSet{}, err
		}
		totalBytes += len(resource.Data)
		if totalBytes > MaxEmbeddedSetBytes {
			return EmbeddedFileSet{}, fmt.Errorf("embedded filesystem exceeds limit of %d bytes", MaxEmbeddedSetBytes)
		}
		set.Entries = append(set.Entries, EmbeddedSetEntry{LogicalPath: logicalPath, Data: resource.Data})
	}

	hash := sha256.New()
	_, _ = hash.Write([]byte(set.OwnerPackageIdentity))
	_, _ = hash.Write([]byte{0})
	for _, entry := range set.Entries {
		_, _ = hash.Write([]byte(entry.LogicalPath))
		_, _ = hash.Write([]byte{0})
		sum := sha256.Sum256(entry.Data)
		_, _ = hash.Write(sum[:])
	}
	identity := hex.EncodeToString(hash.Sum(nil))
	mr.embedMu.Lock()
	defer mr.embedMu.Unlock()
	if !mr.embeddedSetIdentities[identity] {
		if mr.embeddedProgramFileCount+len(set.Entries) > MaxEmbeddedProgramFileCount {
			return EmbeddedFileSet{}, fmt.Errorf("embedded resources exceed program limit of %d files", MaxEmbeddedProgramFileCount)
		}
		if mr.embeddedProgramBytes+totalBytes > MaxEmbeddedProgramBytes {
			return EmbeddedFileSet{}, fmt.Errorf("embedded resources exceed program limit of %d bytes", MaxEmbeddedProgramBytes)
		}
		mr.embeddedSetIdentities[identity] = true
		mr.embeddedProgramFileCount += len(set.Entries)
		mr.embeddedProgramBytes += totalBytes
	}
	mr.embeddedPatternSets[patternCacheKey] = set
	return set, nil
}

func embedPatternScanRoot(pattern string) string {
	parts := strings.Split(pattern, "/")
	prefix := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.ContainsAny(part, "*?[") {
			break
		}
		prefix = append(prefix, part)
	}
	if len(prefix) == 0 {
		return "."
	}
	return strings.Join(prefix, "/")
}

func embedPatternSelectsFile(pattern string, includeAll bool, file string) bool {
	if matched, _ := path.Match(pattern, file); matched {
		return true
	}
	for directory := path.Dir(file); directory != "."; directory = path.Dir(directory) {
		matched, _ := path.Match(pattern, directory)
		if !matched {
			continue
		}
		if includeAll || !embeddedRelativePathHasHiddenElement(strings.TrimPrefix(file, directory+"/")) {
			return true
		}
	}
	return false
}

func embeddedRelativePathHasHiddenElement(relative string) bool {
	for _, element := range strings.Split(relative, "/") {
		if strings.HasPrefix(element, ".") || strings.HasPrefix(element, "_") {
			return true
		}
	}
	return false
}

// ValidateEmbeddedLogicalFilePath validates the target-neutral portable path
// carried by checked resources and AIR embedded-set entries.
func ValidateEmbeddedLogicalFilePath(path string) error {
	if path == "" {
		return fmt.Errorf("embedded resource path must not be empty")
	}
	if strings.Contains(path, `\`) {
		return fmt.Errorf("embedded resource paths use forward slashes")
	}
	if err := module.CheckFilePath(path); err != nil {
		return fmt.Errorf("invalid embedded resource path: %w", err)
	}
	components := strings.Split(path, "/")
	for index, component := range components {
		if component == "go.mod" {
			return fmt.Errorf("embedded resources must not select a file named go.mod")
		}
		if index < len(components)-1 {
			switch component {
			case ".bzr", ".git", ".hg", ".svn", "ard-out", "vendor":
				return fmt.Errorf("embedded resources must not select reserved directory %q", component)
			}
		}
	}
	return nil
}
