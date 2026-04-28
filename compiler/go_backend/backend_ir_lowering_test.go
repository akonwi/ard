package go_backend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
	backendir "github.com/akonwi/ard/go_backend/ir"
)

func TestLowerModuleToBackendIR(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Person {
  name: Str,
}

extern fn now() Int = "Now"

fn main() {
  let p = Person{
    name: "Ari",
  }
  let _ = p
  let n = now()
  if n > 0 {
    "ok"
  } else {
    "nope"
  }
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	if got := irModule.PackageName; got != "main" {
		t.Fatalf("expected package main, got %q", got)
	}
	if len(irModule.Decls) == 0 {
		t.Fatalf("expected lowered declarations")
	}

	var hasStruct, hasExtern, hasMain bool
	var mainDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		switch d := decl.(type) {
		case *backendir.StructDecl:
			if d.Name == "Person" {
				hasStruct = true
			}
		case *backendir.FuncDecl:
			if d.Name == "now" && d.IsExtern {
				hasExtern = true
			}
			if d.Name == "main" && !d.IsExtern {
				hasMain = true
				mainDecl = d
			}
		}
	}

	if !hasStruct || !hasExtern || !hasMain {
		t.Fatalf("expected struct=%v extern=%v main=%v", hasStruct, hasExtern, hasMain)
	}
	if mainDecl == nil || mainDecl.Body == nil {
		t.Fatalf("expected main declaration with body")
	}
	if len(mainDecl.Body.Stmts) == 0 {
		t.Fatalf("expected lowered body statements for main")
	}

	var hasAssign, hasIf, hasForIntRange bool
	for _, stmt := range mainDecl.Body.Stmts {
		switch stmt.(type) {
		case *backendir.AssignStmt:
			hasAssign = true
		case *backendir.IfStmt:
			hasIf = true
		case *backendir.ForIntRangeStmt:
			hasForIntRange = true
		}
	}
	if !hasAssign || !hasIf {
		t.Fatalf("expected lowered body to include assign and if statements, got assign=%v if=%v", hasAssign, hasIf)
	}
	if hasForIntRange {
		t.Fatalf("did not expect for-int-range in this program")
	}
}

func TestLowerModuleToBackendIR_ExternBindingFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
extern fn delay(ms: Int) Void = {
  js = "delay"
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "delay" {
			continue
		}
		if fn.ExternBinding != "<unresolved>" {
			t.Fatalf("expected unresolved extern binding placeholder, got %q", fn.ExternBinding)
		}
		return
	}

	t.Fatalf("expected extern function declaration for delay")
}

func TestLowerModuleToBackendIR_LowersControlFlowConstructs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn compute(limit: Int) Int {
  mut acc = 0
  for i in 0..limit {
    acc = acc + i
  }
  if acc > 10 {
    acc
  } else {
    0
  }
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	var computeDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if ok && fn.Name == "compute" {
			computeDecl = fn
			break
		}
	}
	if computeDecl == nil {
		t.Fatalf("expected compute function in lowered IR")
	}
	if computeDecl.Body == nil {
		t.Fatalf("expected compute body in lowered IR")
	}
	if len(computeDecl.Body.Stmts) < 2 {
		t.Fatalf("expected multiple lowered statements, got %d", len(computeDecl.Body.Stmts))
	}
	for _, stmt := range computeDecl.Body.Stmts {
		if _, ok := stmt.(*backendir.ForIntRangeStmt); ok {
			return
		}
	}
	t.Fatalf("expected compute body to include semantic ForIntRangeStmt")
}

func TestLowerModuleToBackendIR_LowersWhileLoopSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn spin(limit: Int) Int {
  mut i = 0
  while i < limit {
    i = i + 1
  }
  i
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "spin" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			if _, ok := stmt.(*backendir.WhileStmt); ok {
				return
			}
		}
		t.Fatalf("expected spin body to include semantic WhileStmt")
	}

	t.Fatalf("expected spin declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersForLoopSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn total(limit: Int) Int {
  mut out = 0
  for mut i = 0; i < limit; i = i + 1 {
    out = out + i
  }
  out
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "total" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			loop, ok := stmt.(*backendir.ForLoopStmt)
			if !ok {
				continue
			}
			if loop.InitName != "i" {
				t.Fatalf("expected for-loop init name i, got %q", loop.InitName)
			}
			if _, ok := loop.Update.(*backendir.AssignStmt); !ok {
				t.Fatalf("expected for-loop update to lower as AssignStmt, got %T", loop.Update)
			}
			return
		}
		t.Fatalf("expected total body to include semantic ForLoopStmt")
	}

	t.Fatalf("expected total declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersForInLoopsSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn run() Int {
  let values = [1, 2]
  let text = "ab"
  let items: [Str: Int] = ["a": 1]
  mut total = 0

  for value, idx in values {
    total = total + value + idx
  }
  for char, idx in text {
    total = total + idx
    let _ = char
  }
  for key, value in items {
    total = total + value
    let _ = key
  }
  total
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	var runDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if ok && fn.Name == "run" {
			runDecl = fn
			break
		}
	}
	if runDecl == nil || runDecl.Body == nil {
		t.Fatalf("expected run declaration with body")
	}

	var hasForInList, hasForInStr, hasForInMap bool
	for _, stmt := range runDecl.Body.Stmts {
		switch stmt.(type) {
		case *backendir.ForInListStmt:
			hasForInList = true
		case *backendir.ForInStrStmt:
			hasForInStr = true
		case *backendir.ForInMapStmt:
			hasForInMap = true
		}
	}
	if !hasForInList || !hasForInStr || !hasForInMap {
		t.Fatalf("expected semantic for-in loops list=%v str=%v map=%v", hasForInList, hasForInStr, hasForInMap)
	}
}

func TestLowerModuleToBackendIR_LowersBreakStatementSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn run(limit: Int) Int {
  mut total = 0
  while total < limit {
    total = total + 1
    break
  }
  total
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "run" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			loop, ok := stmt.(*backendir.WhileStmt)
			if !ok || loop.Body == nil {
				continue
			}
			for _, loopStmt := range loop.Body.Stmts {
				if _, ok := loopStmt.(*backendir.BreakStmt); ok {
					return
				}
			}
		}
		t.Fatalf("expected run body loop to include semantic BreakStmt")
	}

	t.Fatalf("expected run declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersListAndMapLiteralsSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn build() Int {
  let values = [1, 2]
  let items: [Str: Int] = ["a": 1]
  values.size() + items.size()
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "build" || fn.Body == nil {
			continue
		}

		var hasListLiteral, hasMapLiteral bool
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok {
				continue
			}
			switch assign.Value.(type) {
			case *backendir.ListLiteralExpr:
				hasListLiteral = true
			case *backendir.MapLiteralExpr:
				hasMapLiteral = true
			}
		}

		if !hasListLiteral || !hasMapLiteral {
			t.Fatalf("expected semantic list/map literal assignments, got list=%v map=%v", hasListLiteral, hasMapLiteral)
		}
		return
	}

	t.Fatalf("expected build declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersStructAndEnumLiteralsSemantic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box {
  value: Int,
}

enum Status {
  active,
  inactive,
}

fn run() Int {
  let box = Box{value: 1}
  let status = Status::active
  let _ = status
  box.value
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "run" || fn.Body == nil {
			continue
		}
		var hasStructLiteral, hasEnumVariant bool
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok {
				continue
			}
			switch assign.Value.(type) {
			case *backendir.StructLiteralExpr:
				hasStructLiteral = true
			case *backendir.EnumVariantExpr:
				hasEnumVariant = true
			}
		}
		if !hasStructLiteral || !hasEnumVariant {
			t.Fatalf("expected semantic struct/enum literals, got struct=%v enum=%v", hasStructLiteral, hasEnumVariant)
		}
		return
	}

	t.Fatalf("expected run declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersTryOpWithValueCatchAsTryExpr(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/result as Result

fn half(n: Int) Int!Str {
  match n == 0 {
    true => Result::err("zero"),
    false => Result::ok(n / 2),
  }
}

fn compute(n: Int) Int {
  let res = half(n)
  let value = try res -> err {
    0
  }
  value + 1
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "compute" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok || assign.Target != "value" {
				continue
			}
			tryExpr, ok := assign.Value.(*backendir.TryExpr)
			if !ok {
				t.Fatalf("expected try value catch to lower as TryExpr, got %T", assign.Value)
			}
			if strings.TrimSpace(tryExpr.CatchVar) != "err" {
				t.Fatalf("expected lowered try expression to preserve catch var, got %q", tryExpr.CatchVar)
			}
			if tryExpr.Catch == nil || len(tryExpr.Catch.Stmts) == 0 {
				t.Fatalf("expected lowered try expression to retain catch body")
			}
			if _, ok := tryExpr.Catch.Stmts[len(tryExpr.Catch.Stmts)-1].(*backendir.ReturnStmt); !ok {
				t.Fatalf("expected lowered try catch block to finalize with return, got %T", tryExpr.Catch.Stmts[len(tryExpr.Catch.Stmts)-1])
			}
			return
		}
		t.Fatalf("expected compute body to include value assignment from try expression")
	}

	t.Fatalf("expected compute declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersTryOpWithoutCatchAsTryExpr(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/result as Result

fn half(n: Int) Int!Str {
  match n == 0 {
    true => Result::err("zero"),
    false => Result::ok(n / 2),
  }
}

fn compute(n: Int) Int!Str {
  let value = try half(n)
  Result::ok(value + 1)
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "compute" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok || assign.Target != "value" {
				continue
			}
			tryExpr, ok := assign.Value.(*backendir.TryExpr)
			if !ok {
				t.Fatalf("expected try without catch to lower as TryExpr, got %T", assign.Value)
			}
			if tryExpr.Catch != nil {
				t.Fatalf("expected try without catch to lower with nil catch block")
			}
			if strings.TrimSpace(tryExpr.Kind) != "result" {
				t.Fatalf("expected try without catch to preserve result kind, got %q", tryExpr.Kind)
			}
			return
		}
		t.Fatalf("expected compute body to include value assignment from try expression")
	}

	t.Fatalf("expected compute declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersTryOpWithoutCatchMaybeAsTryExpr(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn compute(n: Int) Int? {
  let value = try half(n)
  maybe::some(value + 1)
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "compute" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok || assign.Target != "value" {
				continue
			}
			tryExpr, ok := assign.Value.(*backendir.TryExpr)
			if !ok {
				t.Fatalf("expected try without catch maybe to lower as TryExpr, got %T", assign.Value)
			}
			if tryExpr.Catch != nil {
				t.Fatalf("expected maybe try without catch to lower with nil catch block")
			}
			if strings.TrimSpace(tryExpr.Kind) != "maybe" {
				t.Fatalf("expected maybe try without catch to preserve maybe kind, got %q", tryExpr.Kind)
			}
			return
		}
		t.Fatalf("expected compute body to include value assignment from try expression")
	}

	t.Fatalf("expected compute declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersTryOpWithUnsafeSubjectAsTryExpr(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/result as Result

fn half(n: Int) Int!Str {
  match n == 0 {
    true => Result::err("zero"),
    false => Result::ok(n / 2),
  }
}

fn compute(n: Int) Int {
  let value = try half(n) -> _ {
    0
  }
  value + 1
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if !ok || fn.Name != "compute" || fn.Body == nil {
			continue
		}
		for _, stmt := range fn.Body.Stmts {
			assign, ok := stmt.(*backendir.AssignStmt)
			if !ok || assign.Target != "value" {
				continue
			}
			if _, ok := assign.Value.(*backendir.TryExpr); !ok {
				t.Fatalf("expected try with unsafe subject to lower as TryExpr, got %T", assign.Value)
			}
			return
		}
		t.Fatalf("expected compute body to include value assignment from try expression")
	}

	t.Fatalf("expected compute declaration in lowered IR")
}

