package air

import (
	"reflect"
	"testing"

	"github.com/akonwi/ard/checker"
)

func TestADR0057PreservesClosureCaptureModes(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		fn main() {
			let count = 1
			let first = Box{value: 1}
			let second = Box{value: 2}
			mut reference = mut first
			let capture_value = fn() Int { count }
			let capture_handle = fn() Int { reference.value }
			let capture_slot = fn() { reference = mut second }
		}
	`)

	seen := map[CaptureMode]bool{}
	for _, function := range program.Functions {
		for _, capture := range function.Captures {
			seen[capture.Mode] = true
		}
	}
	for _, mode := range []CaptureMode{CaptureValue, CaptureReference, CaptureSlot} {
		if !seen[mode] {
			t.Fatalf("capture mode %d missing from AIR: %#v", mode, program.Functions)
		}
	}
}

func TestADR0057BindsGenericReferentThroughReferenceType(t *testing.T) {
	module := checkedModuleWithPath(t, "test", `fn take(value: mut $T) {}`)
	definition, ok := module.Get("take").Type.(*checker.FunctionDef)
	if !ok || len(definition.Parameters) != 1 {
		t.Fatalf("take definition = %#v", module.Get("take").Type)
	}
	lowerer := &lowerer{program: Program{Types: []TypeInfo{
		{ID: 1, Kind: TypeInt, Name: "Int"},
		{ID: 2, Kind: TypeReference, Name: "mut Int", Elem: 1},
	}}}
	function := &functionLowerer{l: lowerer, typeVars: map[string]TypeID{}}
	function.bindTypeVars(definition.Parameters[0].Type, 2)
	if len(function.typeVars) != 1 {
		t.Fatalf("bindings = %#v", function.typeVars)
	}
	for name, bound := range function.typeVars {
		if bound != 1 {
			t.Fatalf("$%s bound to %d, want referent type 1", name, bound)
		}
	}
}

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
	if addressable.Kind != ExprMutRef || existing.Kind != ExprMutRef || fresh.Kind != ExprMutRef {
		t.Fatalf("reference creation kinds = %v/%v/%v, want ExprMutRef", addressable.Kind, existing.Kind, fresh.Kind)
	}
	if addressable.ReferenceMode != AddressablePlace || existing.ReferenceMode != ExistingReference || fresh.ReferenceMode != FreshValue {
		t.Fatalf("reference modes = %v/%v/%v", addressable.ReferenceMode, existing.ReferenceMode, fresh.ReferenceMode)
	}
	for name, expression := range map[string]*Expr{"addressable": addressable, "existing": existing, "fresh": fresh} {
		if typeKind(t, program, expression.Type) != TypeReference {
			t.Fatalf("%s type = %#v, want TypeReference", name, testTypeInfo(t, program, expression.Type))
		}
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
	if dereference.Kind != ExprDeref || dereference.Observational {
		t.Fatalf("explicit deref = %#v, want non-observational ExprDeref", dereference)
	}
	if dereference.Target == nil || typeKind(t, program, dereference.Target.Type) != TypeReference {
		t.Fatalf("deref operand = %#v, want reference", dereference.Target)
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
			mapping: [Str: mut Box],
			callback: fn(mut Box) mut Box,
			result: (mut Box)!Str,
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
	if maybe.Kind != TypeMaybe || maybe.ElemMutable || typeKind(t, program, maybe.Elem) != TypeReference {
		t.Fatalf("maybe type = %#v, want (mut Box)? represented recursively", maybe)
	}
	holder := testTypeInfo(t, program, keep.Signature.Params[2].Type)
	if holder.Kind != TypeStruct || len(holder.GenericArgs) != 1 || typeKind(t, program, holder.GenericArgs[0]) != TypeReference || len(holder.Fields) != 1 || typeKind(t, program, holder.Fields[0].Type) != TypeReference {
		t.Fatalf("holder type = %#v, want Holder<mut Box>", holder)
	}
	mapping := testTypeInfo(t, program, keep.Signature.Params[3].Type)
	if mapping.Kind != TypeMap || typeKind(t, program, mapping.Value) != TypeReference {
		t.Fatalf("map type = %#v, want [Str: mut Box]", mapping)
	}
	callback := testTypeInfo(t, program, keep.Signature.Params[4].Type)
	if callback.Kind != TypeFunction || typeKind(t, program, callback.Params[0]) != TypeReference || typeKind(t, program, callback.Return) != TypeReference {
		t.Fatalf("callback type = %#v, want fn(mut Box) mut Box", callback)
	}
	result := testTypeInfo(t, program, keep.Signature.Params[5].Type)
	if result.Kind != TypeResult || typeKind(t, program, result.Value) != TypeReference {
		t.Fatalf("result type = %#v, want (mut Box)!Str", result)
	}
}

func TestADR0057PreservesReferenceHandlesAcrossFieldsCallsAndReturns(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		struct Holder { current: mut Box }

		fn identity(value: mut Box) mut Box {
			value
		}

		fn keep(holder: Holder, value: mut Box) mut Box {
			let from_field = holder.current
			let from_call = identity(value)
			from_call
		}
	`)

	holder := findType(t, program, "Holder")
	if len(holder.Fields) != 1 || typeKind(t, program, holder.Fields[0].Type) != TypeReference {
		t.Fatalf("Holder fields = %#v, want reference-valued current", holder.Fields)
	}
	identity := findFunction(t, program, "identity")
	if typeKind(t, program, identity.Signature.Params[0].Type) != TypeReference || typeKind(t, program, identity.Signature.Return) != TypeReference {
		t.Fatalf("identity signature = %#v, want reference param and return", identity.Signature)
	}
	keep := findFunction(t, program, "keep")
	if len(keep.Body.Stmts) < 2 {
		t.Fatalf("keep statements = %#v", keep.Body.Stmts)
	}
	field := keep.Body.Stmts[0].Value
	call := keep.Body.Stmts[1].Value
	if field == nil || field.Kind != ExprGetField || typeKind(t, program, field.Type) != TypeReference {
		t.Fatalf("reference field load = %#v", field)
	}
	if call == nil || call.Kind != ExprCall || typeKind(t, program, call.Type) != TypeReference {
		t.Fatalf("reference call result = %#v", call)
	}
}

