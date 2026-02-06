package runtime

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"

	"github.com/akonwi/ard/checker"
)

type Object struct {
	// the raw value will always be the Go representation of the data.
	// Object should only be nested in raw for collections like List, Map.
	raw   any
	_type checker.Type
	kind  Kind
	name  string

	// Results will set one of these to true
	isErr bool
	isOk  bool

	isNone bool
}

func (o Object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o Object) Type() checker.Type {
	return o._type
}

func (o Object) Kind() Kind {
	return o.kind
}

func (o Object) TypeName() string {
	return o.name
}

// simply compares the raw representations.
//
// the checker rules should prevent more complex comparisons.
func (o Object) Equals(other Object) bool {
	switch o._type {
	case checker.Int:
		return o.raw.(int) == other.raw.(int)
	case checker.Float:
		return o.raw.(float64) == other.raw.(float64)
	case checker.Bool:
		return o.raw.(bool) == other.raw.(bool)
	case checker.Str:
		return o.raw.(string) == other.raw.(string)
	default:
		return o.raw == other.raw
	}
}

func (o Object) Raw() any {
	return o.raw
}

// mutate the inner representation
func (o *Object) Set(v any) {
	o.raw = v
}

// todo: eliminating unknown generics in the checker needs more work, particularly for nested scopes - see decode::nullable
//   - the checker successfully refines on variable definitions
//   - it does not work on chained expressions
//     let result = returns_generic()
//     result.expect("foobar").do_stuff() // .expect(...) returns an open generic
func (o *Object) SetRefinedType(declared checker.Type) {
	if result, ok := o._type.(*checker.Result); ok {
		if o.isErr {
			o._type = result.Err()
		}
		if o.isOk {
			o._type = result.Val()
		}
	}
	if checker.IsMaybe(o._type) {
		o._type = declared
	}
	if _, ok := o._type.(*checker.TypeVar); ok {
		o._type = declared
	}
	if strings.Contains(o.Type().String(), "$") && !strings.Contains(declared.String(), "$") {
		o._type = declared

		// for collections, refine insides
		switch declared := declared.(type) {
		case *checker.List:
			raw := o.raw.([]*Object)
			for i := range raw {
				raw[i].SetRefinedType(declared.Of())
			}
		case *checker.Map:
			raw := o.raw.(map[string]*Object)
			for _, v := range raw {
				v.SetRefinedType(declared.Value())
			}
		}
	}
	o.kind = kindForType(o._type)
	o.name = typeNameForType(o._type)
}

func deepCopy(data any) any {
	if data == nil {
		return nil
	}

	switch v := data.(type) {
	case map[string]any:
		newMap := make(map[string]any, len(v))
		for key, val := range v {
			newMap[key] = deepCopy(val)
		}
		return newMap
	case []any:
		newSlice := make([]any, len(v))
		for i, val := range v {
			newSlice[i] = deepCopy(val)
		}
		return newSlice
	default:
		// Primitive types (int, string, bool, float64, etc.) are copied by value.
		return v
	}
}

// deep copies an object
func (o *Object) Copy() *Object {
	copy := &Object{
		raw:    o.raw,
		_type:  o._type,
		kind:   o.kind,
		name:   o.name,
		isErr:  o.isErr,
		isOk:   o.isOk,
		isNone: o.isNone,
	}

	switch o.Type().(type) {
	case *checker.StructDef:
		// Deep copy struct
		originalMap := o.raw.(map[string]*Object)
		rawCopy := make(map[string]*Object)
		for key, value := range originalMap {
			rawCopy[key] = value.Copy()
		}
		copy.raw = rawCopy
	case *checker.List:
		// Deep copy list
		originalSlice := o.AsList()
		copiedSlice := make([]*Object, len(originalSlice))
		for i, value := range originalSlice {
			copiedSlice[i] = value.Copy()
		}
		copy.raw = copiedSlice
	case *checker.Map:
		// Deep copy map
		originalMap := o.AsMap()
		copiedMap := make(map[string]*Object)
		for key, value := range originalMap {
			copiedMap[key] = value.Copy()
		}
		copy.raw = copiedMap
	case *checker.Maybe:
		// Deep copy Maybe - if value is not None, deep copy the inner value.
		if !o.IsNone() {
			if inner, ok := o.Raw().(*Object); ok {
				copy.raw = inner.Copy()
			}
		}
	case *checker.Result:
		// Deep copy Result - the value is an object containing either the success or error value
		if inner, ok := o.Raw().(*Object); ok {
			copy.raw = inner.Copy()
		}
	case *checker.Enum:
		// Enums are represented as int (their discriminant value)
	case *checker.FunctionDef:
		// Functions cannot be copied - return the same function object
		// Functions are immutable so sharing them is safe
	default:
		if o._type == checker.Dynamic {
			// Deep copy the raw value
			copy.raw = deepCopy(o.raw)
		}
		// For primitives (Str, Int, Float, Bool), return a new object with same value
		// These are immutable in Ard, so we can just create a new object
	}

	return copy
}

