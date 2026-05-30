package lsp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
	"go.lsp.dev/protocol"
)

const completionPlaceholder = "__ard_completion__"

type completionKind int

const (
	completionInstance completionKind = iota + 1
	completionStatic
)

type completionContext struct {
	kind   completionKind
	prefix string
	sepEnd int
	offset int
}

func computeCompletions(source string, filePath string, position protocol.Position) []protocol.CompletionItem {
	ctx, ok := completionContextAt(source, position)
	if !ok {
		return []protocol.CompletionItem{}
	}

	parseSource := source[:ctx.sepEnd] + completionPlaceholder + source[ctx.offset:]
	placeholderPoint := offsetToParsePoint(parseSource, ctx.sepEnd+1)
	prog := parseAndCache(parseSource, filePath)
	if prog == nil {
		return []protocol.CompletionItem{}
	}

	expr := findInStmts(prog.Statements, placeholderPoint)
	if expr == nil {
		return []protocol.CompletionItem{}
	}

	items := []protocol.CompletionItem{}
	switch ctx.kind {
	case completionInstance:
		if ip, ok := expr.(*parse.InstanceProperty); ok {
			items = instanceCompletionItems(ip.Target, prog, filePath)
		}
	case completionStatic:
		if sp, ok := expr.(*parse.StaticProperty); ok {
			items = staticCompletionItems(simpleExprName(sp.Target), prog, filePath)
		}
	}

	return withCompletionTextEdits(items, ctx, position)
}

func completionContextAt(source string, position protocol.Position) (completionContext, bool) {
	offset := parsePointToOffset(source, lspPositionToParsePoint(position))
	if offset < 0 || offset > len(source) {
		return completionContext{}, false
	}

	identStart := offset
	for identStart > 0 && isTypeIdentPart(source[identStart-1]) {
		identStart--
	}
	prefix := source[identStart:offset]
	sepEnd := identStart
	if sepEnd >= 1 && source[sepEnd-1] == '.' {
		return completionContext{kind: completionInstance, prefix: prefix, sepEnd: sepEnd, offset: offset}, true
	}
	if sepEnd >= 2 && source[sepEnd-2:sepEnd] == "::" {
		return completionContext{kind: completionStatic, prefix: prefix, sepEnd: sepEnd, offset: offset}, true
	}
	return completionContext{}, false
}

func withCompletionTextEdits(items []protocol.CompletionItem, ctx completionContext, position protocol.Position) []protocol.CompletionItem {
	if len(items) == 0 {
		return items
	}
	startChar := position.Character
	if prefixLen := uint32(len(ctx.prefix)); prefixLen <= startChar {
		startChar -= prefixLen
	} else {
		startChar = 0
	}
	editRange := protocol.Range{
		Start: protocol.Position{Line: position.Line, Character: startChar},
		End:   position,
	}
	for i := range items {
		newText := items[i].InsertText
		if newText == "" {
			newText = items[i].Label
		}
		items[i].TextEdit = &protocol.TextEdit{Range: editRange, NewText: newText}
	}
	return items
}

func formatCompletionMethodDetail(sig *hoverMethodSignature) string {
	prefix := "fn"
	if sig.Mutates {
		prefix += " mut"
	}
	ret := normalizeDisplayType(sig.ReturnType)
	if ret == "" {
		ret = "Void"
	}
	return fmt.Sprintf("%s (%s) %s", prefix, formatHoverParams(sig.Params), ret)
}

func formatCompletionStaticFunctionDetail(sig *hoverStaticFunctionSignature) string {
	ret := normalizeDisplayType(sig.ReturnType)
	if ret == "" {
		ret = "Void"
	}
	return fmt.Sprintf("fn (%s) %s", formatHoverParams(sig.Params), ret)
}

func offsetToParsePoint(source string, offset int) parse.Point {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	row := 1
	lineStart := 0
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			row++
			lineStart = i + 1
		}
	}
	return parse.Point{Row: row, Col: offset - lineStart + 1}
}

func instanceCompletionItems(target parse.Expression, prog *parse.Program, filePath string) []protocol.CompletionItem {
	ownerType := normalizeDisplayType(inferExprType(target))
	if ownerType == "" || ownerType == "?" {
		return []protocol.CompletionItem{}
	}

	items := map[string]protocol.CompletionItem{}
	add := func(item protocol.CompletionItem) {
		if item.Label == "" {
			return
		}
		if _, exists := items[item.Label]; !exists {
			items[item.Label] = item
		}
	}

	for _, name := range builtinMethodNames(ownerType) {
		if sig := builtinMethodSignature(ownerType, name); sig != nil {
			qualifyMethodSignature(sig, prog, filePath)
			add(protocol.CompletionItem{
				Label:      name,
				Kind:       protocol.CompletionItemKindMethod,
				Detail:     formatCompletionMethodDetail(sig),
				InsertText: name,
			})
		}
	}

	for _, item := range localInstanceCompletionItems(ownerType, prog, filePath) {
		add(item)
	}
	for _, item := range importedInstanceCompletionItems(ownerType, prog, filePath) {
		add(item)
	}

	return sortedCompletionItems(items)
}