func TestADR0057LowersConcreteToTraitReferenceProjection(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		trait View {
			fn get() Int
		}
		impl View for Box {
			fn get() Int {
				self.value
			}
		}
		struct Holder { current: mut View }

		fn consume(value: mut View) mut View {
			value
		}

		fn read(value: mut View) Int {
			value.get()
		}

		fn project(value: mut Box) mut View {
			let holder = Holder{current: value}
			consume(value)
		}
	`)

	project := findFunction(t, program, "project")
	if len(project.Body.Stmts) < 1 || project.Body.Stmts[0].Value == nil {
		t.Fatalf("project body = %#v", project.Body)
	}
	fieldProjection := project.Body.Stmts[0].Value.Fields[0].Value
	if fieldProjection.Kind != ExprTraitRefProject {
		t.Fatalf("field projection = %#v, want ExprTraitRefProject", fieldProjection)
	}
	if project.Body.Result == nil || len(project.Body.Result.Args) != 1 {
		t.Fatalf("project result = %#v", project.Body.Result)
	}
	result := &project.Body.Result.Args[0]
	if result.Kind != ExprTraitRefProject || result.Target == nil {
		t.Fatalf("argument projection = %#v, want ExprTraitRefProject", result)
	}
	resultType := testTypeInfo(t, program, result.Type)
	if resultType.Kind != TypeReference || typeKind(t, program, resultType.Elem) != TypeTraitObject {
		t.Fatalf("projection type = %#v, want mut View", resultType)
	}
	targetType := testTypeInfo(t, program, result.Target.Type)
	if targetType.Kind != TypeReference || typeKind(t, program, targetType.Elem) != TypeStruct {
		t.Fatalf("projection target type = %#v, want mut Box", targetType)
	}
	if int(result.Impl) < 0 || int(result.Impl) >= len(program.Impls) {
		t.Fatalf("projection impl = %d, impls = %d", result.Impl, len(program.Impls))
	}
	read := findFunction(t, program, "read")
	if read.Body.Result == nil || read.Body.Result.Kind != ExprCallTrait || read.Body.Result.Target == nil || typeKind(t, program, read.Body.Result.Target.Type) != TypeReference {
		t.Fatalf("trait dispatch through reference = %#v", read.Body.Result)
	}
}

func TestADR0057PreservesTraitReferenceProjectionsInCompoundContexts(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		trait View {
			fn get() Int
		}
		impl View for Box {
			fn get() Int { self.value }
		}

		fn contexts(value: mut Box) {
			let values: [Str: mut View] = ["box": value]
			let keys: [mut View: Int] = [value: 1]
			let nested: [Str: [mut View]] = ["box": [value]]
			let maybe: (mut View)? = Maybe::new(value)
			let result: (mut View)!Str = Result::ok(value)
		}
	`)

	contexts := findFunction(t, program, "contexts")
	if len(contexts.Body.Stmts) < 5 {
		t.Fatalf("contexts statements = %#v", contexts.Body.Stmts)
	}
	for index := 0; index < 5; index++ {
		if contexts.Body.Stmts[index].Value == nil || !containsAIRExprKindDeep(contexts.Body.Stmts[index].Value, ExprTraitRefProject) {
			t.Fatalf("context %d lost trait reference projection: %#v", index, contexts.Body.Stmts[index].Value)
		}
	}
}