func TestLowerExpressionToBackendIR_LowersIfExprSemantic(t *testing.T) {
	ifExpr := &checker.If{
		Condition: &checker.BoolLiteral{Value: true},
		Body: &checker.Block{
			Stmts: []checker.Statement{
				{Expr: &checker.IntLiteral{Value: 1}},
			},
		},
		Else: &checker.Block{
			Stmts: []checker.Statement{
				{Expr: &checker.IntLiteral{Value: 0}},
			},
		},
	}

	lowered := lowerExpressionToBackendIR(ifExpr)
	if loweredIf, ok := lowered.(*backendir.IfExpr); ok {
		if loweredIf.Then == nil || loweredIf.Else == nil {
			t.Fatalf("expected lowered if expression to include then/else blocks")
		}
		if _, ok := loweredIf.Type.(*backendir.PrimitiveType); !ok {
			t.Fatalf("expected lowered if expression type to be primitive int, got %T", loweredIf.Type)
		}
		return
	}

	t.Fatalf("expected checker if expression to lower as backend IfExpr, got %T", lowered)
}

func TestLowerExpressionToBackendIR_LowersScalarMethodsWithoutMarkerCalls(t *testing.T) {
	expressions := []checker.Expression{
		&checker.StrMethod{Subject: &checker.StrLiteral{Value: "abc"}, Kind: checker.StrContains, Args: []checker.Expression{&checker.StrLiteral{Value: "a"}}},
		&checker.StrMethod{Subject: &checker.StrLiteral{Value: "abc"}, Kind: checker.StrTrim},
		&checker.StrMethod{Subject: &checker.StrLiteral{Value: "abc"}, Kind: checker.StrToDyn},
		&checker.IntMethod{Subject: &checker.IntLiteral{Value: 1}, Kind: checker.IntToStr},
		&checker.IntMethod{Subject: &checker.IntLiteral{Value: 1}, Kind: checker.IntToDyn},
		&checker.FloatMethod{Subject: &checker.FloatLiteral{Value: 1.5}, Kind: checker.FloatToInt},
		&checker.FloatMethod{Subject: &checker.FloatLiteral{Value: 1.5}, Kind: checker.FloatToDyn},
		&checker.BoolMethod{Subject: &checker.BoolLiteral{Value: true}, Kind: checker.BoolToStr},
		&checker.BoolMethod{Subject: &checker.BoolLiteral{Value: true}, Kind: checker.BoolToDyn},
	}

	for _, expr := range expressions {
		lowered := lowerExpressionToBackendIR(expr)
		for _, prefix := range []string{"str_method:", "int_method:", "float_method:", "bool_method:"} {
			if containsCallNamePrefix(lowered, prefix) {
				t.Fatalf("expected %T to lower without marker prefix %q", expr, prefix)
			}
		}
	}
}

func TestLowerExpressionToBackendIR_LowersListMapReadMethodsWithoutMarkerCalls(t *testing.T) {
	listSubject := &checker.ListLiteral{
		Elements: []checker.Expression{
			&checker.IntLiteral{Value: 1},
			&checker.IntLiteral{Value: 2},
		},
		ListType: checker.MakeList(checker.Int),
	}
	mapSubject := &checker.MapLiteral{
		Keys: []checker.Expression{
			&checker.StrLiteral{Value: "a"},
		},
		Values: []checker.Expression{
			&checker.IntLiteral{Value: 1},
		},
	}

	expressions := []checker.Expression{
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListSize},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListAt, Args: []checker.Expression{&checker.IntLiteral{Value: 0}}},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListPush, Args: []checker.Expression{&checker.IntLiteral{Value: 3}}, ElementType: checker.Int},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListPrepend, Args: []checker.Expression{&checker.IntLiteral{Value: 0}}, ElementType: checker.Int},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListSet, Args: []checker.Expression{&checker.IntLiteral{Value: 1}, &checker.IntLiteral{Value: 9}}, ElementType: checker.Int},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListSort, Args: []checker.Expression{&checker.FunctionDef{
			Name: "cmp",
			Parameters: []checker.Parameter{
				{Name: "a", Type: checker.Int},
				{Name: "b", Type: checker.Int},
			},
			ReturnType: checker.Bool,
			Body: &checker.Block{Stmts: []checker.Statement{
				{Expr: &checker.BoolLiteral{Value: true}},
			}},
		}}},
		&checker.ListMethod{Subject: listSubject, Kind: checker.ListSwap, Args: []checker.Expression{&checker.IntLiteral{Value: 0}, &checker.IntLiteral{Value: 1}}, ElementType: checker.Int},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapSize},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapKeys},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapHas, Args: []checker.Expression{&checker.StrLiteral{Value: "a"}}},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapGet, Args: []checker.Expression{&checker.StrLiteral{Value: "a"}}, ValueType: checker.Int},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapSet, Args: []checker.Expression{&checker.StrLiteral{Value: "b"}, &checker.IntLiteral{Value: 2}}, ValueType: checker.Int},
		&checker.MapMethod{Subject: mapSubject, Kind: checker.MapDrop, Args: []checker.Expression{&checker.StrLiteral{Value: "a"}}},
	}

	for _, expr := range expressions {
		lowered := lowerExpressionToBackendIR(expr)
		for _, prefix := range []string{"list_method:", "map_method:"} {
			if containsCallNamePrefix(lowered, prefix) {
				t.Fatalf("expected %T to lower without marker prefix %q", expr, prefix)
			}
		}
	}
}

func TestLowerModuleToBackendIR_LowersFunctionLiteralExplicitly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
fn make_adder() fn(Int) Int {
  fn(value: Int) Int {
    value + 1
  }
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend IR lowering to succeed, got error: %v", err)
	}

	var makeAdder *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		if fn, ok := decl.(*backendir.FuncDecl); ok && fn.Name == "make_adder" {
			makeAdder = fn
			break
		}
	}
	if makeAdder == nil || makeAdder.Body == nil || len(makeAdder.Body.Stmts) == 0 {
		t.Fatalf("expected lowered make_adder body")
	}
	ret, ok := makeAdder.Body.Stmts[len(makeAdder.Body.Stmts)-1].(*backendir.ReturnStmt)
	if !ok {
		t.Fatalf("expected final stmt to be ReturnStmt, got %T", makeAdder.Body.Stmts[len(makeAdder.Body.Stmts)-1])
	}
	literal, ok := ret.Value.(*backendir.FuncLiteralExpr)
	if !ok {
		t.Fatalf("expected return value to be FuncLiteralExpr, got %T", ret.Value)
	}
	if len(literal.Params) != 1 || literal.Params[0].Name != "value" {
		t.Fatalf("expected function literal parameter value, got %#v", literal.Params)
	}
	if _, ok := literal.Return.(*backendir.PrimitiveType); !ok {
		t.Fatalf("expected function literal primitive return type, got %T", literal.Return)
	}
}

func TestLowerModuleToBackendIR_PreservesModuleStructLiteralTypeOwner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/http
use ard/maybe

