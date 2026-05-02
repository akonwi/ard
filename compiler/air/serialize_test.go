package air_test

import (
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestSerializeProgram(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int {
			let values = [20, 22]
			values.at(0) + values.at(1)
		}
	`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse error: %s", result.Errors[0].Message)
	}
	c := checker.New("main.ard", result.Program, nil)
	c.Check()
	if c.HasErrors() {
		t.Fatalf("checker diagnostics: %v", c.Diagnostics())
	}
	program, err := air.Lower(c.Module())
	if err != nil {
		t.Fatalf("lower AIR: %v", err)
	}
	data, err := air.SerializeProgram(program)
	if err != nil {
		t.Fatalf("serialize AIR: %v", err)
	}
	decoded, err := air.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("deserialize AIR: %v", err)
	}
	if err := air.Validate(decoded); err != nil {
		t.Fatalf("validate decoded AIR: %v", err)
	}
	if decoded.Entry == air.NoFunction {
		t.Fatal("decoded entry = NoFunction")
	}
	if len(decoded.Functions) != len(program.Functions) {
		t.Fatalf("decoded functions = %d, want %d", len(decoded.Functions), len(program.Functions))
	}
}
