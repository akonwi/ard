package air

import (
	"strings"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestValidateRejectsMalformedDirectGoFieldSet(t *testing.T) {
	program := validDirectGoFieldSetProgram()
	program.Functions[0].Body.Stmts[0].FieldName = ""

	err := Validate(program)
	if err == nil || !strings.Contains(err.Error(), "direct Go field set statement missing field name") {
		t.Fatalf("Validate error = %v, want missing direct Go field name", err)
	}
}

func TestValidateAcceptsDirectGoFieldSet(t *testing.T) {
	if err := Validate(validDirectGoFieldSetProgram()); err != nil {
		t.Fatalf("Validate error = %v", err)
	}
}

func validDirectGoFieldSetProgram() *Program {
	return &Program{
		Modules: []Module{{ID: 0, Path: "test"}},
		Types: []TypeInfo{
			{ID: 1, Kind: TypeVoid, Name: "Void"},
			{ID: 2, Kind: TypeExtern, Name: "Response", ExternBinding: "go:example.com/http::Response"},
			{ID: 3, Kind: TypeInt, Name: "Int"},
		},
		Functions: []Function{{
			ID:     0,
			Module: 0,
			Name:   "set_status",
			Signature: Signature{
				Return: 1,
			},
			Locals: []Local{{ID: 0, Name: "res", Type: 2, Mutable: true}},
			Body: Block{Stmts: []Stmt{{
				Kind:              StmtSetDirectGoField,
				Target:            &Expr{Kind: ExprLoadLocal, Type: 2, Local: 0},
				Value:             &Expr{Kind: ExprConstInt, Type: 3, Int: 201},
				FieldName:         "StatusCode",
				DirectGoFieldType: checker.GoValueType{Kind: checker.GoValueInt, Expr: "int"},
			}}},
		}},
		Entry:  NoFunction,
		Script: NoFunction,
	}
}