func localInstanceCompletionItems(ownerType string, prog *parse.Program, filePath string) []protocol.CompletionItem {
	base := completionTypeBase(ownerType)
	if base == "" || strings.Contains(base, "::") {
		return nil
	}
	items := []protocol.CompletionItem{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *parse.StructDefinition:
			if s.Name.Name != base {
				continue
			}
			for _, field := range s.Fields {
				fieldType := qualifyTypeDisplay(typeDeclString(field.Type), prog, filePath)
				items = append(items, protocol.CompletionItem{
					Label:      field.Name.Name,
					Kind:       protocol.CompletionItemKindField,
					Detail:     fieldType,
					InsertText: field.Name.Name,
				})
			}
		case *parse.ImplBlock:
			if s.Target.Name != base {
				continue
			}
			for i := range s.Methods {
				sig := methodDeclSignature(ownerType, &s.Methods[i])
				qualifyMethodSignature(sig, prog, filePath)
				items = append(items, protocol.CompletionItem{
					Label:      s.Methods[i].Name,
					Kind:       protocol.CompletionItemKindMethod,
					Detail:     formatCompletionMethodDetail(sig),
					InsertText: s.Methods[i].Name,
				})
			}
		case *parse.TraitImplementation:
			if s.ForType.Name != base {
				continue
			}
			for i := range s.Methods {
				sig := methodDeclSignature(ownerType, &s.Methods[i])
				qualifyMethodSignature(sig, prog, filePath)
				items = append(items, protocol.CompletionItem{
					Label:      s.Methods[i].Name,
					Kind:       protocol.CompletionItemKindMethod,
					Detail:     formatCompletionMethodDetail(sig),
					InsertText: s.Methods[i].Name,
				})
			}
		}
	}
	return items
}

func importedInstanceCompletionItems(ownerType string, prog *parse.Program, filePath string) []protocol.CompletionItem {
	importedType, ok := importedTypeForDisplay(ownerType, prog, filePath)
	if !ok {
		return nil
	}
	items := []protocol.CompletionItem{}
	switch def := importedType.(type) {
	case *checker.StructDef:
		fieldNames := make([]string, 0, len(def.Fields))
		for name := range def.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)
		for _, name := range fieldNames {
			fieldType := substituteImportedGenericDisplay(checkerTypeString(def.Fields[name]), importedType, ownerType)
			fieldType = qualifyTypeDisplay(fieldType, prog, filePath)
			items = append(items, protocol.CompletionItem{
				Label:      name,
				Kind:       protocol.CompletionItemKindField,
				Detail:     fieldType,
				InsertText: name,
			})
		}
		items = append(items, checkerMethodCompletionItems(ownerType, def.Methods, importedType, prog, filePath)...)
	case *checker.Enum:
		items = append(items, checkerMethodCompletionItems(ownerType, def.Methods, importedType, prog, filePath)...)
	case *checker.Trait:
		for _, method := range def.GetMethods() {
			m := method
			sig := checkerMethodSignature(ownerType, &m)
			substituteImportedGenericSignature(sig, importedType, ownerType)
			qualifyMethodSignature(sig, prog, filePath)
			items = append(items, protocol.CompletionItem{
				Label:      method.Name,
				Kind:       protocol.CompletionItemKindMethod,
				Detail:     formatCompletionMethodDetail(sig),
				InsertText: method.Name,
			})
		}
	}
	return items
}

func checkerMethodCompletionItems(ownerType string, methods map[string]*checker.FunctionDef, importedType checker.Type, prog *parse.Program, filePath string) []protocol.CompletionItem {
	items := []protocol.CompletionItem{}
	names := make([]string, 0, len(methods))
	for name := range methods {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sig := checkerMethodSignature(ownerType, methods[name])
		substituteImportedGenericSignature(sig, importedType, ownerType)
		qualifyMethodSignature(sig, prog, filePath)
		items = append(items, protocol.CompletionItem{
			Label:      name,
			Kind:       protocol.CompletionItemKindMethod,
			Detail:     formatCompletionMethodDetail(sig),
			InsertText: name,
		})
	}
	return items
}

