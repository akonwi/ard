package air

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func declarationNominalKey(kind TypeKind, modulePath, name string) nominalTypeKey {
	return nominalTypeKey{Kind: kind, ModulePath: modulePath, Name: name}
}

func applicationNominalKey(kind TypeKind, definition TypeID, args []TypeID) nominalTypeKey {
	return nominalTypeKey{Kind: kind, Definition: definition, Args: typeIDsKey(args)}
}

func foreignNominalKey(typ *checker.ForeignType, args []TypeID) nominalTypeKey {
	return nominalTypeKey{
		Kind:      TypeForeignType,
		Target:    typ.Target,
		Namespace: typ.Namespace,
		Symbol:    typ.Name,
		Pointer:   typ.Pointer,
		Args:      typeIDsKey(args),
	}
}

func (l *lowerer) internNominalStruct(typ *checker.StructDef, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	key := declarationNominalKey(TypeStruct, typ.ModulePath, typ.Name)
	seed := TypeInfo{Kind: TypeStruct, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private}
	id, build, err := l.typeInterner.reserveNominal(key, seed)
	if err != nil {
		return NoType, err
	}
	if !build {
		return id, nil
	}
	info := seed
	structFields := checker.StructFields(typ)
	fields := sortedFieldNames(structFields)
	info.Fields = make([]FieldInfo, len(fields))
	for index, name := range fields {
		fieldType, fieldErr := l.internStructFieldType(structFields[name], intern)
		if fieldErr != nil {
			return NoType, l.typeInterner.failNominal(key, fmt.Errorf("lower field %s.%s: %w", typ.Name, name, fieldErr))
		}
		info.Fields[index] = lowerStructFieldInfo(typ, name, fieldType, index)
	}
	return l.typeInterner.completeNominal(key, info)
}

func (l *lowerer) internNominalUnion(typ *checker.Union, intern func(checker.Type) (TypeID, error)) (TypeID, error) {
	key := declarationNominalKey(TypeUnion, typ.ModulePath, typ.Name)
	seed := TypeInfo{Kind: TypeUnion, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private}
	id, build, err := l.typeInterner.reserveNominal(key, seed)
	if err != nil || !build {
		return id, err
	}
	info := seed
	info.Members = make([]UnionMember, len(typ.Types))
	for index, member := range typ.Types {
		memberID, memberErr := intern(member)
		if memberErr != nil {
			return NoType, l.typeInterner.failNominal(key, memberErr)
		}
		info.Members[index] = UnionMember{Type: memberID, Tag: uint32(index), Name: member.String()}
	}
	return l.typeInterner.completeNominal(key, info)
}

func (l *lowerer) internNominalEnum(typ *checker.Enum) (TypeID, error) {
	key := declarationNominalKey(TypeEnum, typ.ModulePath, typ.Name)
	seed := TypeInfo{Kind: TypeEnum, Name: typ.Name, ModulePath: typ.ModulePath, Private: typ.Private}
	_, _, err := l.typeInterner.reserveNominal(key, seed)
	if err != nil {
		return NoType, err
	}
	info := seed
	info.EnumOpen = typ.Open
	info.Variants = make([]VariantInfo, len(typ.Values))
	for index, variant := range typ.Values {
		info.Variants[index] = VariantInfo{Name: variant.Name, Discriminant: variant.Value}
	}
	return l.typeInterner.completeNominal(key, info)
}
