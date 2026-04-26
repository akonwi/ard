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
			if containsCallNamed(assign.Value, "try_op") {
				t.Fatalf("expected value-catch try lowering without try_op marker")
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
			if containsCallNamed(assign.Value, "try_op") {
				t.Fatalf("expected try without catch to lower without try_op marker fallback")
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
			if containsCallNamed(assign.Value, "try_op") {
				t.Fatalf("expected maybe try without catch to lower without try_op marker fallback")
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
			if containsCallNamed(assign.Value, "try_op") {
				t.Fatalf("expected try with unsafe subject to lower without try_op marker fallback")
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
	if containsCallNamed(lowered, "bool_match") {
		t.Fatalf("expected bool match lowering without bool_match marker call")
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
	if containsCallNamed(lowered, "int_match") {
		t.Fatalf("expected int match lowering without int_match marker call")
	}
}

func TestLowerExpressionToBackendIR_IntMatchFallsBackForUnsafeSubject(t *testing.T) {
	intMatch := &checker.IntMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: checker.Int},
		IntCases: map[int]*checker.Block{
			1: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(intMatch)
	if !containsCallNamed(lowered, "int_match") {
		t.Fatalf("expected int match with unsafe subject to keep int_match marker fallback")
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
	if containsCallNamed(lowered, "int_match") {
		t.Fatalf("expected int match without catch-all to lower without int_match marker fallback")
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
	if containsCallNamed(lowered, "conditional_match") {
		t.Fatalf("expected conditional match lowering without conditional_match marker call")
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
	if containsCallNamed(lowered, "conditional_match") {
		t.Fatalf("expected conditional match without catch-all to lower without conditional_match marker fallback")
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
	if containsCallNamed(lowered, "option_match") {
		t.Fatalf("expected option match lowering without option_match marker call")
	}
	if len(ifExpr.Then.Stmts) == 0 {
		t.Fatalf("expected option match then block to include pattern binding")
	}
	assign, ok := ifExpr.Then.Stmts[0].(*backendir.AssignStmt)
	if !ok || assign.Target != "num" {
		t.Fatalf("expected option match then block to start with binding assign to num, got %T", ifExpr.Then.Stmts[0])
	}
}

func TestLowerExpressionToBackendIR_OptionMatchFallsBackForUnsafeSubject(t *testing.T) {
	optionMatch := &checker.OptionMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: checker.MakeMaybe(checker.Int)},
		Some: &checker.Match{
			Pattern: &checker.Identifier{Name: "num"},
			Body:    &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		None: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(optionMatch)
	if !containsCallNamed(lowered, "option_match") {
		t.Fatalf("expected option match with unsafe subject to keep option_match marker fallback")
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
	if containsCallNamed(lowered, "result_match") {
		t.Fatalf("expected result match lowering without result_match marker call")
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

func TestLowerExpressionToBackendIR_ResultMatchFallsBackForUnsafeSubject(t *testing.T) {
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
	if !containsCallNamed(lowered, "result_match") {
		t.Fatalf("expected result match with unsafe subject to keep result_match marker fallback")
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
	if containsCallNamed(lowered, "enum_match") {
		t.Fatalf("expected enum match lowering without enum_match marker call")
	}
}

func TestLowerExpressionToBackendIR_EnumMatchFallsBackForUnsafeSubject(t *testing.T) {
	enumMatch := &checker.EnumMatch{
		Subject: &checker.FunctionCall{Name: "next", ReturnType: &checker.Enum{Name: "Status"}},
		Cases: []*checker.Block{
			&checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 1}}}},
		},
		CatchAll: &checker.Block{Stmts: []checker.Statement{{Expr: &checker.IntLiteral{Value: 0}}}},
	}

	lowered := lowerExpressionToBackendIR(enumMatch)
	if !containsCallNamed(lowered, "enum_match") {
		t.Fatalf("expected enum match with unsafe subject to keep enum_match marker fallback")
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
	if containsCallNamed(lowered, "enum_match") {
		t.Fatalf("expected enum match without catch-all to lower without enum_match marker fallback")
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
	if containsCallNamed(lowered, "union_match") {
		t.Fatalf("expected union match lowering without union_match marker call")
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
	if containsCallNamed(lowered, "union_match") {
		t.Fatalf("expected union match without catch-all to lower without union_match marker fallback")
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
