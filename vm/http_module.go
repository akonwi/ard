package vm

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct{}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Handle(vm *VM, call *checker.FunctionCall) *object {
	switch call.Name {
	case "send":
		// Cast back to *VM to access the original evalHttpSend function
		// This preserves the existing complex HTTP logic
		request := vm.Eval(call.Args[0])
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

		resultType := call.Type().(*checker.Result)

		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}

		req.Header = headers

		resp, err := client.Do(req)
		if err != nil {
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
		}
		defer resp.Body.Close()

		// Read the response body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return makeErr(&object{err.Error(), resultType.Err()}, resultType)
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

		return makeOk(&object{respMap, resultType.Val()}, resultType)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
	}
}

// do HTTP::Response methods
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

			// autoimport ard/json so we can call json::decode
			vm.moduleRegistry.Register(&JSONModule{})

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