// MarshalJSONTo implements JSON v2 marshaling interface
func (o *Object) MarshalJSONTo(enc *jsontext.Encoder) error {
	return json.MarshalEncode(enc, o.GoValue(),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

// MarshalJSON implements the traditional JSON marshaling interface
func (o *Object) MarshalJSON() ([]byte, error) {
	return json.Marshal(o.GoValue(),
		json.FormatNilSliceAsNull(true),
		json.FormatNilMapAsNull(true),
	)
}

// Get the Go value. useful for encoding out and scripting results
func (o *Object) GoValue() any {
	if o._type == nil {
		return o.raw
	}

	switch o._type.(type) {
	case *checker.FunctionDef:
		return o._type.String()
	case *checker.List:
		raw := o.raw.([]*Object)
		_array := make([]any, len(raw))
		for i, item := range raw {
			_array[i] = item.GoValue()
		}
		return _array
	case *checker.StructDef:
		m := o.raw.(map[string]*Object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.GoValue()
		}
		return rawMap
	case *checker.Map:
		m := o.raw.(map[string]*Object)
		rawMap := make(map[string]any)
		for key, value := range m {
			rawMap[key] = value.GoValue()
		}
		return rawMap
	}

	return o.raw
}

func (o Object) AsBool() bool {
	if o._type == checker.Bool {
		return o.raw.(bool)
	}
	panic(fmt.Sprintf("%T is not a Bool", o._type))
}

func (o Object) IsInt() (int, bool) {
	if o.kind == KindInt {
		return o.raw.(int), true
	}
	return 0, false
}

func (o Object) AsInt() int {
	if int, ok := o.raw.(int); ok {
		return int
	}
	panic(fmt.Sprintf("%s is not an Int", o))
}

func (o Object) IsFloat() bool {
	return o.kind == KindFloat
}

func (o Object) AsFloat() float64 {
	if o.kind == KindFloat {
		return o.raw.(float64)
	}
	panic(fmt.Sprintf("%T is not a Float", o._type))
}

func (o Object) AsString() string {
	if str, ok := o.raw.(string); ok {
		return str
	}
	panic(fmt.Sprintf("%s is not a string", o))
}

func (o Object) IsStr() (string, bool) {
	if o.kind == KindStr {
		return o.raw.(string), true
	}
	return "", false
}

func (o *Object) AsList() []*Object {
	if list, ok := o.raw.([]*Object); ok {
		return list
	}
	panic(fmt.Sprintf("%s could not be cast to a List", o))
}

func (o *Object) AsMap() map[string]*Object {
	if m, ok := o.raw.(map[string]*Object); ok {
		return m
	}
	panic(fmt.Sprintf("%T is not a Map", o._type))
}

func (o Object) MapType() *checker.Map {
	if o.kind != KindMap {
		return nil
	}
	if m, ok := o._type.(*checker.Map); ok {
		return m
	}
	return nil
}

func (o Object) StructType() *checker.StructDef {
	if o.kind != KindStruct {
		return nil
	}
	if s, ok := o._type.(*checker.StructDef); ok {
		return s
	}
	return nil
}

func (o Object) EnumType() *checker.Enum {
	if o.kind != KindEnum {
		return nil
	}
	if e, ok := o._type.(*checker.Enum); ok {
		return e
	}
	return nil
}

func MakeStr(s string) *Object {
	return &Object{
		_type: checker.Str,
		kind:  KindStr,
		name:  checker.Str.String(),
		raw:   s,
	}
}

func MakeInt(i int) *Object {
	return &Object{
		_type: checker.Int,
		kind:  KindInt,
		name:  checker.Int.String(),
		raw:   i,
	}
}

func MakeFloat(f float64) *Object {
	return &Object{
		_type: checker.Float,
		kind:  KindFloat,
		name:  checker.Float.String(),
		raw:   f,
	}
}

func MakeBool(b bool) *Object {
	return &Object{
		_type: checker.Bool,
		kind:  KindBool,
		name:  checker.Bool.String(),
		raw:   b,
	}
}

func MakeNone(of checker.Type) *Object {
	return &Object{
		_type:  checker.MakeMaybe(of),
		kind:   KindMaybe,
		name:   checker.MakeMaybe(of).String(),
		raw:    nil,
		isNone: true,
	}
}

func (o Object) ToNone() *Object {
	if !checker.IsMaybe(o._type) {
		panic(fmt.Errorf("Cannot make Maybe::none from %s", o))
	}
	o.isNone = true
	return &o
}

// create a Maybe::Some from an existing Maybe
func (o Object) ToSome(val any) *Object {
	if !checker.IsMaybe(o._type) {
		panic(fmt.Errorf("Cannot make Maybe::some from %s", o))
	}
	o.raw = val
	o.isNone = false
	return &o
}

func (o Object) IsNone() bool {
	return o.isNone
}

func MakeList(of checker.Type, items ...*Object) *Object {
	return &Object{
		_type: checker.MakeList(of),
		kind:  KindList,
		name:  checker.MakeList(of).String(),
		raw:   items,
	}
}

func (o *Object) List_Push(item *Object) {
	list := o.AsList()
	o.raw = append(list, item)
}

func MakeMap(keyType, valueType checker.Type) *Object {
	return &Object{
		_type: checker.MakeMap(keyType, valueType),
		kind:  KindMap,
		name:  checker.MakeMap(keyType, valueType).String(),
		raw:   make(map[string]*Object),
	}
}

func ToMapKey(o *Object) string {
	// Create a string representation for the key
	var keyStr string
	switch v := o.raw.(type) {
	case string:
		keyStr = v
	case int:
		keyStr = strconv.Itoa(v)
	case bool:
		keyStr = strconv.FormatBool(v)
	case float64:
		keyStr = strconv.FormatFloat(v, 'g', -1, 64)
	default:
		// For complex types bail and use debug string
		keyStr = o.String()
	}

	return keyStr
}

// map_set sets a key-value pair in the map object. returns true if successful, false otherwise.
func (o *Object) Map_Set(key, val *Object) bool {
	if _, isMap := o._type.(*checker.Map); !isMap {
		return false
	}

	raw := o.raw.(map[string]*Object)
	raw[ToMapKey(key)] = val
	return true
}

// Ard primitives can be used as keys. The raw representation is a string, so convert the string from Go back to Ard
func (o Object) Map_GetKey(str string) *Object {
	keyType := o._type.(*checker.Map).Key()
	key := Make(nil, keyType)

	switch keyType.String() {
	case checker.Str.String():
		key.raw = str
	case checker.Int.String():
		if _num, err := strconv.Atoi(str); err != nil {
			panic(fmt.Errorf("Couldn't turn map key %s into int", str))
		} else {
			key.raw = _num
		}
	case checker.Bool.String():
		if _bool, err := strconv.ParseBool(str); err != nil {
			panic(fmt.Errorf("Couldn't turn map key %s into bool", str))
		} else {
			key.raw = _bool
		}
	case checker.Float.String():
		if _float, err := strconv.ParseFloat(str, 64); err != nil {
			panic(fmt.Errorf("Couldn't turn map key %s into float", str))
		} else {
			key.raw = _float
		}
	case checker.Dynamic.String():
		key.raw = str
	default:
		panic(fmt.Errorf("Unsupported map key: %s", keyType))
	}

	return key
}

/* these Result factory functions lose the initial union and only know which part it is */

// create Result::Err
func MakeErr(err *Object) *Object {
	unwrapped := err.UnwrapResult()
	unwrapped.isErr = true
	return unwrapped
}

// create Result::Ok
func MakeOk(err *Object) *Object {
	unwrapped := err.UnwrapResult()
	unwrapped.isOk = true
	return unwrapped
}

func (o Object) IsResult() bool {
	return o.isOk || o.isErr
}

func (o Object) IsOk() bool {
	return o.isOk
}

func (o Object) IsErr() bool {
	return o.isErr
}

func (o *Object) UnwrapResult() *Object {
	new := Make(o.raw, o._type)
	new.isNone = o.isNone
	return new
}

func MakeStruct(of checker.Type, fields map[string]*Object) *Object {
	return &Object{
		raw:   fields,
		_type: of,
		kind:  KindStruct,
		name:  of.String(),
	}
}

func (o Object) IsStruct() bool {
	return o.kind == KindStruct
}

func (o Object) Struct_Get(key string) *Object {
	if !o.IsStruct() {
		panic(fmt.Errorf("%s is not a struct", o._type))
	}

	if fields, ok := o.raw.(map[string]*Object); ok {
		if field, exists := fields[key]; exists {
			return field
		}
	}
	return nil
}

func MakeDynamic(val any) *Object {
	return &Object{
		raw:   val,
		_type: checker.Dynamic,
		kind:  KindDynamic,
		name:  checker.Dynamic.String(),
	}
}

func Make(val any, of checker.Type) *Object {
	return &Object{
		raw:   val,
		_type: of,
		kind:  kindForType(of),
		name:  typeNameForType(of),
	}
}

// use a single instance of void. lame attempt at optimization
var void = &Object{
	raw: nil, _type: checker.Void, kind: KindVoid, name: checker.Void.String(),
}

func Void() *Object {
	return void
}

type Closure interface {
	Eval(args ...*Object) *Object
	EvalIsolated(args ...*Object) *Object
	GetParams() []checker.Parameter
}
