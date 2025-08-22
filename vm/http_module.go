package vm

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/vm/runtime"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct{}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Program() *checker.Program {
	return nil
}

/*
 * examples
 * - "/foo/bar" -> "/foo/bar"
 * - "/foo/:bar" -> "/foo/{bar}"
 * - "/foo/:bar/:qux" -> "/foo/{bar}/{qux}"
 */
func convertToGoPattern(path string) string {
	// Convert :param to {param} format
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + part[1:] + "}"
		}
	}

	return strings.Join(parts, "/")
}

func (m *HTTPModule) Handle(vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "serve":
		port := args[0].Raw().(int)
		handlers := args[1].Raw().(map[string]*runtime.Object)

		_mux := http.NewServeMux()
		for path, handler := range handlers {
			_mux.HandleFunc(convertToGoPattern(path), func(w http.ResponseWriter, r *http.Request) {
				// Convert Go request to  http::Request
				headers := make(map[string]*runtime.Object)
				for k, v := range r.Header {
					if len(v) > 0 {
						headers[k] = runtime.MakeStr(v[0])
					}
				}

				bodyType := checker.HttpRequestDef.Fields["body"]
				var body *runtime.Object
				if r.Body != nil {
					bodyBytes, err := io.ReadAll(r.Body)
					if err == nil {
						body = runtime.Make(string(bodyBytes), bodyType)
					} else {
						body = runtime.Make(nil, bodyType)
					}
					r.Body.Close()
				} else {
					body = runtime.Make(nil, bodyType)
				}

				// Convert HTTP method string to Method enum
				var methodEnum *runtime.Object
				switch r.Method {
				case "GET":
					methodEnum = runtime.Make(int8(0), checker.HttpMethodDef) // Get variant
				case "POST":
					methodEnum = runtime.Make(int8(1), checker.HttpMethodDef) // Post variant
				case "PUT":
					methodEnum = runtime.Make(int8(2), checker.HttpMethodDef) // Put variant
				case "DELETE":
					methodEnum = runtime.Make(int8(3), checker.HttpMethodDef) // Del variant
				case "PATCH":
					methodEnum = runtime.Make(int8(4), checker.HttpMethodDef) // Patch variant
				case "OPTIONS":
					methodEnum = runtime.Make(int8(5), checker.HttpMethodDef) // Patch variant
				default:
					methodEnum = runtime.Make(int8(0), checker.HttpMethodDef) // Default to Get
				}

				requestMap := map[string]*runtime.Object{
					"method":  methodEnum,
					"url":     runtime.MakeStr(r.URL.String()),
					"headers": runtime.Make(headers, checker.MakeMap(checker.Str, checker.Str)),
					"body":    body,
					"path": runtime.Make(func() *runtime.Object {
						return runtime.MakeStr(r.URL.Path)
					}, checker.HttpRequestDef.Methods["path"]),
					"path_param": runtime.Make(func(args ...*runtime.Object) *runtime.Object {
						name := args[0].Raw().(string)
						return runtime.MakeStr(r.PathValue(name))
					}, checker.HttpRequestDef.Methods["path_param"]),
					"query_param": runtime.Make(func(args ...*runtime.Object) *runtime.Object {
						name := args[0].Raw().(string)
						return runtime.MakeStr(r.URL.Query().Get(name))
					}, checker.HttpRequestDef.Methods["query_param"]),
				}

				request := runtime.MakeStruct(checker.HttpRequestDef, requestMap)

				// Call the Ard handler function
				handle, ok := handler.Raw().(*Closure)
				if !ok {
					panic(fmt.Errorf("Handler for '%s' is not a function", path))
				}

				// Create a copy of the closure with a new VM for isolation to prevent race conditions
				// This follows the same pattern as the async module
				isolatedHandle := *handle
				isolatedHandle.vm = New(vm.imports)
				response := isolatedHandle.eval(request)

				// Convert Ard Response to Go HTTP response
				respMap := response.Raw().(map[string]*runtime.Object)
				status := respMap["status"].Raw().(int)
				responseBody := respMap["body"].Raw().(string)

				// Set response headers if present
				if headersObj, ok := respMap["headers"]; ok {
					if headersMap, ok := headersObj.Raw().(map[string]*runtime.Object); ok {
						for k, v := range headersMap {
							if strVal, ok := v.Raw().(string); ok {
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

		return runtime.Void() // Server runs indefinitely
	case "respond":
		status := args[0].Raw().(int)
		body := args[1].Raw().(string)

		responseMap := map[string]*runtime.Object{
			"status":  runtime.MakeInt(status),
			"headers": runtime.Make(make(map[string]*runtime.Object), checker.MakeMap(checker.Str, checker.Str)),
			"body":    runtime.MakeStr(body),
		}

		return runtime.MakeStruct(checker.HttpResponseDef, responseMap)
	case "send":
		// Cast back to *VM to access the original evalHttpSend function
		// This preserves the existing complex HTTP logic
		request := args[0]
		requestMap := request.Raw().(map[string]*runtime.Object)

		// Convert Method enum to string
		methodEnum := requestMap["method"]
		var method string
		switch methodEnum.Raw().(int8) {
		case 0: // Get
			method = "GET"
		case 1: // Post
			method = "POST"
		case 2: // Put
			method = "PUT"
		case 3: // Del
			method = "DELETE"
		case 4: // Patch
			method = "PATCH"
		case 5: // Options
			method = "OPTIONS"
		default:
			method = "GET"
		}
		url := requestMap["url"].Raw().(string)

		var body io.Reader = nil

		if bodyObj, ok := requestMap["body"]; ok && bodyObj.Raw() != nil {
			body = strings.NewReader(bodyObj.Raw().(string))
		}

		headers := make(http.Header)
		rawHeaders := requestMap["headers"].Raw().(map[string]*runtime.Object)
		for k, v := range rawHeaders {
			if strVal, ok := v.Raw().(string); ok {
				headers.Set(k, strVal)
			}
		}

		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		req, err := http.NewRequest(method, url, body)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}

		req.Header = headers

		resp, err := client.Do(req)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		defer resp.Body.Close()

		// Read the response body
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return runtime.MakeErr(runtime.MakeStr(err.Error()))
		}
		bodyStr := string(bodyBytes)

		// Create response headers map
		respHeadersMap := make(map[string]*runtime.Object)
		for k, v := range resp.Header {
			if len(v) > 0 {
				respHeadersMap[k] = runtime.MakeStr(v[0])
			}
		}

		// Create the response object
		respMap := map[string]*runtime.Object{
			"status":  runtime.MakeInt(resp.StatusCode),
			"headers": runtime.Make(respHeadersMap, checker.MakeMap(checker.Str, checker.Str)),
			"body":    runtime.MakeStr(bodyStr),
		}

		return runtime.MakeOk(runtime.MakeStruct(checker.HttpResponseDef, respMap))
	default:
		panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
	}
}

func (m *HTTPModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch structName {
	case "Response":
		return m.handleResponseStatic(call, args)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s::%s()", structName, call.Name))
	}
}

func (m *HTTPModule) handleResponseStatic(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch call.Name {
	case "new":
		// Response::new(status: Int, body: Str) -> Response
		if len(args) != 2 {
			panic(fmt.Errorf("Response::new expects 2 arguments, got %d", len(args)))
		}

		status := args[0].Raw().(int)
		body := args[1].Raw().(string)

		// Create response object structure
		respMap := map[string]*runtime.Object{
			"status":  runtime.MakeInt(status),
			"headers": runtime.Make(make(map[string]*runtime.Object), checker.MakeMap(checker.Str, checker.Str)),
			"body":    runtime.MakeStr(body),
		}

		return runtime.MakeStruct(checker.HttpResponseDef, respMap)
	default:
		panic(fmt.Errorf("Unimplemented: Response::%s()", call.Name))
	}
}

// do HTTP::Response methods
func (mod *HTTPModule) evalHttpResponseMethod(response *runtime.Object, method *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	// Get raw response struct
	respMap, ok := response.Raw().(map[string]*runtime.Object)
	if !ok {
		fmt.Println("HTTP Error: Response is not correctly formatted")
		return runtime.Make(nil, method.Type())
	}

	switch method.Name {
	case "is_ok":
		{
			status := respMap["status"].Raw().(int)
			return runtime.MakeBool(status >= 200 && status <= 300)
		}
	default:
		panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
	}
}

// do HTTP::Request methods
func (mod *HTTPModule) evalHttpRequestMethod(request *runtime.Object, method *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	reqMap, ok := request.Raw().(map[string]*runtime.Object)
	if !ok {
		fmt.Println("HTTP Error: Response is not correctly formatted")
		return runtime.Make(nil, method.Type())
	}

	switch method.Name {
	case "path":
		{
			parsed, err := url.Parse(reqMap["url"].Raw().(string))
			if err != nil {
				return runtime.MakeErr(runtime.MakeStr(err.Error()))
			}
			return runtime.MakeOk(runtime.MakeStr(parsed.Path))
		}
	default:
		if raw, ok := reqMap[method.Name]; ok {
			fn, ok := raw.Raw().(func(...*runtime.Object) *runtime.Object)
			if !ok {
				panic(fmt.Errorf("HTTP Error: Request method is not a function: %s", method.Name))
			}
			return fn(args...)
		}
	}
	panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
}
