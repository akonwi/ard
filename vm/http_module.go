package vm

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct{}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "serve":
		port := args[0].raw.(int)
		handlers := args[1].raw.(map[string]*object)

		_mux := http.NewServeMux()
		for path, handler := range handlers {
			_mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
				// Convert Go request to  http::Request
				headers := make(map[string]*object)
				for k, v := range r.Header {
					if len(v) > 0 {
						headers[k] = &object{v[0], checker.Str}
					}
				}

				var body *object
				if r.Body != nil {
					bodyBytes, err := io.ReadAll(r.Body)
					if err == nil {
						body = &object{string(bodyBytes), checker.Str}
					} else {
						body = &object{"", checker.Str}
					}
					r.Body.Close()
				} else {
					body = &object{"", checker.Str}
				}

				requestMap := map[string]*object{
					"method":  {r.Method, checker.Str},
					"url":     {r.URL.String(), checker.Str},
					"headers": {headers, checker.MakeMap(checker.Str, checker.Str)},
					"body":    body,
					"path": {func() *object {
						return &object{r.URL.Path, checker.Str}
					}, checker.HttpRequestDef.Fields["path"]},
				}

				request := &object{requestMap, checker.HttpRequestDef}

				// Call the Ard handler function
				handle, ok := handler.raw.(func(args ...*object) *object)
				if !ok {
					panic(fmt.Errorf("Handler for '%s' is not a function", path))
				}
				response := handle(request)

				// Convert Ard Response to Go HTTP response
				respMap := response.raw.(map[string]*object)
				status := respMap["status"].raw.(int)
				responseBody := respMap["body"].raw.(string)

				// Set response headers if present
				if headersObj, ok := respMap["headers"]; ok {
					if headersMap, ok := headersObj.raw.(map[string]*object); ok {
						for k, v := range headersMap {
							if strVal, ok := v.raw.(string); ok {
								w.Header().Set(k, strVal)
							}
						}
					}
				}

				w.WriteHeader(status)
				w.Write([]byte(responseBody))
			})
		}

		err := http.ListenAndServe(fmt.Sprintf(":%d", port), _mux)
		if err != nil {
			panic(fmt.Errorf("Server failed: %v", err))
		}

		return &object{nil, nil} // Server runs indefinitely
	case "respond":
		status := args[0].raw.(int)
		body := args[1].raw.(string)

		responseMap := map[string]*object{
			"status":  &object{status, checker.Int},
			"headers": &object{make(map[string]*object), checker.MakeMap(checker.Str, checker.Str)},
			"body":    &object{body, checker.Str},
		}

		return &object{responseMap, checker.HttpResponseDef}
	case "send":
		// Cast back to *VM to access the original evalHttpSend function
		// This preserves the existing complex HTTP logic
		request := args[0]
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
func (mod *HTTPModule) evalHttpResponseMethod(response *object, method *checker.FunctionCall, args []*object) *object {
	// Get raw response struct
	respMap, ok := response.raw.(map[string]*object)
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
			jsonMod := &JSONModule{}
			vm := New(map[string]checker.Module{})
			return jsonMod.Handle(vm, checker.CreateCall("decode",
				[]checker.Expression{&checker.StrLiteral{Value: bodyStr}},
				checker.FunctionDef{
					ReturnType: method.Type(),
				},
			), []*object{&object{bodyStr, checker.Str}})
		}
	default:
		panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
	}
}

// do HTTP::Request methods
func (mod *HTTPModule) evalHttpRequestMethod(request *object, method *checker.FunctionCall, args []*object) *object {
	reqMap, ok := request.raw.(map[string]*object)
	if !ok {
		fmt.Println("HTTP Error: Response is not correctly formatted")
		return &object{nil, method.Type()}
	}

	switch method.Name {
	case "path":
		{
			parsed, err := url.Parse(reqMap["url"].raw.(string))
			resultType := method.Type().(*checker.Result)
			if err != nil {
				return makeErr(&object{err.Error(), resultType.Err()}, resultType)
			}
			return makeOk(&object{parsed.Path, resultType.Val()}, resultType)
		}
	default:
		panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
	}
}
