package checker_v2

type Type interface {
	String() string
}

type str struct{}

func (s str) String() string { return "Str" }

var Str = &str{}

type _int struct{}

func (i _int) String() string { return "Int" }

var Int = &_int{}

type float struct{}

func (f float) String() string { return "Float" }

var Float = &float{}

type _bool struct{}

func (b _bool) String() string { return "Bool" }

var Bool = &_bool{}