fn main() {
  let req = http::Request{
    method: http::Method::Post,
    url: "http://example.com",
    headers: ["content-type": "text/plain"],
    body: maybe::some("raw text"),
  }
  let _ = req
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend IR lowering to succeed, got error: %v", err)
	}

	var mainDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		if fn, ok := decl.(*backendir.FuncDecl); ok && fn.Name == "main" {
			mainDecl = fn
			break
		}
	}
	if mainDecl == nil || mainDecl.Body == nil || len(mainDecl.Body.Stmts) == 0 {
		t.Fatalf("expected lowered main body")
	}
	assign, ok := mainDecl.Body.Stmts[0].(*backendir.AssignStmt)
	if !ok {
		t.Fatalf("expected first stmt to assign request, got %T", mainDecl.Body.Stmts[0])
	}
	literal, ok := assign.Value.(*backendir.StructLiteralExpr)
	if !ok {
		t.Fatalf("expected request value to be StructLiteralExpr, got %T", assign.Value)
	}
	named, ok := literal.Type.(*backendir.NamedType)
	if !ok {
		t.Fatalf("expected request literal type to be NamedType, got %T", literal.Type)
	}
	if named.Name != "Request" || named.Module != "ard/http" {
		t.Fatalf("expected module-owned Request type, got module=%q name=%q", named.Module, named.Name)
	}
	var methodField backendir.Expr
	for _, field := range literal.Fields {
		if field.Name == "method" {
			methodField = field.Value
			break
		}
	}
	methodVariant, ok := methodField.(*backendir.EnumVariantExpr)
	if !ok {
		t.Fatalf("expected method field to lower to EnumVariantExpr, got %T", methodField)
	}
	methodType, ok := methodVariant.Type.(*backendir.NamedType)
	if !ok {
		t.Fatalf("expected method enum type to be NamedType, got %T", methodVariant.Type)
	}
	if methodType.Name != "Method" || methodType.Module != "ard/http" {
		t.Fatalf("expected module-owned Method type, got module=%q name=%q", methodType.Module, methodType.Name)
	}
}

func TestLowerExpressionToBackendIR_LowersMaybeResultConstructorsExplicitly(t *testing.T) {
	maybeSome := lowerExpressionToBackendIR(&checker.ModuleFunctionCall{
		Module: "ard/maybe",
		Call: &checker.FunctionCall{
			Name:       "some",
			Args:       []checker.Expression{&checker.IntLiteral{Value: 10}},
			ReturnType: checker.MakeMaybe(checker.Int),
		},
	})
	if _, ok := maybeSome.(*backendir.MaybeSomeExpr); !ok {
		t.Fatalf("expected maybe::some to lower to MaybeSomeExpr, got %T", maybeSome)
	}

	maybeNone := lowerExpressionToBackendIR(&checker.ModuleFunctionCall{
		Module: "ard/maybe",
		Call: &checker.FunctionCall{
			Name:       "none",
			ReturnType: checker.MakeMaybe(checker.Int),
		},
	})
	if _, ok := maybeNone.(*backendir.MaybeNoneExpr); !ok {
		t.Fatalf("expected maybe::none to lower to MaybeNoneExpr, got %T", maybeNone)
	}

	resultOk := lowerExpressionToBackendIR(&checker.ModuleFunctionCall{
		Module: "ard/result",
		Call: &checker.FunctionCall{
			Name:       "ok",
			Args:       []checker.Expression{&checker.IntLiteral{Value: 1}},
			ReturnType: checker.MakeResult(checker.Int, checker.Str),
		},
	})
	if _, ok := resultOk.(*backendir.ResultOkExpr); !ok {
		t.Fatalf("expected result::ok to lower to ResultOkExpr, got %T", resultOk)
	}

	resultErr := lowerExpressionToBackendIR(&checker.ModuleFunctionCall{
		Module: "ard/result",
		Call: &checker.FunctionCall{
			Name:       "err",
			Args:       []checker.Expression{&checker.StrLiteral{Value: "bad"}},
			ReturnType: checker.MakeResult(checker.Int, checker.Str),
		},
	})
	if _, ok := resultErr.(*backendir.ResultErrExpr); !ok {
		t.Fatalf("expected result::err to lower to ResultErrExpr, got %T", resultErr)
	}
}

func TestLowerExpressionToBackendIR_LowersMaybeResultMethodsWithoutMarkerCalls(t *testing.T) {
	expressions := []checker.Expression{
		&checker.MaybeMethod{Subject: &checker.Identifier{Name: "maybeVal"}, Kind: checker.MaybeIsSome},
		&checker.MaybeMethod{Subject: &checker.Identifier{Name: "maybeVal"}, Kind: checker.MaybeIsNone},
		&checker.MaybeMethod{Subject: &checker.Identifier{Name: "maybeVal"}, Kind: checker.MaybeExpect, Args: []checker.Expression{&checker.StrLiteral{Value: "missing"}}},
		&checker.MaybeMethod{Subject: &checker.Identifier{Name: "maybeVal"}, Kind: checker.MaybeOr, Args: []checker.Expression{&checker.IntLiteral{Value: 10}}},
		&checker.ResultMethod{Subject: &checker.Identifier{Name: "resultVal"}, Kind: checker.ResultIsOk},
		&checker.ResultMethod{Subject: &checker.Identifier{Name: "resultVal"}, Kind: checker.ResultIsErr},
		&checker.ResultMethod{Subject: &checker.Identifier{Name: "resultVal"}, Kind: checker.ResultExpect, Args: []checker.Expression{&checker.StrLiteral{Value: "bad"}}},
		&checker.ResultMethod{Subject: &checker.Identifier{Name: "resultVal"}, Kind: checker.ResultOr, Args: []checker.Expression{&checker.IntLiteral{Value: 0}}},
	}

	for _, expr := range expressions {
		lowered := lowerExpressionToBackendIR(expr)
		for _, prefix := range []string{"maybe_method:", "result_method:"} {
			if containsCallNamePrefix(lowered, prefix) {
				t.Fatalf("expected %T to lower without marker prefix %q", expr, prefix)
			}
		}
	}
}

func TestLowerExpressionToBackendIR_LowersListCopyExpressionSemantic(t *testing.T) {
	expr := &checker.CopyExpression{
		Expr:  &checker.Identifier{Name: "items"},
		Type_: checker.MakeList(checker.Int),
	}

	lowered := lowerExpressionToBackendIR(expr)
	copyExpr, ok := lowered.(*backendir.CopyExpr)
	if !ok {
		t.Fatalf("expected list copy expression to lower as backend CopyExpr, got %T", lowered)
	}
	if _, ok := copyExpr.Type.(*backendir.ListType); !ok {
		t.Fatalf("expected copy expression type to be list type, got %T", copyExpr.Type)
	}
	if containsCallNamed(lowered, "copy_expr") {
		t.Fatalf("expected list copy expression to lower without copy_expr marker call")
	}
}

func TestLowerExpressionToBackendIR_LowersFiberExecutionWithStringLiterals(t *testing.T) {
	lowered := lowerExpressionToBackendIR(&checker.FiberExecution{})
	call, ok := lowered.(*backendir.CallExpr)
	if !ok {
		t.Fatalf("expected fiber execution to lower as call expr, got %T", lowered)
	}
	callee, ok := call.Callee.(*backendir.IdentExpr)
	if !ok || callee.Name != "fiber_execution" {
		t.Fatalf("expected fiber execution callee to be fiber_execution, got %T %#v", call.Callee, call.Callee)
	}
	if len(call.Args) != 2 {
		t.Fatalf("expected fiber execution lowering to emit 2 args, got %d", len(call.Args))
	}
	for i, arg := range call.Args {
		lit, ok := arg.(*backendir.LiteralExpr)
		if !ok {
			t.Fatalf("expected fiber execution arg[%d] to be literal, got %T", i, arg)
		}
		if lit.Kind != "str" {
			t.Fatalf("expected fiber execution arg[%d] kind to be str, got %q", i, lit.Kind)
		}
	}
}

func TestLowerExpressionToBackendIR_LowersBoolMatchAsIfExpr(t *testing.T) {
	boolMatch := &checker.BoolMatch{
		Subject: &checker.BoolLiteral{Value: true},
		True: &checker.Block{
			Stmts: []checker.Statement{
				{Expr: &checker.IntLiteral{Value: 1}},
			},
		},
		False: &checker.Block{
			Stmts: []checker.Statement{
				{Expr: &checker.IntLiteral{Value: 2}},
			},
		},
	}

	lowered := lowerExpressionToBackendIR(boolMatch)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected bool match to lower as backend IfExpr, got %T", lowered)
	}
}

func TestLowerExpressionToBackendIR_LowersIntMatchAsIfExpr(t *testing.T) {
	intMatch := &checker.IntMatch{
		Subject: &checker.Identifier{Name: "num"},
		IntCases: map[int]*checker.Block{
			1: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.StrLiteral{Value: "one"}}}},
		},
		RangeCases: map[checker.IntRange]*checker.Block{
			{Start: 2, End: 4}: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.StrLiteral{Value: "few"}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.StrLiteral{Value: "many"}}}},
	}

	lowered := lowerExpressionToBackendIR(intMatch)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected int match to lower as backend IfExpr, got %T", lowered)
	}
}