func staticCompletionItems(target string, prog *parse.Program, filePath string) []protocol.CompletionItem {
	if target == "" || prog == nil {
		return []protocol.CompletionItem{}
	}
	items := map[string]protocol.CompletionItem{}
	add := func(item protocol.CompletionItem) {
		if item.Label == "" {
			return
		}
		if _, exists := items[item.Label]; !exists {
			items[item.Label] = item
		}
	}

	if mod, ok := importedModuleForAlias(target, prog, filePath); ok {
		for _, item := range moduleCompletionItems(target, mod, prog, filePath) {
			add(item)
		}
	}
	for _, item := range localStaticCompletionItems(target, prog, filePath) {
		add(item)
	}
	for _, item := range importedTypeStaticCompletionItems(target, prog, filePath) {
		add(item)
	}

	return sortedCompletionItems(items)
}

func moduleCompletionItems(alias string, mod checker.Module, prog *parse.Program, filePath string) []protocol.CompletionItem {
	if mod == nil || mod.Program() == nil {
		return nil
	}
	items := []protocol.CompletionItem{}
	for _, stmt := range mod.Program().Statements {
		if stmt.Expr != nil {
			switch fn := stmt.Expr.(type) {
			case *checker.FunctionDef:
				if strings.Contains(fn.Name, "::") || mod.Get(fn.Name).IsZero() {
					continue
				}
				items = append(items, staticFunctionCompletionItem(alias, fn.Name, fn.Type(), prog, filePath))
			case *checker.ExternalFunctionDef:
				if strings.Contains(fn.Name, "::") || mod.Get(fn.Name).IsZero() {
					continue
				}
				items = append(items, staticFunctionCompletionItem(alias, fn.Name, fn.Type(), prog, filePath))
			}
		}
		if stmt.Stmt != nil {
			switch s := stmt.Stmt.(type) {
			case *checker.VariableDef:
				if mod.Get(s.Name).IsZero() {
					continue
				}
				typeLabel := qualifyTypeDisplay(checkerTypeString(s.Type()), prog, filePath)
				items = append(items, protocol.CompletionItem{
					Label:      s.Name,
					Kind:       protocol.CompletionItemKindVariable,
					Detail:     typeLabel,
					InsertText: s.Name,
				})
			case *checker.StructDef:
				if mod.Get(s.Name).IsZero() {
					continue
				}
				items = append(items, protocol.CompletionItem{Label: s.Name, Kind: protocol.CompletionItemKindStruct, Detail: alias + "::" + s.Name, InsertText: s.Name})
			case *checker.Enum:
				if mod.Get(s.Name).IsZero() {
					continue
				}
				items = append(items, protocol.CompletionItem{Label: s.Name, Kind: protocol.CompletionItemKindEnum, Detail: alias + "::" + s.Name, InsertText: s.Name})
			case *checker.Union:
				if mod.Get(s.Name).IsZero() {
					continue
				}
				items = append(items, protocol.CompletionItem{Label: s.Name, Kind: protocol.CompletionItemKindClass, Detail: alias + "::" + s.Name, InsertText: s.Name})
			case *checker.ExternType:
				if mod.Get(s.Name_).IsZero() {
					continue
				}
				items = append(items, protocol.CompletionItem{Label: s.Name_, Kind: protocol.CompletionItemKindClass, Detail: alias + "::" + s.Name_, InsertText: s.Name_})
			}
		}
	}
	return items
}

func importedTypeStaticCompletionItems(target string, prog *parse.Program, filePath string) []protocol.CompletionItem {
	importedType, ok := importedTypeForDisplay(target, prog, filePath)
	if !ok {
		return nil
	}
	alias, memberName, _ := importedTypeDisplayParts(target)
	mod, ok := importedModuleForAlias(alias, prog, filePath)
	if !ok {
		return nil
	}
	items := []protocol.CompletionItem{}
	for _, stmt := range mod.Program().Statements {
		if stmt.Expr == nil {
			continue
		}
		var fnType checker.Type
		var fnName string
		switch fn := stmt.Expr.(type) {
		case *checker.FunctionDef:
			fnName = fn.Name
			fnType = fn.Type()
		case *checker.ExternalFunctionDef:
			fnName = fn.Name
			fnType = fn.Type()
		}
		prefix := memberName + "::"
		if !strings.HasPrefix(fnName, prefix) || mod.Get(fnName).IsZero() {
			continue
		}
		label := strings.TrimPrefix(fnName, prefix)
		items = append(items, staticFunctionCompletionItemForType(target, label, fnType, importedType, target, prog, filePath))
	}
	if enumDef, ok := importedType.(*checker.Enum); ok {
		for _, variant := range enumDef.Values {
			items = append(items, protocol.CompletionItem{
				Label:      variant.Name,
				Kind:       protocol.CompletionItemKindEnumMember,
				Detail:     qualifyTypeDisplay(target, prog, filePath),
				InsertText: variant.Name,
			})
		}
	}
	return items
}

