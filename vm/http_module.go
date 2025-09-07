package vm

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// HTTPModule handles ard/http module functions
type HTTPModule struct {
	vm *VM
	hq *GlobalVM
}

func (m *HTTPModule) Path() string {
	return "ard/http"
}

func (m *HTTPModule) Program() *checker.Program {
	return nil
}

func (m *HTTPModule) get(name string) *runtime.Object {
	sym, _ := m.vm.scope.get(name)
	return sym
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

func (m *HTTPModule) Handle(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
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
				}

				request := runtime.MakeStruct(checker.HttpRequestDef, requestMap)

				// Call the Ard handler function
				handle, ok := handler.Raw().(*VMClosure)
				if !ok {
					panic(fmt.Errorf("Handler for '%s' is not a function", path))
				}

				// Create a copy of the closure with a new VM for isolation to prevent race conditions
				// This follows the same pattern as the async module
				isolatedHandle := handle.copy()
				isolatedHandle.vm = NewVM()
				isolatedHandle.vm.hq = m.hq
				response := isolatedHandle.Eval(request)

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
	default:
		panic(fmt.Errorf("Unimplemented: http::%s()", call.Name))
	}
}

func (m *HTTPModule) HandleStatic(structName string, call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	switch structName {
	case "Response":
		return m.handleResponseStatic(call, args)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s::%s()", structName, call.Name))
	}
}

func (m *HTTPModule) handleResponseStatic(call *checker.FunctionCall, args []*runtime.Object) *runtime.Object {
	panic(fmt.Errorf("Unimplemented: Response::%s()", call.Name))
}