func TestLowerExpressionToBackendIR_LowersIntMatchWithUnsafeSubjectSemanticSingleEval(t *testing.T) {
	subjectCall := &checker.FunctionCall{Name: "next", ReturnType: checker.Int}
	intMatch := &checker.IntMatch{
		Subject: subjectCall,
		IntCases: map[int]*checker.Block{
			1: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		RangeCases: map[checker.IntRange]*checker.Block{
			{Start: 2, End: 4}: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 2}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(intMatch)
	block, ok := lowered.(*backendir.BlockExpr)
	if !ok {
		t.Fatalf("expected int match with unsafe subject to lower as backend BlockExpr, got %T", lowered)
	}
	if len(block.Setup) != 1 {
		t.Fatalf("expected int match BlockExpr to hoist subject in single setup statement, got %d statements", len(block.Setup))
	}
	assign, ok := block.Setup[0].(*backendir.AssignStmt)
	if !ok {
		t.Fatalf("expected int match subject hoist to be AssignStmt, got %T", block.Setup[0])
	}
	if strings.TrimSpace(assign.Target) == "" {
		t.Fatalf("expected int match subject hoist target to be a synthetic temp name, got empty string")
	}
	if call, ok := assign.Value.(*backendir.CallExpr); !ok {
		t.Fatalf("expected int match subject hoist value to be call expression, got %T", assign.Value)
	} else if ident, ok := call.Callee.(*backendir.IdentExpr); !ok || ident.Name != "next" {
		t.Fatalf("expected int match subject hoist value to invoke next(), got %#v", call.Callee)
	}
	if _, ok := block.Value.(*backendir.IfExpr); !ok {
		t.Fatalf("expected int match BlockExpr value to be IfExpr chain, got %T", block.Value)
	}
	if countCallsNamed(block.Value, "next") != 0 {
		t.Fatalf("expected int match unsafe subject to be evaluated once via temp; found %d duplicate evaluations of next() in body", countCallsNamed(block.Value, "next"))
	}
}

func TestLowerExpressionToBackendIR_LowersIntMatchWithoutCatchAllWithNonExhaustivePanic(t *testing.T) {
	intMatch := &checker.IntMatch{
		Subject: &checker.Identifier{Name: "num"},
		IntCases: map[int]*checker.Block{
			1: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
	}

	lowered := lowerExpressionToBackendIR(intMatch)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected int match without catch-all to lower as backend IfExpr, got %T", lowered)
	}
	if !containsCallNamed(lowered, "panic") {
		t.Fatalf("expected int match without catch-all to include non-exhaustive panic path")
	}
}

func TestLowerExpressionToBackendIR_LowersConditionalMatchAsIfExpr(t *testing.T) {
	conditional := &checker.ConditionalMatch{
		Cases: []checker.ConditionalCase{
			{
				Condition: &checker.BoolLiteral{Value: true},
				Body:      &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
			},
			{
				Condition: &checker.BoolLiteral{Value: false},
				Body:      &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 2}}}},
			},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 3}}}},
	}

	lowered := lowerExpressionToBackendIR(conditional)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected conditional match to lower as backend IfExpr, got %T", lowered)
	}
}

func TestLowerExpressionToBackendIR_LowersConditionalMatchWithoutCatchAllWithNonExhaustivePanic(t *testing.T) {
	conditional := &checker.ConditionalMatch{
		Cases: []checker.ConditionalCase{
			{
				Condition: &checker.BoolLiteral{Value: true},
				Body:      &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
			},
		},
	}

	lowered := lowerExpressionToBackendIR(conditional)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected conditional match without catch-all to lower as backend IfExpr, got %T", lowered)
	}
	if !containsCallNamed(lowered, "panic") {
		t.Fatalf("expected conditional match without catch-all to include non-exhaustive panic path")
	}
}