func localStaticCompletionItems(target string, prog *parse.Program, filePath string) []protocol.CompletionItem {
	base := completionTypeBase(target)
	if base == "" || strings.Contains(base, "::") {
		return nil
	}
	items := []protocol.CompletionItem{}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *parse.StaticFunctionDeclaration:
			if simpleExprName(s.Path.Target) != base {
				continue
			}
			if prop := simpleExprName(s.Path.Property); prop != "" {
				items = append(items, staticParseFunctionCompletionItem(target, prop, &s.FunctionDeclaration, prog, filePath))
			}
		case *parse.EnumDefinition:
			if s.Name != base {
				continue
			}
			for _, variant := range s.Variants {
				items = append(items, protocol.CompletionItem{
					Label:      variant.Name,
					Kind:       protocol.CompletionItemKindEnumMember,
					Detail:     target,
					InsertText: variant.Name,
				})
			}
		}
	}
	return items
}

func staticFunctionCompletionItem(qualifier string, label string, fnType checker.Type, prog *parse.Program, filePath string) protocol.CompletionItem {
	return staticFunctionCompletionItemForType(qualifier, label, fnType, nil, "", prog, filePath)
}

func staticFunctionCompletionItemForType(qualifier string, label string, fnType checker.Type, importedType checker.Type, ownerType string, prog *parse.Program, filePath string) protocol.CompletionItem {
	sig := checkerStaticFunctionSignature(qualifier, label, fnType)
	if sig != nil {
		if importedType != nil {
			substituteImportedGenericStaticSignature(sig, importedType, ownerType)
		}
		qualifyStaticFunctionSignature(sig, prog, filePath)
		return protocol.CompletionItem{
			Label:      label,
			Kind:       protocol.CompletionItemKindFunction,
			Detail:     formatCompletionStaticFunctionDetail(sig),
			InsertText: label,
		}
	}
	return protocol.CompletionItem{Label: label, Kind: protocol.CompletionItemKindFunction, InsertText: label}
}

func substituteImportedGenericStaticSignature(sig *hoverStaticFunctionSignature, importedType checker.Type, ownerType string) {
	for i := range sig.Params {
		sig.Params[i].Type = substituteImportedGenericDisplay(sig.Params[i].Type, importedType, ownerType)
	}
	sig.ReturnType = substituteImportedGenericDisplay(sig.ReturnType, importedType, ownerType)
}

func staticParseFunctionCompletionItem(qualifier string, label string, fd *parse.FunctionDeclaration, prog *parse.Program, filePath string) protocol.CompletionItem {
	sig := parseStaticFunctionSignature(qualifier, fd)
	sig.Name = label
	qualifyStaticFunctionSignature(sig, prog, filePath)
	return protocol.CompletionItem{Label: label, Kind: protocol.CompletionItemKindFunction, Detail: formatCompletionStaticFunctionDetail(sig), InsertText: label}
}

func sortedCompletionItems(items map[string]protocol.CompletionItem) []protocol.CompletionItem {
	labels := make([]string, 0, len(items))
	for label := range items {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	out := make([]protocol.CompletionItem, 0, len(labels))
	for _, label := range labels {
		out = append(out, items[label])
	}
	return out
}

func completionTypeBase(typeName string) string {
	typeName = strings.TrimSpace(normalizeDisplayType(typeName))
	typeName = strings.TrimSuffix(typeName, "?")
	if idx := strings.Index(typeName, "<"); idx >= 0 {
		typeName = typeName[:idx]
	}
	return typeName
}

func builtinMethodNames(ownerType string) []string {
	ownerType = normalizeDisplayType(ownerType)
	switch ownerType {
	case "Str":
		return []string{"at", "contains", "is_empty", "replace", "replace_all", "size", "split", "starts_with", "ends_with", "to_str", "to_dyn", "trim"}
	case "Int":
		return []string{"to_str", "to_dyn"}
	case "Float":
		return []string{"to_str", "to_dyn", "to_int"}
	case "Bool":
		return []string{"to_str", "to_dyn"}
	}
	if _, ok := listElementType(ownerType); ok {
		return []string{"at", "prepend", "push", "set", "size", "sort", "swap"}
	}
	if _, _, ok := mapEntryTypes(ownerType); ok {
		return []string{"get", "keys", "set", "drop", "has", "size"}
	}
	if _, ok := maybeInnerType(ownerType); ok {
		return []string{"expect", "is_none", "is_some", "or", "map", "and_then"}
	}
	if _, _, ok := resultTypes(ownerType); ok {
		return []string{"expect", "or", "is_ok", "is_err", "map", "map_err", "and_then"}
	}
	return nil
}
