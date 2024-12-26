package vm

type variable struct {
	mut   bool
	value any
}

type scope struct {
	variables map[string]variable
}

func newScope() *scope {
	return &scope{variables: make(map[string]variable)}
}