func TestLowerExpressionToBackendIR_LowersOptionMatchAsIfExpr(t *testing.T) {
	optionMatch := &checker.OptionMatch{
		Subject: &checker.Identifier{Name: "opt"},
		Some: &checker.Match{
			Pattern: &checker.Identifier{Name: "num"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		None: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(optionMatch)
	ifExpr, ok := lowered.(*backendir.IfExpr)
	if !ok {
		t.Fatalf("expected option match to lower as backend IfExpr, got %T", lowered)
	}
	if len(ifExpr.Then.Stmts) == 0 {
		t.Fatalf("expected option match then block to include pattern binding")
	}
	assign, ok := ifExpr.Then.Stmts[0].(*backendir.AssignStmt)
	if !ok || assign.Target != "num" {
		t.Fatalf("expected option match then block to start with binding assign to num, got %T", ifExpr.Then.Stmts[0])
	}
}

func TestLowerExpressionToBackendIR_LowersOptionMatchWithUnsafeSubjectSemanticSingleEval(t *testing.T) {
	optionMatch := &checker.OptionMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: checker.MakeMaybe(checker.Int)},
		Some: &checker.Match{
			Pattern: &checker.Identifier{Name: "num"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		None: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(optionMatch)
	block, ok := lowered.(*backendir.BlockExpr)
	if !ok {
		t.Fatalf("expected option match with unsafe subject to lower as backend BlockExpr, got %T", lowered)
	}
	if len(block.Setup) != 1 {
		t.Fatalf("expected option match BlockExpr to hoist subject in single setup statement, got %d statements", len(block.Setup))
	}
	if _, ok := block.Setup[0].(*backendir.AssignStmt); !ok {
		t.Fatalf("expected option match subject hoist to be AssignStmt, got %T", block.Setup[0])
	}
	if _, ok := block.Value.(*backendir.IfExpr); !ok {
		t.Fatalf("expected option match BlockExpr value to be IfExpr, got %T", block.Value)
	}
	if countCallsNamed(block.Value, "next") != 0 {
		t.Fatalf("expected option match unsafe subject to be evaluated once via temp; found %d duplicate evaluations of next() in body", countCallsNamed(block.Value, "next"))
	}
}

func TestLowerExpressionToBackendIR_LowersResultMatchAsIfExpr(t *testing.T) {
	resultMatch := &checker.ResultMatch{
		Subject: &checker.Identifier{Name: "res"},
		Ok: &checker.Match{
			Pattern: &checker.Identifier{Name: "ok"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		Err: &checker.Match{
			Pattern: &checker.Identifier{Name: "err"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
		},
	}

	lowered := lowerExpressionToBackendIR(resultMatch)
	ifExpr, ok := lowered.(*backendir.IfExpr)
	if !ok {
		t.Fatalf("expected result match to lower as backend IfExpr, got %T", lowered)
	}
	if len(ifExpr.Then.Stmts) == 0 || len(ifExpr.Else.Stmts) == 0 {
		t.Fatalf("expected result match branches to include pattern bindings")
	}
	if assign, ok := ifExpr.Then.Stmts[0].(*backendir.AssignStmt); !ok || assign.Target != "ok" {
		t.Fatalf("expected result match ok branch to start with binding assign to ok, got %T", ifExpr.Then.Stmts[0])
	}
	if assign, ok := ifExpr.Else.Stmts[0].(*backendir.AssignStmt); !ok || assign.Target != "err" {
		t.Fatalf("expected result match err branch to start with binding assign to err, got %T", ifExpr.Else.Stmts[0])
	}
}

func TestLowerExpressionToBackendIR_LowersResultMatchWithUnsafeSubjectSemanticSingleEval(t *testing.T) {
	resultMatch := &checker.ResultMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: checker.MakeResult(checker.Int, checker.Str)},
		Ok: &checker.Match{
			Pattern: &checker.Identifier{Name: "ok"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		Err: &checker.Match{
			Pattern: &checker.Identifier{Name: "err"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
		},
	}

	lowered := lowerExpressionToBackendIR(resultMatch)
	block, ok := lowered.(*backendir.BlockExpr)
	if !ok {
		t.Fatalf("expected result match with unsafe subject to lower as backend BlockExpr, got %T", lowered)
	}
	if len(block.Setup) != 1 {
		t.Fatalf("expected result match BlockExpr to hoist subject in single setup statement, got %d statements", len(block.Setup))
	}
	if _, ok := block.Setup[0].(*backendir.AssignStmt); !ok {
		t.Fatalf("expected result match subject hoist to be AssignStmt, got %T", block.Setup[0])
	}
	if _, ok := block.Value.(*backendir.IfExpr); !ok {
		t.Fatalf("expected result match BlockExpr value to be IfExpr, got %T", block.Value)
	}
	if countCallsNamed(block.Value, "next") != 0 {
		t.Fatalf("expected result match unsafe subject to be evaluated once via temp; found %d duplicate evaluations of next() in body", countCallsNamed(block.Value, "next"))
	}
}

func TestLowerExpressionToBackendIR_LowersEnumMatchAsIfExpr(t *testing.T) {
	enumMatch := &checker.EnumMatch{
		Subject: &checker.Identifier{Name: "status"},
		Cases: []*checker.Block{
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 2}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(enumMatch)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected enum match to lower as backend IfExpr, got %T", lowered)
	}
}

func TestLowerExpressionToBackendIR_LowersEnumMatchWithUnsafeSubjectSemanticSingleEval(t *testing.T) {
	enumMatch := &checker.EnumMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: &checker.Enum{Name: "Status"}},
		Cases: []*checker.Block{
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 2}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(enumMatch)
	block, ok := lowered.(*backendir.BlockExpr)
	if !ok {
		t.Fatalf("expected enum match with unsafe subject to lower as backend BlockExpr, got %T", lowered)
	}
	if len(block.Setup) != 1 {
		t.Fatalf("expected enum match BlockExpr to hoist subject in single setup statement, got %d statements", len(block.Setup))
	}
	if _, ok := block.Setup[0].(*backendir.AssignStmt); !ok {
		t.Fatalf("expected enum match subject hoist to be AssignStmt, got %T", block.Setup[0])
	}
	if _, ok := block.Value.(*backendir.IfExpr); !ok {
		t.Fatalf("expected enum match BlockExpr value to be IfExpr, got %T", block.Value)
	}
	if countCallsNamed(block.Value, "next") != 0 {
		t.Fatalf("expected enum match unsafe subject to be evaluated once via temp; found %d duplicate evaluations of next() in body", countCallsNamed(block.Value, "next"))
	}
}

func TestLowerExpressionToBackendIR_LowersEnumMatchWithoutCatchAllWithNonExhaustivePanic(t *testing.T) {
	enumMatch := &checker.EnumMatch{
		Subject: &checker.Identifier{Name: "status"},
		Cases: []*checker.Block{
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
	}

	lowered := lowerExpressionToBackendIR(enumMatch)
	if _, ok := lowered.(*backendir.IfExpr); !ok {
		t.Fatalf("expected enum match without catch-all to lower as backend IfExpr, got %T", lowered)
	}
	if !containsCallNamed(lowered, "panic") {
		t.Fatalf("expected enum match without catch-all to include non-exhaustive panic path")
	}
}

func TestLowerExpressionToBackendIR_LowersUnionMatchSemantic(t *testing.T) {
	intCase := &checker.Match{
		Pattern: &checker.Identifier{Name: "num"},
		Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
	}
	strCase := &checker.Match{
		Pattern: &checker.Identifier{Name: "text"},
		Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 2}}}},
	}
	unionMatch := &checker.UnionMatch{
		Subject: &checker.Identifier{Name: "value"},
		TypeCases: map[string]*checker.Match{
			checker.Int.String(): intCase,
			checker.Str.String(): strCase,
		},
		TypeCasesByType: map[checker.Type]*checker.Match{
			checker.Int: intCase,
			checker.Str: strCase,
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(unionMatch)
	unionExpr, ok := lowered.(*backendir.UnionMatchExpr)
	if !ok {
		t.Fatalf("expected union match to lower as backend UnionMatchExpr, got %T", lowered)
	}
	if len(unionExpr.Cases) != 2 {
		t.Fatalf("expected union match lowering with 2 cases, got %d", len(unionExpr.Cases))
	}
}

func TestLowerExpressionToBackendIR_LowersUnionMatchWithoutCatchAllWithNonExhaustivePanic(t *testing.T) {
	intCase := &checker.Match{
		Pattern: &checker.Identifier{Name: "num"},
		Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
	}
	unionMatch := &checker.UnionMatch{
		Subject: &checker.Identifier{Name: "value"},
		TypeCases: map[string]*checker.Match{
			checker.Int.String(): intCase,
		},
		TypeCasesByType: map[checker.Type]*checker.Match{
			checker.Int: intCase,
		},
	}

	lowered := lowerExpressionToBackendIR(unionMatch)
	unionExpr, ok := lowered.(*backendir.UnionMatchExpr)
	if !ok {
		t.Fatalf("expected union match without catch-all to lower as backend UnionMatchExpr, got %T", lowered)
	}
	if unionExpr.CatchAll == nil {
		t.Fatalf("expected union match without catch-all to synthesize non-exhaustive panic catch-all block")
	}
	if !containsCallNamedInBlock(unionExpr.CatchAll, "panic") {
		t.Fatalf("expected union match synthesized catch-all to include panic call")
	}
}

func TestLowerExpressionToBackendIR_ComprehensiveNodeCoverage(t *testing.T) {
	block := &checker.Block{
		Stmts: []checker.Statement{
			{Expr: &checker.IntLiteral{Value: 1}},
		},
	}
	match := &checker.Match{
		Pattern: &checker.Identifier{Name: "value"},
		Body:    block,
	}

	expressions := []checker.Expression{
		block,
		&checker.Enum{Name: "Status"},
		&checker.Union{Name: "NumOrStr", Types: []checker.Type{checker.Int, checker.Str}},
		&checker.StrMethod{Subject: &checker.StrLiteral{Value: "a"}, Kind: checker.StrContains, Args: []checker.Expression{&checker.StrLiteral{Value: "b"}}},
		&checker.IntMethod{Subject: &checker.IntLiteral{Value: 42}, Kind: checker.IntToStr},
		&checker.FloatMethod{Subject: &checker.FloatLiteral{Value: 1.25}, Kind: checker.FloatToInt},
		&checker.BoolMethod{Subject: &checker.BoolLiteral{Value: true}, Kind: checker.BoolToStr},
		&checker.ListMethod{Subject: &checker.ListLiteral{Elements: []checker.Expression{&checker.IntLiteral{Value: 1}}}, Kind: checker.ListSize},
		&checker.MapMethod{Subject: &checker.MapLiteral{}, Kind: checker.MapSize},
		&checker.MaybeMethod{Subject: &checker.Identifier{Name: "m"}, Kind: checker.MaybeIsSome},
		&checker.ResultMethod{Subject: &checker.Identifier{Name: "r"}, Kind: checker.ResultIsOk},
		&checker.EnumVariant{Discriminant: 2, Variant: 1, EnumType: &checker.Enum{Name: "Color"}},
		&checker.OptionMatch{Subject: &checker.Identifier{Name: "opt"}, Some: match, None: block},
		&checker.ResultMatch{Subject: &checker.Identifier{Name: "res"}, Ok: match, Err: match},
		&checker.BoolMatch{Subject: &checker.BoolLiteral{Value: true}, True: block, False: block},
		&checker.IntMatch{
			Subject:    &checker.IntLiteral{Value: 1},
			IntCases:   map[int]*checker.Block{1: block},
			RangeCases: map[checker.IntRange]*checker.Block{{Start: 2, End: 4}: block},
			CatchAll:   block,
		},
		&checker.EnumMatch{Subject: &checker.Identifier{Name: "enumValue"}, Cases: []*checker.Block{block}, CatchAll: block},
		&checker.UnionMatch{Subject: &checker.Identifier{Name: "unionValue"}, TypeCases: map[string]*checker.Match{"Int": match}, CatchAll: block},
		&checker.ConditionalMatch{Cases: []checker.ConditionalCase{{Condition: &checker.BoolLiteral{Value: true}, Body: block}}, CatchAll: block},
		&checker.TryOp{OkType: checker.Int, ErrType: checker.Str, CatchVar: "err", CatchBlock: block, Kind: checker.TryResult},
		&checker.CopyExpression{Expr: &checker.IntLiteral{Value: 3}, Type_: checker.Int},
		&checker.FiberStart{},
		&checker.FiberEval{},
		&checker.FiberExecution{},
		&checker.FunctionDef{Name: "anon", ReturnType: checker.Int, Body: block},
		&checker.ExternalFunctionDef{Name: "externFn", ReturnType: checker.Int, ExternalBinding: "ExternFn"},
	}

	for _, expr := range expressions {
		lowered := lowerExpressionToBackendIR(expr)
		if containsCallNamed(lowered, "unknown_expr") {
			t.Fatalf("expected expression %T to lower without unknown_expr fallback", expr)
		}
	}
}

func TestLowerNonProducingToBackendIR_ComprehensiveNodeCoverage(t *testing.T) {
	block := &checker.Block{
		Stmts: []checker.Statement{
			{Expr: &checker.IntLiteral{Value: 1}},
		},
	}
	nonProducingNodes := []checker.NonProducing{
		&checker.ForIntRange{Cursor: "v", Index: "i", Start: &checker.IntLiteral{Value: 0}, End: &checker.IntLiteral{Value: 10}, Body: block},
		&checker.ForInStr{Cursor: "c", Index: "i", Value: &checker.StrLiteral{Value: "abc"}, Body: block},
		&checker.ForInList{Cursor: "c", Index: "i", List: &checker.ListLiteral{Elements: []checker.Expression{&checker.IntLiteral{Value: 1}}}, Body: block},
		&checker.ForInMap{Key: "k", Val: "v", Map: &checker.MapLiteral{}, Body: block},
		&checker.ForLoop{
			Init:      &checker.VariableDef{Name: "i", Value: &checker.IntLiteral{Value: 0}},
			Condition: &checker.IntLess{Left: &checker.Identifier{Name: "i"}, Right: &checker.IntLiteral{Value: 3}},
			Update:    &checker.Reassignment{Target: &checker.Identifier{Name: "i"}, Value: &checker.IntAddition{Left: &checker.Identifier{Name: "i"}, Right: &checker.IntLiteral{Value: 1}}},
			Body:      block,
		},
		&checker.WhileLoop{Condition: &checker.BoolLiteral{Value: true}, Body: block},
		&checker.Union{Name: "Value", Types: []checker.Type{checker.Int, checker.Str}},
		&checker.ExternType{Name_: "Handle"},
	}

	for _, node := range nonProducingNodes {
		stmts := lowerNonProducingToBackendIR(node)
		if len(stmts) == 0 {
			t.Fatalf("expected non-empty statement lowering for %T", node)
		}
		for _, stmt := range stmts {
			if containsCallNamedInStmt(stmt, "nonproducing_stmt") {
				t.Fatalf("expected %T to lower without nonproducing fallback", node)
			}
		}
	}
}

func TestLowerCheckerTypeToBackendIR_LowersUnionToNamedType(t *testing.T) {
	// Union types in signature/value positions must lower to backend IR
	// NamedType so emitted Go signatures reference the concrete union
	// interface name rather than any/Dynamic. This guarantees that
	// union-typed parameters/returns/variables are emitted as `Shape`
	// (the union interface) instead of being erased to `any`.
	pointerUnion := &checker.Union{
		Name:  "Shape",
		Types: []checker.Type{checker.Int, checker.Str},
	}
	loweredPtr := lowerCheckerTypeToBackendIR(pointerUnion)
	namedPtr, ok := loweredPtr.(*backendir.NamedType)
	if !ok {
		t.Fatalf("expected *checker.Union to lower to *backendir.NamedType, got %T (%+v)", loweredPtr, loweredPtr)
	}
	if namedPtr.Name != "Shape" {
		t.Fatalf("expected pointer-union NamedType.Name to be %q, got %q", "Shape", namedPtr.Name)
	}
	if len(namedPtr.Args) != 0 {
		t.Fatalf("expected union NamedType to have no type args, got %d", len(namedPtr.Args))
	}

	valueUnion := checker.Union{
		Name:  "Value",
		Types: []checker.Type{checker.Int, checker.Str},
	}
	loweredVal := lowerCheckerTypeToBackendIR(valueUnion)
	namedVal, ok := loweredVal.(*backendir.NamedType)
	if !ok {
		t.Fatalf("expected checker.Union (value) to lower to *backendir.NamedType, got %T (%+v)", loweredVal, loweredVal)
	}
	if namedVal.Name != "Value" {
		t.Fatalf("expected value-union NamedType.Name to be %q, got %q", "Value", namedVal.Name)
	}

	// Functions whose parameters or return types reference a union must
	// surface the union as a NamedType in the lowered FuncType so the
	// emitter can reach the concrete union interface instead of the
	// Dynamic erasure.
	funcType := &checker.FunctionDef{
		Name: "label",
		Parameters: []checker.Parameter{
			{Name: "shape", Type: pointerUnion},
		},
		ReturnType: pointerUnion,
	}
	loweredFunc := lowerCheckerTypeToBackendIR(funcType)
	fnType, ok := loweredFunc.(*backendir.FuncType)
	if !ok {
		t.Fatalf("expected function type to lower to *backendir.FuncType, got %T", loweredFunc)
	}
	if len(fnType.Params) != 1 {
		t.Fatalf("expected lowered FuncType to have one param, got %d", len(fnType.Params))
	}
	paramNamed, ok := fnType.Params[0].(*backendir.NamedType)
	if !ok || paramNamed.Name != "Shape" {
		t.Fatalf("expected union-typed parameter to lower to NamedType{Name: %q}, got %T (%+v)", "Shape", fnType.Params[0], fnType.Params[0])
	}
	returnNamed, ok := fnType.Return.(*backendir.NamedType)
	if !ok || returnNamed.Name != "Shape" {
		t.Fatalf("expected union-typed return to lower to NamedType{Name: %q}, got %T (%+v)", "Shape", fnType.Return, fnType.Return)
	}
}

func TestLowerCheckerTypeToBackendIR_LowersTraitToTraitType(t *testing.T) {
	trait := &checker.Trait{Name: "ToString"}
	lowered := lowerCheckerTypeToBackendIR(trait)
	traitType, ok := lowered.(*backendir.TraitType)
	if !ok {
		t.Fatalf("expected trait to lower to *backendir.TraitType, got %T (%+v)", lowered, lowered)
	}
	if traitType.Name != "ToString" {
		t.Fatalf("expected lowered trait name %q, got %q", "ToString", traitType.Name)
	}
}

func TestLowerModuleToBackendIR_LowersTraitCallArgsExplicitly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/io

fn main() {
  io::print("ok")
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend IR lowering to succeed, got error: %v", err)
	}
	var mainDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		if fn, ok := decl.(*backendir.FuncDecl); ok && fn.Name == "main" {
			mainDecl = fn
			break
		}
	}
	if mainDecl == nil || mainDecl.Body == nil || len(mainDecl.Body.Stmts) == 0 {
		t.Fatalf("expected lowered main func body with statements")
	}
	exprStmt, ok := mainDecl.Body.Stmts[0].(*backendir.ExprStmt)
	if !ok {
		t.Fatalf("expected first main-body stmt to be *backendir.ExprStmt, got %T", mainDecl.Body.Stmts[0])
	}
	call, ok := exprStmt.Value.(*backendir.CallExpr)
	if !ok {
		t.Fatalf("expected entrypoint expr to lower to *backendir.CallExpr, got %T", exprStmt.Value)
	}
	if len(call.Args) != 1 {
		t.Fatalf("expected one call arg, got %d", len(call.Args))
	}
	coerce, ok := call.Args[0].(*backendir.TraitCoerceExpr)
	if !ok {
		t.Fatalf("expected trait-typed call arg to lower to *backendir.TraitCoerceExpr, got %T", call.Args[0])
	}
	traitType, ok := coerce.Type.(*backendir.TraitType)
	if !ok {
		t.Fatalf("expected coercion type to be *backendir.TraitType, got %T", coerce.Type)
	}
	if traitType.Name != "ToString" {
		t.Fatalf("expected coercion trait name %q, got %q", "ToString", traitType.Name)
	}
}

func TestLowerUnionAndExternTypeDeclToBackendIR(t *testing.T) {
	unionDecl := lowerUnionDeclToBackendIR(&checker.Union{
		Name:  "Value",
		Types: []checker.Type{checker.Int, checker.Str},
	})
	union, ok := unionDecl.(*backendir.UnionDecl)
	if !ok {
		t.Fatalf("expected union decl, got %T", unionDecl)
	}
	if union.Name != "Value" || len(union.Types) != 2 {
		t.Fatalf("expected union decl Value with 2 types, got %+v", union)
	}

	externDecl := lowerExternTypeDeclToBackendIR(&checker.ExternType{Name_: "Handle"})
	externType, ok := externDecl.(*backendir.ExternTypeDecl)
	if !ok {
		t.Fatalf("expected extern type decl, got %T", externDecl)
	}
	if externType.Name != "Handle" {
		t.Fatalf("expected extern type name Handle, got %q", externType.Name)
	}
}

func TestLowerModuleToBackendIR_EncodesMutableParamReferenceSemantics(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box { value: Int }

fn set_box(mut box: Box) {
  box.value = 2
}

fn bump(mut value: Int) {
  value = value + 1
}

fn append_one(mut values: [Int]) {
  values.push(1)
}

fn main() {
  mut box = Box{value: 1}
  set_box(box)

  mut value = 1
  bump(value)

  mut values = [1]
  append_one(values)
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend IR lowering to succeed, got error: %v", err)
	}

	funcs := map[string]*backendir.FuncDecl{}
	for _, decl := range irModule.Decls {
		if fn, ok := decl.(*backendir.FuncDecl); ok {
			funcs[fn.Name] = fn
		}
	}
	setBox := funcs["set_box"]
	if setBox == nil || len(setBox.Params) != 1 || !setBox.Params[0].ByRef {
		t.Fatalf("expected set_box param to lower as by-ref, got %#v", setBox)
	}
	bump := funcs["bump"]
	if bump == nil || len(bump.Params) != 1 || bump.Params[0].ByRef {
		t.Fatalf("expected bump param to remain by-value, got %#v", bump)
	}
	appendOne := funcs["append_one"]
	if appendOne == nil || len(appendOne.Params) != 1 || !appendOne.Params[0].ByRef {
		t.Fatalf("expected append_one param to lower as by-ref, got %#v", appendOne)
	}
	mainDecl := funcs["main"]
	if mainDecl == nil || mainDecl.Body == nil {
		t.Fatalf("expected lowered main func body")
	}
	var callArgs []backendir.Expr
	for _, stmt := range mainDecl.Body.Stmts {
		exprStmt, ok := stmt.(*backendir.ExprStmt)
		if !ok {
			continue
		}
		call, ok := exprStmt.Value.(*backendir.CallExpr)
		if !ok {
			continue
		}
		callee, ok := call.Callee.(*backendir.IdentExpr)
		if !ok {
			continue
		}
		if callee.Name == "set_box" || callee.Name == "append_one" {
			if len(call.Args) != 1 {
				t.Fatalf("expected one arg for %s", callee.Name)
			}
			if _, ok := call.Args[0].(*backendir.AddressOfExpr); !ok {
				t.Fatalf("expected %s arg to lower to AddressOfExpr, got %T", callee.Name, call.Args[0])
			}
		}
		if callee.Name == "bump" {
			if len(call.Args) != 1 {
				t.Fatalf("expected one arg for bump")
			}
			if _, ok := call.Args[0].(*backendir.AddressOfExpr); ok {
				t.Fatalf("expected bump arg to remain by-value, got %T", call.Args[0])
			}
		}
		callArgs = call.Args
	}
	_ = callArgs
}

func TestLowerModuleToBackendIR_CarriesStructAndEnumMethods(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box { value: Int }

impl Box {
  fn get() Int {
    self.value
  }
}

enum Light { red, green }

impl Light {
  fn rank() Int {
    match self {
      Light::red => 0,
      Light::green => 1,
    }
  }
}

fn main() {
  let box = Box{value: 1}
  let _ = box.get()
  let light = Light::green
  let _ = light.rank()
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend IR lowering to succeed, got error: %v", err)
	}

	var boxDecl *backendir.StructDecl
	var lightDecl *backendir.EnumDecl
	for _, decl := range irModule.Decls {
		switch typed := decl.(type) {
		case *backendir.StructDecl:
			if typed.Name == "Box" {
				boxDecl = typed
			}
		case *backendir.EnumDecl:
			if typed.Name == "Light" {
				lightDecl = typed
			}
		}
	}
	if boxDecl == nil {
		t.Fatalf("expected lowered IR to include Box struct decl")
	}
	if lightDecl == nil {
		t.Fatalf("expected lowered IR to include Light enum decl")
	}
	if len(boxDecl.Methods) != 1 || boxDecl.Methods[0] == nil || boxDecl.Methods[0].Name != "get" {
		t.Fatalf("expected Box struct decl to carry get method, got %#v", boxDecl.Methods)
	}
	if got := strings.TrimSpace(boxDecl.Methods[0].ReceiverName); got != "self" {
		t.Fatalf("expected Box.get receiver name %q, got %q", "self", got)
	}
	if len(lightDecl.Methods) != 1 || lightDecl.Methods[0] == nil || lightDecl.Methods[0].Name != "rank" {
		t.Fatalf("expected Light enum decl to carry rank method, got %#v", lightDecl.Methods)
	}
}

func TestLowerModuleToBackendIR_CollectsOrphanUnionDecls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	// Union type aliases (`type X = A | B`) are registered in scope by
	// the checker but never surface as a Statement entry in the program.
	// They are observable only through type metadata of signatures and
	// variables that reference them. Backend IR lowering must still
	// surface those orphan unions as IR `UnionDecl`s so the Go emitter
	// can produce the corresponding interface declaration.
	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

fn label(shape: Shape) Str {
  match shape {
    Square => "square",
    Circle => "circle",
  }
}

fn main() {
  let s = Square{size: 1}
  let kind = label(s)
  let _ = kind
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	var unionDecl *backendir.UnionDecl
	for _, decl := range irModule.Decls {
		if u, ok := decl.(*backendir.UnionDecl); ok && u.Name == "Shape" {
			unionDecl = u
			break
		}
	}
	if unionDecl == nil {
		t.Fatalf("expected backend IR module to contain UnionDecl for Shape, got decls: %#v", irModule.Decls)
	}
	if len(unionDecl.Types) != 2 {
		t.Fatalf("expected Shape union decl to retain 2 member types, got %d: %#v", len(unionDecl.Types), unionDecl.Types)
	}

	// Ensure orphan collection is idempotent: a second invocation of the
	// lowerer on the same module should still produce exactly one
	// UnionDecl for Shape (no duplicates).
	again, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected second backend ir lowering to succeed, got error: %v", err)
	}
	count := 0
	for _, decl := range again.Decls {
		if u, ok := decl.(*backendir.UnionDecl); ok && u.Name == "Shape" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one UnionDecl for Shape after orphan collection, got %d", count)
	}
}

func TestLowerModuleToBackendIR_EmitsPackageVariableDecls(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "utils.ard", `
let answer = 42
`)

	irModule, err := lowerModuleToBackendIR(module, "utils", false)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	for _, decl := range irModule.Decls {
		if variable, ok := decl.(*backendir.VarDecl); ok {
			if variable.Name != "answer" {
				t.Fatalf("expected variable name answer, got %q", variable.Name)
			}
			return
		}
	}

	t.Fatalf("expected package variable declaration in lowered IR")
}

func TestLowerModuleToBackendIR_LowersMemberReassignmentStmt(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
struct Box {
  value: Int,
}

fn update() Int {
  mut box = Box{value: 1}
  box.value = 2
  box.value
}
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}

	var updateDecl *backendir.FuncDecl
	for _, decl := range irModule.Decls {
		fn, ok := decl.(*backendir.FuncDecl)
		if ok && fn.Name == "update" {
			updateDecl = fn
			break
		}
	}
	if updateDecl == nil || updateDecl.Body == nil {
		t.Fatalf("expected lowered update function body")
	}

	for _, stmt := range updateDecl.Body.Stmts {
		memberAssign, ok := stmt.(*backendir.MemberAssignStmt)
		if !ok {
			continue
		}
		if memberAssign.Field != "value" {
			t.Fatalf("expected member assignment field value, got %q", memberAssign.Field)
		}
		return
	}

	t.Fatalf("expected member reassignment to lower as MemberAssignStmt")
}

// TestLowerModuleToBackendIR_ComprehensiveNodeCoverage exercises module
// lowering with a checker module that spans the full range of declaration
// and expression node kinds covered by the migration: struct/enum/union/
// extern-type declarations, package-level variables, regular and extern
// functions, and the migrated try/match expression families. It asserts
// that the resulting module remains structurally valid.
func TestLowerModuleToBackendIR_ComprehensiveNodeCoverage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ard.toml"), []byte("name = \"demo\"\nard = \">= 0.1.0\"\n"), 0o644); err != nil {
		t.Fatalf("failed to write ard.toml: %v", err)
	}

	module := checkedModuleFromSource(t, dir, "main.ard", `
use ard/maybe

struct Square { size: Int }
struct Circle { radius: Int }

type Shape = Square | Circle

extern type Handle

enum Color { Red, Blue, Green }

extern fn now() Int = "Now"

fn divide(a: Int, b: Int) Int!Str {
  match b == 0 {
    true => Result::err("division by zero"),
    false => Result::ok(a / b),
  }
}

fn classify(n: Int) Str {
  match n {
    0 => "zero",
    1..3 => "small",
    _ => "many",
  }
}

fn describe(c: Color) Str {
  match c {
    Color::Red => "red",
    Color::Blue => "blue",
    _ => "other",
  }
}

fn label(shape: Shape) Str {
  match shape {
    Square => "square",
    Circle => "circle",
  }
}

fn half(n: Int) Int? {
  match n > 0 {
    true => maybe::some(n / 2),
    false => maybe::none(),
  }
}

fn first_some(opt: Int?) Int {
  match opt {
    v => v,
    _ => 0,
  }
}

fn unwrap_result(value: Int!Str) Int {
  match value {
    ok(v) => v,
    err(msg) => 0,
  }
}

fn render(a: Int, b: Int) Str!Str {
  let value = try divide(a, b)
  Result::ok(value.to_str())
}

fn final_message() Str {
  try render(1, 0) -> err {
    "bad: {err}"
  }
}

fn use_maybe() Int {
  let value = try half(2) -> _ {
    0
  }
  value + 1
}

let a = final_message()
let b = use_maybe()
let c = classify(2)
let d = describe(Color::Blue)
let e = label(Square{size: 1})
let f = first_some(maybe::some(7))
let g = unwrap_result(Result::ok(11))
let h = now()
`)

	irModule, err := lowerModuleToBackendIR(module, "main", true)
	if err != nil {
		t.Fatalf("expected backend ir lowering to succeed, got error: %v", err)
	}
	if irModule == nil {
		t.Fatalf("expected non-nil backend ir module")
	}

	// Guard against regressions in declaration coverage by asserting the
	// module retains the expected declaration kinds. Package-level let
	// bindings are kept in the entrypoint block (not promoted to
	// VarDecls) when lowered with entrypoint=true, so we cover VarDecl
	// separately by re-lowering the same module with entrypoint=false.
	var hasStruct, hasEnum, hasUnion, hasExternType, hasExtern, hasFunc bool
	for _, decl := range irModule.Decls {
		switch d := decl.(type) {
		case *backendir.StructDecl:
			if d.Name == "Square" || d.Name == "Circle" {
				hasStruct = true
			}
		case *backendir.EnumDecl:
			if d.Name == "Color" {
				hasEnum = true
			}
		case *backendir.UnionDecl:
			if d.Name == "Shape" {
				hasUnion = true
			}
		case *backendir.ExternTypeDecl:
			if d.Name == "Handle" {
				hasExternType = true
			}
		case *backendir.FuncDecl:
			if d.IsExtern && d.Name == "now" {
				hasExtern = true
			}
			if !d.IsExtern && d.Name == "render" {
				hasFunc = true
			}
		}
	}
	if !hasStruct || !hasEnum || !hasUnion || !hasExternType || !hasExtern || !hasFunc {
		t.Fatalf("expected comprehensive declaration coverage; got struct=%v enum=%v union=%v externType=%v extern=%v func=%v",
			hasStruct, hasEnum, hasUnion, hasExternType, hasExtern, hasFunc)
	}

	// Re-lower with entrypoint=false to cover the package-variable
	// declaration path.
	nonEntrypointIR, err := lowerModuleToBackendIR(module, "demo_pkg", false)
	if err != nil {
		t.Fatalf("expected non-entrypoint backend ir lowering to succeed, got error: %v", err)
	}
	hasVar := false
	for _, decl := range nonEntrypointIR.Decls {
		if vd, ok := decl.(*backendir.VarDecl); ok {
			if vd.Name == "a" || vd.Name == "b" || vd.Name == "c" {
				hasVar = true
				break
			}
		}
	}
	if !hasVar {
		t.Fatalf("expected non-entrypoint lowering to surface package variable decls")
	}
}

