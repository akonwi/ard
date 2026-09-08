package air

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/akonwi/ard/checker"
)

// structuralTypeKey is the semantic identity of an anonymous AIR type.
// Display names and origin (checker-derived versus lowering-synthesized) are
// deliberately excluded. Child TypeIDs are already canonical when a key is
// constructed.
type structuralTypeKey struct {
	Kind     TypeKind
	Elem     TypeID
	Length   int
	Key      TypeID
	Value    TypeID
	Error    TypeID
	Params   string
	Return   TypeID
	Variadic bool
}

type nominalTypeKey struct {
	Kind       TypeKind
	ModulePath string
	Name       string
	Definition TypeID
	Target     string
	Namespace  string
	Symbol     string
	Pointer    bool
	Args       string
}

type nominalEntryState uint8

const (
	nominalBuilding nominalEntryState = iota
	nominalComplete
	nominalFailed
)

type nominalEntry struct {
	id    TypeID
	state nominalEntryState
	err   error
}

type typeParamKey struct {
	owner string
	index int
}

type typeInterner struct {
	program     *Program
	structural  map[structuralTypeKey]TypeID
	nominal     map[nominalTypeKey]*nominalEntry
	nominalByID map[TypeID]*nominalEntry
	typeParams  map[typeParamKey]TypeID
}

func newTypeInterner(program *Program) *typeInterner {
	return &typeInterner{
		program:     program,
		structural:  map[structuralTypeKey]TypeID{},
		nominal:     map[nominalTypeKey]*nominalEntry{},
		nominalByID: map[TypeID]*nominalEntry{},
		typeParams:  map[typeParamKey]TypeID{},
	}
}

func (i *typeInterner) reserveNominal(key nominalTypeKey, seed TypeInfo) (TypeID, bool, error) {
	if entry, ok := i.nominal[key]; ok {
		switch entry.state {
		case nominalFailed:
			return NoType, false, entry.err
		case nominalBuilding, nominalComplete:
			return entry.id, false, nil
		}
	}
	id := TypeID(len(i.program.Types) + 1)
	seed.ID = id
	i.program.Types = append(i.program.Types, seed)
	entry := &nominalEntry{id: id, state: nominalBuilding}
	i.nominal[key] = entry
	i.nominalByID[id] = entry
	return id, true, nil
}

func (i *typeInterner) completeNominal(key nominalTypeKey, info TypeInfo) (TypeID, error) {
	entry, ok := i.nominal[key]
	if !ok {
		return NoType, fmt.Errorf("nominal AIR type was not reserved: %+v", key)
	}
	if entry.state == nominalFailed {
		return NoType, entry.err
	}
	info.ID = entry.id
	if entry.state == nominalComplete {
		existing := i.program.Types[entry.id-1]
		if !reflect.DeepEqual(existing, info) {
			return NoType, fmt.Errorf("conflicting AIR metadata for nominal type %s", info.Name)
		}
		return entry.id, nil
	}
	i.program.Types[entry.id-1] = info
	entry.state = nominalComplete
	return entry.id, nil
}

func (i *typeInterner) failNominal(key nominalTypeKey, err error) error {
	entry, ok := i.nominal[key]
	if !ok {
		return err
	}
	entry.state = nominalFailed
	entry.err = err
	return err
}

func (i *typeInterner) nominalAvailable(id TypeID) bool {
	entry, nominal := i.nominalByID[id]
	return !nominal || entry.state == nominalComplete
}

