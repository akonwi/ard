package checker

import "fmt"

type TypedIntLiteral struct {
	Value int
	Text  string
	Typed Type
}

func (i *TypedIntLiteral) String() string {
	if i.Text != "" {
		return i.Text
	}
	return fmt.Sprintf("%d", i.Value)
}
func (i *TypedIntLiteral) Type() Type { return i.Typed }

type TypedFloatLiteral struct {
	Value float64
	Text  string
	Typed Type
}

func (f *TypedFloatLiteral) String() string {
	if f.Text != "" {
		return f.Text
	}
	return fmt.Sprintf("%g", f.Value)
}
func (f *TypedFloatLiteral) Type() Type { return f.Typed }
