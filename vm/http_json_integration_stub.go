//go:build !goexperiment.jsonv2

package vm

import (
	"fmt"

	"github.com/akonwi/ard/checker"
)

func (mod *HTTPModule) decodeJsonResponse(bodyStr string, returnType checker.Type) *object {
	fmt.Println("HTTP Error: JSON decoding not available - requires goexperiment.jsonv2 build tag")
	return &object{nil, returnType}
}
