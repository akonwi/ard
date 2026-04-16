package io

import (
	ardgo "github.com/akonwi/ard/go"
)

func print(string string) {
	_, err := ardgo.CallExtern("Print", string)
	if err != nil {
		panic(err)
	}
}

func Print(value ardgo.ToString) {
	print(value.ToStr())
}

func ReadLine() ardgo.Result[string, string] {
	result, err := ardgo.CallExtern("ReadLine")
	if err != nil {
		panic(err)
	}
	return ardgo.CoerceExtern[ardgo.Result[string, string]](result)
}
