package vm

import (
	"fmt"
)

type IO struct{}

func (io IO) print(string string) {
	fmt.Println(string)
}
