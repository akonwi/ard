// Example: How to add custom FFI functions with the new uniform signature
// All FFI functions now follow this pattern:
//
//   func myFunction(args []*object) (*object, error)
//
// This eliminates reflection and provides:
// - ⚡ Direct function calls (no reflection overhead)
// - 🔍 Type safety within each function  
// - 🛡️ Consistent error handling
// - 🧹 Clean, uniform signatures

package main

import (
	"fmt"
	"strings"

	"github.com/akonwi/ard/checker"
)

// Example: Custom string manipulation function
func ffi_string_reverse(args []*object) (*object, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("string_reverse expects 1 argument, got %d", len(args))
	}
	
	str, ok := args[0].raw.(string)
	if !ok {
		return nil, fmt.Errorf("string_reverse expects string argument, got %T", args[0].raw)
	}
	
	// Reverse the string
	runes := []rune(str)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	
	result := string(runes)
	return &object{raw: result, _type: checker.Str}, nil
}

// Example: Custom math function  
func ffi_math_sum_all(args []*object) (*object, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("sum_all expects at least 1 argument")
	}
	
	sum := 0
	for i, arg := range args {
		val, ok := arg.raw.(int)
		if !ok {
			return nil, fmt.Errorf("sum_all expects int arguments, got %T for argument %d", arg.raw, i)
		}
		sum += val
	}
	
	return &object{raw: sum, _type: checker.Int}, nil
}

// Example: Function that works with lists
func ffi_list_join(args []*object) (*object, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("list_join expects 2 arguments (list, separator), got %d", len(args))
	}
	
	// Get the list
	list, ok := args[0].raw.([]*object)
	if !ok {
		return nil, fmt.Errorf("first argument must be a list, got %T", args[0].raw)
	}
	
	// Get the separator
	separator, ok := args[1].raw.(string)
	if !ok {
		return nil, fmt.Errorf("second argument must be a string separator, got %T", args[1].raw)
	}
	
	// Convert list elements to strings and join
	var parts []string
	for _, item := range list {
		parts = append(parts, fmt.Sprintf("%v", item.raw))
	}
	
	result := strings.Join(parts, separator)
	return &object{raw: result, _type: checker.Str}, nil
}

/*
Usage in Ard:

extern fn string_reverse(s: Str) Str = "string.reverse"
extern fn math_average(numbers: [Int]) Int = "math.average"  
extern fn list_join(items: [Str], sep: Str) Str = "list.join"

fn main() {
    let reversed = string_reverse("hello")
    print(reversed)  // "olleh"
    
    let avg = math_average([1, 2, 3, 4, 5])
    print(avg)  // 3
    
    let joined = list_join(["a", "b", "c"], ", ")
    print(joined)  // "a, b, c"
}
*/
