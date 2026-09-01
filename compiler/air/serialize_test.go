package air_test

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/air"
	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/parse"
)

func TestSerializeProgramPreservesGoFieldTags(t *testing.T) {
	result := parse.Parse([]byte(`
		struct Config {
			#go:yaml("global_context,omitempty")
			value: Str,
		}
	`), "main.ard")
	if len(result.Errors) > 0 {
		t.Fatalf("parse errors: %v", result.Errors)
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
	for _, typ := range decoded.Types {
		if typ.Kind == air.TypeStruct && typ.Name == "Config" {
			tags := typ.Fields[0].GoTags
			if len(tags) != 1 || tags[0].Key != "yaml" || tags[0].Value != "global_context,omitempty" {
				t.Fatalf("decoded Go field tags = %#v", tags)
			}
			return
		}
	}
	t.Fatal("decoded Config type missing")
}

func TestSerializeProgramPreservesEveryExprPayloadType(t *testing.T) {
	payloads := []air.ExprPayload{
		&air.TextExprPayload{},
		&air.BoolExprPayload{},
		&air.EnumExprPayload{},
		&air.LocalExprPayload{},
		&air.GlobalExprPayload{},
		&air.CallExprPayload{Spread: &air.SpreadExprPayload{}},
		&air.ForeignExprPayload{Spread: &air.SpreadExprPayload{}},
		&air.InterfaceExprPayload{},
		&air.ReferenceExprPayload{},
		&air.FieldExprPayload{},
		&air.TagExprPayload{},
		&air.TraitExprPayload{},
		&air.AggregateExprPayload{},
		&air.BinaryExprPayload{},
		&air.BlockExprPayload{},
		&air.IfExprPayload{},
		&air.EnumMatchExprPayload{},
		&air.IntMatchExprPayload{},
		&air.StrMatchExprPayload{},
		&air.UnionMatchExprPayload{},
		&air.ForeignMatchExprPayload{},
		&air.MaybeMatchExprPayload{},
		&air.ResultMatchExprPayload{},
		&air.TryExprPayload{},
		&air.SelectExprPayload{},
		&air.MaybeCallExprPayload{},
		&air.UnsafeCastExprPayload{},
	}
	program := &air.Program{Functions: []air.Function{{Body: air.Block{Stmts: make([]air.Stmt, len(payloads))}}}}
	for i, payload := range payloads {
		program.Functions[0].Body.Stmts[i] = air.Stmt{Kind: air.StmtExpr, Expr: &air.Expr{Payload: payload}}
	}

	data, err := air.SerializeProgram(program)
	if err != nil {
		t.Fatalf("serialize AIR payloads: %v", err)
	}
	decoded, err := air.DeserializeProgram(data)
	if err != nil {
		t.Fatalf("deserialize AIR payloads: %v", err)
	}
	for i, want := range payloads {
		got := decoded.Functions[0].Body.Stmts[i].Expr.Payload
		if reflect.TypeOf(got) != reflect.TypeOf(want) {
			t.Fatalf("payload %d type = %T, want %T", i, got, want)
		}
	}
}

func TestSerializeProgram(t *testing.T) {
	result := parse.Parse([]byte(`
		fn main() Int {
			let values = [20, 22]
			values.at(0).expect("bounds") + values.at(1).expect("bounds")
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
