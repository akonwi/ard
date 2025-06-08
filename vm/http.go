package vm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
)



func evalHttpSend(vm *VM, call *checker.FunctionCall) *object {
	request := vm.eval(call.Args[0])
	requestMap := request.raw.(map[string]*object)

	method := requestMap["method"].raw.(string)
	url := requestMap["url"].raw.(string)

	var body io.Reader = nil

	if bodyObj, ok := requestMap["body"]; ok && bodyObj.raw != nil {
		body = strings.NewReader(bodyObj.raw.(string))
	}

	headers := make(http.Header)
	rawHeaders := requestMap["headers"].raw.(map[string]*object)
	for k, v := range rawHeaders {
		if strVal, ok := v.raw.(string); ok {
			headers.Set(k, strVal)
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		fmt.Printf("HTTP Error creating request: %v\n", err)
		return &object{nil, call.Type()}
	}

	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("HTTP Error executing request: %v\n", err)
		return &object{nil, call.Type()}
	}
	defer resp.Body.Close()

	// Read the response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("HTTP Error reading response body: %v\n", err)
		return &object{nil, call.Type()}
	}
	bodyStr := string(bodyBytes)

	// Create response headers map
	respHeadersMap := make(map[string]*object)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeadersMap[k] = &object{v[0], checker.Str}
		}
	}

	// Create the response object
	respMap := map[string]*object{
		"status":  &object{resp.StatusCode, checker.Int},
		"headers": &object{respHeadersMap, checker.MakeMap(checker.Str, checker.Str)},
		"body":    &object{bodyStr, checker.Str},
	}

	return &object{respMap, call.Type()}
}

// Handle HTTP Response method
func (vm *VM) evalHttpResponseMethod(resp *object, method *checker.FunctionCall) *object {
	// Get raw response struct
	respMap, ok := resp.raw.(map[string]*object)
	if !ok {
		fmt.Println("HTTP Error: Response is not correctly formatted")
		return &object{nil, method.Type()}
	}

	switch method.Name {
	case "is_ok":
		{
			status := respMap["status"].raw.(int)
			return &object{status >= 200 && status <= 300, method.Type()}
		}
	case "json":
		{
			// Get the body
			bodyObj, ok := respMap["body"]
			if !ok || bodyObj == nil {
				fmt.Println("HTTP Error: Response missing body")
				return &object{nil, method.Type()}
			}

			// Cast body to string
			bodyStr, ok := bodyObj.raw.(string)
			if !ok || bodyStr == "" {
				fmt.Println("HTTP Error: Response body is not a string or is empty")
				return &object{nil, method.Type()}
			}

			// Create a synthetic function call to json::decode()
			res := vm.eval(&checker.ModuleFunctionCall{
			Module: "ard/json",
				Call: checker.CreateCall("decode",
					[]checker.Expression{&checker.StrLiteral{Value: bodyStr}},
					checker.FunctionDef{
						ReturnType: method.Type(),
					},
				),
			})
			return res
		}
	default:
		panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
	}
}
