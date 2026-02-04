package bytecode

import (
	"bytes"
	"encoding/gob"
)

func SerializeProgram(program Program) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(program); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DeserializeProgram(data []byte) (Program, error) {
	var program Program
	dec := gob.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&program); err != nil {
		return Program{}, err
	}
	return program, nil
}
