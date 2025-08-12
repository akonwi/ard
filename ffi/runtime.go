package main

import (
	"bufio"
	"fmt"
	"os"
)

// go_print prints a value to stdout
// Used by extern fn print(value: $T) Void = "runtime.go_print"
func go_print(value any) {
	fmt.Println(value)
}

// go_read_line reads a line from stdin
// Used by extern fn read_line() Str = "runtime.go_read_line"
func go_read_line() string {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text()
	}
	return ""
}