func (i *typeInterner) validateComplete() error {
	issues := map[string]error{}
	keys := make([]string, 0)
	for key, entry := range i.nominal {
		var err error
		switch entry.state {
		case nominalBuilding:
			err = fmt.Errorf("nominal AIR type remained incomplete: %+v", key)
		case nominalFailed:
			err = entry.err
		}
		if err != nil {
			description := fmt.Sprintf("%+v", key)
			issues[description] = err
			keys = append(keys, description)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return issues[keys[0]]
}

func structuralIdentity(info TypeInfo) (structuralTypeKey, bool) {
	key := structuralTypeKey{Kind: info.Kind}
	switch info.Kind {
	case TypeList, TypeSlice, TypeMaybe, TypeChannel, TypeReceiver, TypeSender, TypeReference:
		key.Elem = info.Elem
	case TypeFixedArray:
		key.Elem = info.Elem
		key.Length = info.Length
	case TypeMap:
		key.Key = info.Key
		key.Value = info.Value
	case TypeResult:
		key.Value = info.Value
		key.Error = info.Error
	case TypeFunction:
		key.Params = typeIDsKey(info.Params)
		key.Return = info.Return
		key.Variadic = info.Variadic
	default:
		return structuralTypeKey{}, false
	}
	return key, true
}

func (i *typeInterner) internStructural(info TypeInfo) (TypeID, error) {
	key, ok := structuralIdentity(info)
	if !ok {
		return NoType, fmt.Errorf("AIR type kind %d is not structural", info.Kind)
	}
	if id, exists := i.structural[key]; exists {
		return id, nil
	}
	name, err := canonicalStructuralTypeName(i.program, info)
	if err != nil {
		return NoType, err
	}
	id := TypeID(len(i.program.Types) + 1)
	info.ID = id
	info.Name = name
	i.structural[key] = id
	i.program.Types = append(i.program.Types, info)
	return id, nil
}

func canonicalStructuralTypeName(program *Program, info TypeInfo) (string, error) {
	typeName := func(id TypeID) (string, error) {
		if !validTypeID(program, id) {
			return "", fmt.Errorf("structural AIR type references invalid child type %d", id)
		}
		return program.Types[id-1].Name, nil
	}
	elemName := func() (string, error) { return typeName(info.Elem) }

	switch info.Kind {
	case TypeList:
		elem, err := elemName()
		return "[" + elem + "]", err
	case TypeSlice:
		elem, err := elemName()
		return "Slice<" + elem + ">", err
	case TypeFixedArray:
		elem, err := elemName()
		return fmt.Sprintf("[%s; %d]", elem, info.Length), err
	case TypeChannel:
		elem, err := elemName()
		return "Chan<" + elem + ">", err
	case TypeReceiver:
		elem, err := elemName()
		return "Receiver<" + elem + ">", err
	case TypeSender:
		elem, err := elemName()
		return "Sender<" + elem + ">", err
	case TypeReference:
		elem, err := elemName()
		return "mut " + elem, err
	case TypeMaybe:
		elem, err := elemName()
		return elem + "?", err
	case TypeMap:
		key, err := typeName(info.Key)
		if err != nil {
			return "", err
		}
		value, err := typeName(info.Value)
		return "[" + key + ":" + value + "]", err
	case TypeResult:
		value, err := typeName(info.Value)
		if err != nil {
			return "", err
		}
		errType, err := typeName(info.Error)
		return value + "!" + errType, err
	case TypeFunction:
		params := make([]string, len(info.Params))
		for index, param := range info.Params {
			name, err := typeName(param)
			if err != nil {
				return "", err
			}
			if info.Variadic && index == len(info.Params)-1 {
				name = "..." + name
			}
			params[index] = name
		}
		result, err := typeName(info.Return)
		return "fn(" + strings.Join(params, ",") + ") " + result, err
	default:
		return "", fmt.Errorf("AIR type kind %d is not structural", info.Kind)
	}
}

// internCheckerStructuralType converts checker composites child-first, then
// interns their AIR shape through the same canonical path used by synthesized
// lowering types. Valid recursive graphs cross a nominal type, whose ID is
// reserved before its structural children are visited.
func (l *lowerer) internCheckerStructuralType(t checker.Type) (TypeID, bool, error) {
	internElem := func(elem checker.Type, kind TypeKind) (TypeID, bool, error) {
		elemID, err := l.internType(elem)
		if err != nil {
			return NoType, true, err
		}
		id, err := l.typeInterner.internStructural(TypeInfo{Kind: kind, Elem: elemID})
		return id, true, err
	}

	switch typ := t.(type) {
	case *checker.List:
		return internElem(typ.Of(), TypeList)
	case *checker.Slice:
		return internElem(typ.Of(), TypeSlice)
	case *checker.FixedArray:
		elem, err := l.internType(typ.Of())
		if err != nil {
			return NoType, true, err
		}
		id, err := l.typeInterner.internStructural(TypeInfo{Kind: TypeFixedArray, Elem: elem, Length: typ.Len()})
		return id, true, err
	case *checker.Chan:
		return internElem(typ.Of(), TypeChannel)
	case *checker.Receiver:
		return internElem(typ.Of(), TypeReceiver)
	case *checker.Sender:
		return internElem(typ.Of(), TypeSender)
	case *checker.Map:
		key, err := l.internType(typ.Key())
		if err != nil {
			return NoType, true, err
		}
		value, err := l.internType(typ.Value())
		if err != nil {
			return NoType, true, err
		}
		id, err := l.typeInterner.internStructural(TypeInfo{Kind: TypeMap, Key: key, Value: value})
		return id, true, err
	case *checker.Maybe:
		return internElem(typ.Of(), TypeMaybe)
	case *checker.Result:
		value, err := l.internType(typ.Val())
		if err != nil {
			return NoType, true, err
		}
		errType, err := l.internType(typ.Err())
		if err != nil {
			return NoType, true, err
		}
		id, err := l.typeInterner.internStructural(TypeInfo{Kind: TypeResult, Value: value, Error: errType})
		return id, true, err
	case *checker.FunctionDef:
		params := make([]TypeID, len(typ.Parameters))
		for index, param := range typ.Parameters {
			paramType, err := l.internFunctionParamType(param, l.internType)
			if err != nil {
				return NoType, true, err
			}
			params[index] = paramType
		}
		returnType, err := l.internType(typ.ReturnType)
		if err != nil {
			return NoType, true, err
		}
		variadic := len(typ.Parameters) > 0 && typ.Parameters[len(typ.Parameters)-1].Variadic
		id, err := l.typeInterner.internStructural(TypeInfo{Kind: TypeFunction, Params: params, Return: returnType, Variadic: variadic})
		return id, true, err
	default:
		return NoType, false, nil
	}
}
