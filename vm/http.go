package vm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
)

func evalInHTTP(vm *VM, call *checker.FunctionCall) *object {
	switch call.Name {
	case "get":
		return evalHttpGet(vm, call)
	case "post":
		return evalHttpPost(vm, call)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
	}
}

func evalHttpGet(vm *VM, call *checker.FunctionCall) *object {
	request := vm.eval(call.Args[0])
	// Extract the request parameters
	requestMap := request.raw.(map[string]*object)

	// Get URL (required parameter)
	urlObj, urlOk := requestMap["url"]
	if !urlOk || urlObj == nil {
		fmt.Println("HTTP Error: Missing required 'url' parameter in request")
		return &object{nil, call.Type()}
	}
	url, ok := urlObj.raw.(string)
	if !ok {
		fmt.Println("HTTP Error: 'url' parameter must be a string")
		return &object{nil, call.Type()}
	}

	// Get headers (required parameter)
	headersObj, headersOk := requestMap["headers"]
	if !headersOk || headersObj == nil {
		fmt.Println("HTTP Error: Missing required 'headers' parameter in request")
		return &object{nil, call.Type()}
	}

	// Process headers
	headers := make(http.Header)
	if rawHeaders, ok := headersObj.raw.(map[string]*object); ok {
		for k, v := range rawHeaders {
			if strVal, ok := v.raw.(string); ok {
				headers.Set(k, strVal)
			}
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("HTTP Error creating request: %v\n", err)
		return &object{nil, call.Type()}
	}

	// Add headers to request
	req.Header = headers

	// Execute the request
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

func evalHttpPost(vm *VM, call *checker.FunctionCall) *object {
	request := vm.eval(call.Args[0])
	requestMap := request.raw.(map[string]*object)

	urlObj, urlOk := requestMap["url"]
	if !urlOk || urlObj == nil {
		fmt.Println("HTTP Error: Missing required 'url' parameter in request")
		return &object{nil, call.Type()}
	}
	url, ok := urlObj.raw.(string)
	if !ok {
		fmt.Println("HTTP Error: 'url' parameter must be a string")
		return &object{nil, call.Type()}
	}

	headersObj, headersOk := requestMap["headers"]
	if !headersOk || headersObj == nil {
		fmt.Println("HTTP Error: Missing required 'headers' parameter in request")
		return &object{nil, call.Type()}
	}

	var body io.Reader = nil

	if bodyObj, ok := requestMap["body"]; ok {
		body = strings.NewReader(bodyObj.raw.(string))
	}

	headers := make(http.Header)
	if rawHeaders, ok := headersObj.raw.(map[string]*object); ok {
		for k, v := range rawHeaders {
			if strVal, ok := v.raw.(string); ok {
				headers.Set(k, strVal)
			}
		}
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		fmt.Printf("HTTP Error creating request: %v\n", err)
		return &object{nil, call.Type()}
	}

	// Add headers to request
	req.Header = headers

	// Execute the request
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

// Handle HTTP Response json method
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

			// fmt.Printf("bodyStr:\n\t%s\n", bodyStr)

			// Use the existing JSON decoding logic
			// Create a synthetic function call to reuse the existing JSON decode logic
			res := vm.eval(&checker.PackageFunctionCall{
				Package: "ard/json",
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
