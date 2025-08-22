package runtime

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"

	"github.com/akonwi/ard/checker"
)

type Object struct {
	// the raw value will always be the Go representation of the data.
	// Object should only be nested in raw for collections like List, Map.
	raw   any
	_type checker.Type

	// Results will set one of these to true
	isErr bool
	isOk  bool
}

func (o Object) String() string {
	return fmt.Sprintf("%v:%s", o.raw, o._type)
}

func (o Object) Type() checker.Type {
	return o._type
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

// slightly different from .Set()
func (o *Object) Reassign(val *Object) {
	o.raw = val.raw

	// Update target type to match value type.
	// o._type could be a generic and if the checker allowed it, o should become the new type
	o._type = val._type
}

// deep copies an object
func (o *Object) Copy() *Object {
	copy := &Object{
		raw:   o.raw,
		_type: o._type,
		isErr: o.isErr,
		isOk:  o.isOk,
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
		// Deep copy Maybe - if value is nil (None), copy as-is, otherwise deep copy the value
		if o.Raw() == nil {
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
		// Enums are represented as int8
	case *checker.FunctionDef:
		// Functions cannot be copied - return the same function object
		// Functions are immutable so sharing them is safe
	default:
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
	if o._type == checker.Int {
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
	return o._type == checker.Float
}

func (o Object) AsFloat() float64 {
	if o._type == checker.Float {
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
	if o._type == checker.Int {
		return o.raw.(string), true
	}
	return "", false
}

func (o *Object) AsList() []*Object {
	if list, ok := o.raw.([]*Object); ok {
		return list
	}
	panic(fmt.Sprintf("%T is not a List", o._type))
}

func (o *Object) AsMap() map[string]*Object {
	if m, ok := o.raw.(map[string]*Object); ok {
		return m
	}
	panic(fmt.Sprintf("%T is not a Map", o._type))
}

func MakeStr(s string) *Object {
	return &Object{
		_type: checker.Str,
		raw:   s,
	}
}

func MakeInt(i int) *Object {
	return &Object{
		_type: checker.Int,
		raw:   i,
	}
}

func MakeFloat(f float64) *Object {
	return &Object{
		_type: checker.Float,
		raw:   f,
	}
}

func MakeBool(b bool) *Object {
	return &Object{
		_type: checker.Bool,
		raw:   b,
	}
}

// instantiate a $T?
func MakeMaybe(raw any, of checker.Type) *Object {
	return &Object{
		_type: checker.MakeMaybe(of),
		raw:   raw,
	}
}

func (o Object) ToMaybe() *Object {
	o._type = checker.MakeMaybe(o._type)
	return &o
}

func MakeList(of checker.Type, items ...*Object) *Object {
	return &Object{
		_type: checker.MakeList(of),
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
		raw:   make(map[string]*Object),
	}
}

// todo: just use o.String()
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
	default:
		panic(fmt.Errorf("Unsupported map key: %s", keyType))
	}

	return key
}

/* these Result factory functions lose the initial union and only know which part it is */

// create Result::Err
func MakeErr(err *Object) *Object {
	unwrapped := err.Unwrap()
	unwrapped.isErr = true
	return unwrapped
}

// create Result::Ok
func MakeOk(err *Object) *Object {
	unwrapped := err.Unwrap()
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

// return an object w/o Result indicators
// a no-op if not already a result
func (o *Object) Unwrap() *Object {
	return Make(o.raw, o._type)
}

func MakeStruct(of checker.Type, fields map[string]*Object) *Object {
	return &Object{
		raw:   fields,
		_type: of,
	}
}

func (o Object) IsStruct() bool {
	if _, ok := o._type.(*checker.StructDef); ok {
		return true
	}
	return false
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
	}
}

func Make(val any, of checker.Type) *Object {
	return &Object{
		raw:   val,
		_type: of,
	}
}

// use a single instance of void
var void = &Object{
	raw: nil, _type: checker.Void,
}

func Void() *Object {
	return void
}