func TestADR0057MaybeMatchReferenceLocalMetadataAgreesWithType(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		fn read(value: (mut Box)?) Int {
			match value {
				found => found.value,
				_ => 0,
			}
		}
	`)

	read := findFunction(t, program, "read")
	if read.Body.Result == nil || read.Body.Result.Kind != ExprMatchMaybe {
		t.Fatalf("read body = %#v", read.Body)
	}
	local := read.Locals[read.Body.Result.SomeLocal]
	if typeKind(t, program, local.Type) != TypeReference || !local.Reference {
		t.Fatalf("some local = %#v, want reference type and compatibility metadata", local)
	}
}

func TestADR0057MutatingMethodReceiverIsReferenceTyped(t *testing.T) {
	program := lowerSource(t, `
		struct Box { value: Int }
		struct Holder { inner: Box }
		impl Box {
			fn get() Int {
				self.value
			}
			fn mut set(value: Int) {
				self.value = value
			}
		}
		fn update(holder: mut Holder) {
			holder.inner.set(2)
		}
		fn observe(value: mut Box) Int {
			value.get()
		}
	`)

	set := findFunction(t, program, "Box.set")
	if len(set.Signature.Params) == 0 {
		t.Fatalf("set signature = %#v", set.Signature)
	}
	receiver := testTypeInfo(t, program, set.Signature.Params[0].Type)
	if receiver.Kind != TypeReference || typeKind(t, program, receiver.Elem) != TypeStruct {
		t.Fatalf("mutating receiver = %#v, want mut Box", receiver)
	}
	update := findFunction(t, program, "update")
	if update.Body.Result == nil || len(update.Body.Result.Args) == 0 {
		t.Fatalf("update body = %#v", update.Body)
	}
	interiorReceiver := update.Body.Result.Args[0]
	if interiorReceiver.Kind != ExprMutRef || interiorReceiver.ReferenceMode != AddressablePlace || interiorReceiver.Target == nil || interiorReceiver.Target.Kind != ExprGetField {
		t.Fatalf("interior receiver = %#v, want AddressablePlace field reference", interiorReceiver)
	}
	observe := findFunction(t, program, "observe")
	if set.Signature.Params[0].Type != observe.Signature.Params[0].Type || set.Signature.Params[0].Type != interiorReceiver.Type {
		t.Fatalf("reference type identity split: receiver=%d param=%d interior=%d", set.Signature.Params[0].Type, observe.Signature.Params[0].Type, interiorReceiver.Type)
	}
	if observe.Body.Result == nil || len(observe.Body.Result.Args) == 0 {
		t.Fatalf("observe body = %#v", observe.Body)
	}
	observedReceiver := observe.Body.Result.Args[0]
	if observedReceiver.Kind != ExprDeref || !observedReceiver.Observational || observedReceiver.Target == nil || typeKind(t, program, observedReceiver.Target.Type) != TypeReference {
		t.Fatalf("observed receiver = %#v, want observational dereference", observedReceiver)
	}
}

func TestADR0057DistinguishesObservationalAndExplicitDereference(t *testing.T) {
	program := lowerSource(t, `
		fn observe(reference: mut Int) Int {
			let implicit = reference + 1
			let explicit = deref reference
			implicit + explicit
		}
	`)

	observe := findFunction(t, program, "observe")
	if len(observe.Body.Stmts) < 2 {
		t.Fatalf("observe statements = %#v", observe.Body.Stmts)
	}
	implicit := observe.Body.Stmts[0].Value
	explicit := observe.Body.Stmts[1].Value
	if implicit == nil || implicit.Left == nil || implicit.Left.Kind != ExprDeref || !implicit.Left.Observational {
		t.Fatalf("implicit observation = %#v", implicit)
	}
	if explicit == nil || explicit.Kind != ExprDeref || explicit.Observational {
		t.Fatalf("explicit dereference = %#v", explicit)
	}
}

func containsAIRExprKindDeep(expr *Expr, kind ExprKind) bool {
	if expr == nil {
		return false
	}
	if expr.Kind == kind {
		return true
	}
	if containsAIRExprKindDeep(expr.Target, kind) || containsAIRExprKindDeep(expr.Left, kind) || containsAIRExprKindDeep(expr.Right, kind) || containsAIRExprKindDeep(expr.Condition, kind) {
		return true
	}
	for i := range expr.Args {
		if containsAIRExprKindDeep(&expr.Args[i], kind) {
			return true
		}
	}
	for i := range expr.Entries {
		if containsAIRExprKindDeep(&expr.Entries[i].Key, kind) || containsAIRExprKindDeep(&expr.Entries[i].Value, kind) {
			return true
		}
	}
	for i := range expr.Fields {
		if containsAIRExprKindDeep(&expr.Fields[i].Value, kind) {
			return true
		}
	}
	return false
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
