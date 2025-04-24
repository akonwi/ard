package checker_v2

type Type interface {
	String() string
	get(name string) Type
}

type str struct{}

func (s str) String() string { return "Str" }
func (s str) get(name string) Type {
	switch name {
	case "size":
		return Int
	default:
		return nil
	}
}

var Str = &str{}

type _int struct{}

func (i _int) String() string { return "Int" }
func (i _int) get(name string) Type {
	switch name {
	default:
		return nil
	}
}

var Int = &_int{}

type float struct{}

func (f float) String() string { return "Float" }
func (f float) get(name string) Type {
	switch name {
	default:
		return nil
	}
}

var Float = &float{}

type _bool struct{}

func (b _bool) String() string { return "Bool" }
func (b _bool) get(name string) Type {
	switch name {
	default:
		return nil
	}
}

var Bool = &_bool{}

type void struct{}

func (v void) String() string       { return "Void" }
func (v void) get(name string) Type { return nil }

var Void = &void{}
