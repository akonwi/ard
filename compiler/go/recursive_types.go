package gotarget

import "github.com/akonwi/ard/air"

func structFieldByName(typ air.TypeInfo, name string) (air.FieldInfo, bool) {
	for _, field := range typ.Fields {
		if field.Name == name {
			return field, true
		}
	}
	return air.FieldInfo{}, false
}
