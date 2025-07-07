package vm

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"

	"github.com/akonwi/ard/checker"
)

func (o *object) MarshalSONTo(enc *jsontext.Encoder) error {
	if o._type == checker.Str || o._type == checker.Int || o._type == checker.Float || o._type == checker.Bool {
		str, err := json.Marshal(&o.raw)
		fmt.Printf("marshalling: %v = %s\n", o.raw, str)
		if err != nil {
			return err
		}
		enc.WriteValue(jsontext.Value(str))
		return nil
	}
	return nil
}

func json_encode(data any, t checker.Type) ([]byte, error) {
	fmt.Printf("marshalling: %v: %s\n", data, t)
	// if m, ok := t.(*checker.Maybe); ok {
	// return json.Marshal(data)
	// }
	if t == checker.Str || t == checker.Int || t == checker.Float || t == checker.Bool || checker.IsMaybe(t) {
		str, err := json.Marshal(data)
		return str, err
	}
	return []byte{}, nil
}