func containsCallNamedInStmt(stmt backendir.Stmt, name string) bool {
	switch s := stmt.(type) {
	case *backendir.ExprStmt:
		return containsCallNamed(s.Value, name)
	case *backendir.BreakStmt:
		return false
	case *backendir.AssignStmt:
		return containsCallNamed(s.Value, name)
	case *backendir.MemberAssignStmt:
		return containsCallNamed(s.Subject, name) || containsCallNamed(s.Value, name)
	case *backendir.ForIntRangeStmt:
		return containsCallNamed(s.Start, name) || containsCallNamed(s.End, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.ForLoopStmt:
		return containsCallNamed(s.InitValue, name) || containsCallNamed(s.Cond, name) || containsCallNamedInStmt(s.Update, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.ForInStrStmt:
		return containsCallNamed(s.Value, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.ForInListStmt:
		return containsCallNamed(s.List, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.ForInMapStmt:
		return containsCallNamed(s.Map, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.WhileStmt:
		return containsCallNamed(s.Cond, name) || containsCallNamedInBlock(s.Body, name)
	case *backendir.ReturnStmt:
		return containsCallNamed(s.Value, name)
	case *backendir.IfStmt:
		return containsCallNamed(s.Cond, name) || containsCallNamedInBlock(s.Then, name) || containsCallNamedInBlock(s.Else, name)
	default:
		return false
	}
}

func containsCallNamedInBlock(block *backendir.Block, name string) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if containsCallNamedInStmt(stmt, name) {
			return true
		}
	}
	return false
}

func countCallsNamed(expr backendir.Expr, name string) int {
	switch e := expr.(type) {
	case nil:
		return 0
	case *backendir.CallExpr:
		count := 0
		if ident, ok := e.Callee.(*backendir.IdentExpr); ok && ident.Name == name {
			count++
		}
		count += countCallsNamed(e.Callee, name)
		for _, arg := range e.Args {
			count += countCallsNamed(arg, name)
		}
		return count
	case *backendir.SelectorExpr:
		return countCallsNamed(e.Subject, name)
	case *backendir.ListLiteralExpr:
		count := 0
		for _, element := range e.Elements {
			count += countCallsNamed(element, name)
		}
		return count
	case *backendir.MapLiteralExpr:
		count := 0
		for _, entry := range e.Entries {
			count += countCallsNamed(entry.Key, name)
			count += countCallsNamed(entry.Value, name)
		}
		return count
	case *backendir.StructLiteralExpr:
		count := 0
		for _, field := range e.Fields {
			count += countCallsNamed(field.Value, name)
		}
		return count
	case *backendir.IfExpr:
		return countCallsNamed(e.Cond, name) + countCallsNamedInBlock(e.Then, name) + countCallsNamedInBlock(e.Else, name)
	case *backendir.UnionMatchExpr:
		count := countCallsNamed(e.Subject, name) + countCallsNamedInBlock(e.CatchAll, name)
		for _, matchCase := range e.Cases {
			count += countCallsNamedInBlock(matchCase.Body, name)
		}
		return count
	case *backendir.TryExpr:
		return countCallsNamed(e.Subject, name) + countCallsNamedInBlock(e.Catch, name)
	case *backendir.PanicExpr:
		count := countCallsNamed(e.Message, name)
		if name == "panic" {
			count++
		}
		return count
	case *backendir.CopyExpr:
		return countCallsNamed(e.Value, name)
	case *backendir.BlockExpr:
		count := countCallsNamed(e.Value, name)
		for _, stmt := range e.Setup {
			count += countCallsNamedInStmt(stmt, name)
		}
		return count
	default:
		return 0
	}
}

func countCallsNamedInBlock(block *backendir.Block, name string) int {
	if block == nil {
		return 0
	}
	count := 0
	for _, stmt := range block.Stmts {
		count += countCallsNamedInStmt(stmt, name)
	}
	return count
}

func countCallsNamedInStmt(stmt backendir.Stmt, name string) int {
	switch s := stmt.(type) {
	case *backendir.ExprStmt:
		return countCallsNamed(s.Value, name)
	case *backendir.AssignStmt:
		return countCallsNamed(s.Value, name)
	case *backendir.MemberAssignStmt:
		return countCallsNamed(s.Subject, name) + countCallsNamed(s.Value, name)
	case *backendir.ReturnStmt:
		return countCallsNamed(s.Value, name)
	case *backendir.IfStmt:
		return countCallsNamed(s.Cond, name) + countCallsNamedInBlock(s.Then, name) + countCallsNamedInBlock(s.Else, name)
	case *backendir.ForIntRangeStmt:
		return countCallsNamed(s.Start, name) + countCallsNamed(s.End, name) + countCallsNamedInBlock(s.Body, name)
	case *backendir.ForLoopStmt:
		return countCallsNamed(s.InitValue, name) + countCallsNamed(s.Cond, name) + countCallsNamedInStmt(s.Update, name) + countCallsNamedInBlock(s.Body, name)
	case *backendir.ForInStrStmt:
		return countCallsNamed(s.Value, name) + countCallsNamedInBlock(s.Body, name)
	case *backendir.ForInListStmt:
		return countCallsNamed(s.List, name) + countCallsNamedInBlock(s.Body, name)
	case *backendir.ForInMapStmt:
		return countCallsNamed(s.Map, name) + countCallsNamedInBlock(s.Body, name)
	case *backendir.WhileStmt:
		return countCallsNamed(s.Cond, name) + countCallsNamedInBlock(s.Body, name)
	default:
		return 0
	}
}

func containsCallNamed(expr backendir.Expr, name string) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *backendir.CallExpr:
		if ident, ok := e.Callee.(*backendir.IdentExpr); ok && ident.Name == name {
			return true
		}
		if containsCallNamed(e.Callee, name) {
			return true
		}
		for _, arg := range e.Args {
			if containsCallNamed(arg, name) {
				return true
			}
		}
		return false
	case *backendir.SelectorExpr:
		return containsCallNamed(e.Subject, name)
	case *backendir.ListLiteralExpr:
		for _, element := range e.Elements {
			if containsCallNamed(element, name) {
				return true
			}
		}
		return false
	case *backendir.MapLiteralExpr:
		for _, entry := range e.Entries {
			if containsCallNamed(entry.Key, name) || containsCallNamed(entry.Value, name) {
				return true
			}
		}
		return false
	case *backendir.StructLiteralExpr:
		for _, field := range e.Fields {
			if containsCallNamed(field.Value, name) {
				return true
			}
		}
		return false
	case *backendir.EnumVariantExpr:
		return false
	case *backendir.IfExpr:
		if containsCallNamed(e.Cond, name) || containsCallNamedInBlock(e.Then, name) || containsCallNamedInBlock(e.Else, name) {
			return true
		}
		return false
	case *backendir.UnionMatchExpr:
		if containsCallNamed(e.Subject, name) || containsCallNamedInBlock(e.CatchAll, name) {
			return true
		}
		for _, matchCase := range e.Cases {
			if containsCallNamedInBlock(matchCase.Body, name) {
				return true
			}
		}
		return false
	case *backendir.TryExpr:
		return containsCallNamed(e.Subject, name) || containsCallNamedInBlock(e.Catch, name)
	case *backendir.PanicExpr:
		if name == "panic" {
			return true
		}
		return containsCallNamed(e.Message, name)
	case *backendir.CopyExpr:
		return containsCallNamed(e.Value, name)
	case *backendir.BlockExpr:
		for _, stmt := range e.Setup {
			if containsCallNamedInStmt(stmt, name) {
				return true
			}
		}
		return containsCallNamed(e.Value, name)
	default:
		return false
	}
}

func containsCallNamePrefix(expr backendir.Expr, prefix string) bool {
	switch e := expr.(type) {
	case nil:
		return false
	case *backendir.CallExpr:
		if ident, ok := e.Callee.(*backendir.IdentExpr); ok && strings.HasPrefix(ident.Name, prefix) {
			return true
		}
		if containsCallNamePrefix(e.Callee, prefix) {
			return true
		}
		for _, arg := range e.Args {
			if containsCallNamePrefix(arg, prefix) {
				return true
			}
		}
		return false
	case *backendir.SelectorExpr:
		return containsCallNamePrefix(e.Subject, prefix)
	case *backendir.ListLiteralExpr:
		for _, element := range e.Elements {
			if containsCallNamePrefix(element, prefix) {
				return true
			}
		}
		return false
	case *backendir.MapLiteralExpr:
		for _, entry := range e.Entries {
			if containsCallNamePrefix(entry.Key, prefix) || containsCallNamePrefix(entry.Value, prefix) {
				return true
			}
		}
		return false
	case *backendir.StructLiteralExpr:
		for _, field := range e.Fields {
			if containsCallNamePrefix(field.Value, prefix) {
				return true
			}
		}
		return false
	case *backendir.IfExpr:
		return containsCallNamePrefix(e.Cond, prefix) || containsCallNamePrefixInBlock(e.Then, prefix) || containsCallNamePrefixInBlock(e.Else, prefix)
	case *backendir.UnionMatchExpr:
		if containsCallNamePrefix(e.Subject, prefix) || containsCallNamePrefixInBlock(e.CatchAll, prefix) {
			return true
		}
		for _, matchCase := range e.Cases {
			if containsCallNamePrefixInBlock(matchCase.Body, prefix) {
				return true
			}
		}
		return false
	case *backendir.TryExpr:
		return containsCallNamePrefix(e.Subject, prefix) || containsCallNamePrefixInBlock(e.Catch, prefix)
	case *backendir.PanicExpr:
		return strings.HasPrefix("panic", prefix) || containsCallNamePrefix(e.Message, prefix)
	case *backendir.CopyExpr:
		return containsCallNamePrefix(e.Value, prefix)
	case *backendir.BlockExpr:
		for _, stmt := range e.Setup {
			if containsCallNamePrefixInStmt(stmt, prefix) {
				return true
			}
		}
		return containsCallNamePrefix(e.Value, prefix)
	default:
		return false
	}
}

func containsCallNamePrefixInBlock(block *backendir.Block, prefix string) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Stmts {
		if containsCallNamePrefixInStmt(stmt, prefix) {
			return true
		}
	}
	return false
}

