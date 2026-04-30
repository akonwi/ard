package bytecode_test

import (
	"os"
	"testing"

	"github.com/akonwi/ard/bytecode"
	"github.com/akonwi/ard/bytecode/vm"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestSerializeProgram(t *testing.T) {
	result := parse.Parse([]byte("let val = 2\nval + 3"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}

	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	decoded, err := bytecode.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("Deserialize error: %v", err)
	}

	res, err := vm.New(decoded).Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if got := res.GoValue(); got != 5 {
		t.Fatalf("Expected 5, got %v", got)
	}
}

func TestSerializeProgramWithExterns(t *testing.T) {
	result := parse.Parse([]byte("let val = Int::from_str(\"2\").or(0)\nval + 3"), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	if len(program.Externs) == 0 {
		t.Fatal("expected serialized program to contain extern entries")
	}

	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	decoded, err := bytecode.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("Deserialize error: %v", err)
	}
	if len(decoded.Externs) == 0 {
		t.Fatal("expected decoded program to contain extern entries")
	}

	res, err := vm.New(decoded).Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if got := res.GoValue(); got != 5 {
		t.Fatalf("Expected 5, got %v", got)
	}
}

func TestSerializeProgramWithStructLayouts(t *testing.T) {
	input := `
		struct Person {
			name: Str,
			age: Int,
		}

		let p = Person { name: "Alice", age: 30 }
		p.age
	`
	result := parse.Parse([]byte(input), "test.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("Parse errors: %v", result.Errors[0].Message)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	resolver, err := checker.NewModuleResolver(workingDir)
	if err != nil {
		t.Fatalf("Failed to init module resolver: %v", err)
	}
	c := checker.New("test.ard", result.Program, resolver)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("Diagnostics found: %v", c.Diagnostics())
	}

	emitter := bytecode.NewEmitter()
	program, err := emitter.EmitProgram(c.Module())
	if err != nil {
		t.Fatalf("Emit error: %v", err)
	}
	var layout *bytecode.StructTypeEntry
	for i := range program.Structs {
		if program.Structs[i].Name == "Person" {
			layout = &program.Structs[i]
			break
		}
	}
	if layout == nil {
		t.Fatalf("expected struct layout metadata for Person, got %#v", program.Structs)
	}
	if len(layout.Fields) != 2 {
		t.Fatalf("expected 2 struct fields, got %d", len(layout.Fields))
	}
	if layout.Fields[0].Name != "age" || layout.Fields[1].Name != "name" {
		t.Fatalf("expected sorted struct field names [age name], got %#v", layout.Fields)
	}

	data, err := bytecode.SerializeProgram(program)
	if err != nil {
		t.Fatalf("Serialize error: %v", err)
	}
	decoded, err := bytecode.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("Deserialize error: %v", err)
	}
	var decodedLayout *bytecode.StructTypeEntry
	for i := range decoded.Structs {
		if decoded.Structs[i].Name == "Person" {
			decodedLayout = &decoded.Structs[i]
			break
		}
	}
	if decodedLayout == nil {
		t.Fatalf("expected decoded program to contain struct layout metadata for Person, got %#v", decoded.Structs)
	}
	if len(decodedLayout.Fields) != 2 {
		t.Fatalf("expected decoded struct layout to contain 2 fields, got %d", len(decodedLayout.Fields))
	}

	res, err := vm.New(decoded).Run("main")
	if err != nil {
		t.Fatalf("VM error: %v", err)
	}
	if got := res.GoValue(); got != 30 {
		t.Fatalf("Expected 30, got %v", got)
	}
}
