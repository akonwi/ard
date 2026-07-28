package air

import (
	"reflect"
	"testing"
)

func TestADR0057LowersExplicitReferenceCreationModes(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }

		fn main() {
			let value = Box{value: 1}
			let addressable = mut value
			let existing = mut addressable
			let fresh = mut Box{value: 2}
		}
	`)

	mainFn := findFunction(t, program, "main")
	if len(mainFn.Body.Stmts) < 4 {
		t.Fatalf("main statements = %#v", mainFn.Body.Stmts)
	}
	addressable := mainFn.Body.Stmts[1].Value
	existing := mainFn.Body.Stmts[2].Value
	fresh := mainFn.Body.Stmts[3].Value
	for name, expression := range map[string]*Expr{
		"AddressablePlace":  addressable,
		"ExistingReference": existing,
		"FreshValue":        fresh,
	} {
		if expression == nil {
			t.Fatalf("%s expression = nil", name)
		}
	}
	if addressable.Kind != existing.Kind || addressable.Kind != fresh.Kind {
		t.Fatalf("reference creation modes use different AIR operations: addressable=%v existing=%v fresh=%v", addressable.Kind, existing.Kind, fresh.Kind)
	}
	assertDistinctAIRReferenceMetadata(t, addressable, existing, "AddressablePlace", "ExistingReference")
	assertDistinctAIRReferenceMetadata(t, addressable, fresh, "AddressablePlace", "FreshValue")
	assertDistinctAIRReferenceMetadata(t, existing, fresh, "ExistingReference", "FreshValue")
}

func TestADR0057LowersDedicatedDereferenceExpression(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }

		fn main() Box {
			let value = Box{value: 1}
			let reference = mut value
			let copy = deref reference
			copy
		}
	`)

	mainFn := findFunction(t, program, "main")
	if len(mainFn.Body.Stmts) < 3 || mainFn.Body.Stmts[2].Value == nil {
		t.Fatalf("main statements = %#v", mainFn.Body.Stmts)
	}
	dereference := mainFn.Body.Stmts[2].Value
	if dereference.Kind == ExprMutRef || dereference.Kind == ExprLoadLocal {
		t.Fatalf("deref has no dedicated AIR operation: %#v", dereference)
	}
	if !hasDirectAIROperand(reflect.ValueOf(dereference).Elem()) {
		t.Fatalf("deref has no direct AIR operand: %#v", dereference)
	}
	if got := testTypeInfo(t, program, dereference.Type).Kind; got != TypeStruct {
		t.Fatalf("deref type kind = %v, want TypeStruct", got)
	}
}

func TestADR0057LowersReferenceShapedCompoundTypes(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		struct Holder<$T> { value: $T }

		fn keep(
			values: [mut Box],
			maybe: (mut Box)?,
			holder: Holder<mut Box>,
		) Holder<mut Box> {
			holder
		}
	`)

	keep := findFunction(t, program, "keep")
	list := testTypeInfo(t, program, keep.Signature.Params[0].Type)
	if list.Kind != TypeList || typeKind(t, program, list.Elem) != TypeReference {
		t.Fatalf("list type = %#v, want [mut Box]", list)
	}
	maybe := testTypeInfo(t, program, keep.Signature.Params[1].Type)
	if maybe.Kind != TypeMaybe || (!maybe.ElemMutable && typeKind(t, program, maybe.Elem) != TypeReference) {
		t.Fatalf("maybe type = %#v, want (mut Box)?", maybe)
	}
	holder := testTypeInfo(t, program, keep.Signature.Params[2].Type)
	if holder.Kind != TypeStruct || len(holder.GenericArgs) != 1 || typeKind(t, program, holder.GenericArgs[0]) != TypeReference {
		t.Fatalf("holder type = %#v, want Holder<mut Box>", holder)
	}
}

func assertDistinctAIRReferenceMetadata(t *testing.T, left, right *Expr, leftName, rightName string) {
	t.Helper()
	leftMetadata := *left
	rightMetadata := *right
	leftMetadata.Type = NoType
	rightMetadata.Type = NoType
	leftMetadata.Target = nil
	rightMetadata.Target = nil
	if reflect.DeepEqual(leftMetadata, rightMetadata) {
		t.Fatalf("%s and %s have indistinguishable AIR reference metadata", leftName, rightName)
	}
}

func hasDirectAIROperand(value reflect.Value) bool {
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		if !field.IsValid() {
			continue
		}
		if field.Type() == reflect.TypeOf((*Expr)(nil)) && !field.IsNil() {
			return true
		}
		if field.Type() == reflect.TypeOf(Expr{}) && !field.IsZero() {
			return true
		}
		if field.Type() == reflect.TypeOf([]Expr{}) && field.Len() > 0 {
			return true
		}
	}
	return false
}