func containsCallNamePrefixInStmt(stmt backendir.Stmt, prefix string) bool {
	switch s := stmt.(type) {
	case *backendir.ExprStmt:
		return containsCallNamePrefix(s.Value, prefix)
	case *backendir.AssignStmt:
		return containsCallNamePrefix(s.Value, prefix)
	case *backendir.MemberAssignStmt:
		return containsCallNamePrefix(s.Subject, prefix) || containsCallNamePrefix(s.Value, prefix)
	case *backendir.ForIntRangeStmt:
		return containsCallNamePrefix(s.Start, prefix) || containsCallNamePrefix(s.End, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.ForLoopStmt:
		return containsCallNamePrefix(s.InitValue, prefix) || containsCallNamePrefix(s.Cond, prefix) || containsCallNamePrefixInStmt(s.Update, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.ForInStrStmt:
		return containsCallNamePrefix(s.Value, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.ForInListStmt:
		return containsCallNamePrefix(s.List, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.ForInMapStmt:
		return containsCallNamePrefix(s.Map, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.WhileStmt:
		return containsCallNamePrefix(s.Cond, prefix) || containsCallNamePrefixInBlock(s.Body, prefix)
	case *backendir.ReturnStmt:
		return containsCallNamePrefix(s.Value, prefix)
	case *backendir.IfStmt:
		return containsCallNamePrefix(s.Cond, prefix) || containsCallNamePrefixInBlock(s.Then, prefix) || containsCallNamePrefixInBlock(s.Else, prefix)
	default:
		return false
	}
}
