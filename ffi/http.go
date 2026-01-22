package ffi

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/akonwi/ard/checker"
	"github.com/akonwi/ard/runtime"
)

// fn (req: Dynamic) Str
func GetReqPath(args []*runtime.Object, _ checker.Type) *runtime.Object {
	req := args[0].Raw().(*http.Request)
	return runtime.MakeStr(req.URL.Path)
}

// fn (req: Dynamic, name: Str) Str
func GetPathValue(args []*runtime.Object, _ checker.Type) *runtime.Object {
	req := args[0].Raw().(*http.Request)
	name := args[1].Raw().(string)
	return runtime.MakeStr(req.PathValue(name))
}

// fn (req: Dynamic, name: Str) Str
func GetQueryParam(args []*runtime.Object, _ checker.Type) *runtime.Object {
	req := args[0].Raw().(*http.Request)
	name := args[1].Raw().(string)
	return runtime.MakeStr(req.URL.Query().Get(name))
}

// fn (method: Str, url: Str, body: Str, headers: [Str:Str]) Response!Str
func HTTP_Send(args []*runtime.Object, returnType checker.Type) *runtime.Object {
	method := args[0].AsString()
	url := args[1].AsString()
	body := func() io.Reader {
		str := ""
		if string, ok := args[2].IsStr(); ok {
			str = string
		}
		return strings.NewReader(str)
	}()
	headers := make(http.Header)

	for k, v := range args[3].AsMap() {
		headers.Set(k, v.AsString())
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
	respHeadersMap := runtime.MakeMap(checker.Str, checker.Str)
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeadersMap.Map_Set(runtime.MakeStr(k), runtime.MakeStr(v[0]))
		}
	}

	// Create the response object
	respMap := map[string]*runtime.Object{
		"status":  runtime.MakeInt(resp.StatusCode),
		"headers": respHeadersMap,
		"body":    runtime.MakeStr(bodyStr),
	}

	resultType := returnType.(*checker.Result)

	return runtime.MakeOk(runtime.MakeStruct(resultType.Val(), respMap))
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

// fn serve(port: Int, handlers: [Str:fn(Request) Response])
func HTTP_Serve(args []*runtime.Object, _ checker.Type) *runtime.Object {
	port := args[0].AsInt()
	handlers := args[1].AsMap()

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

			body := runtime.MakeNone(checker.Str)
			if r.Body != nil {
				bodyBytes, err := io.ReadAll(r.Body)
				if err == nil {
					body.Set(string(bodyBytes))
				}
				r.Body.Close()
			}

			handle, ok := handler.Raw().(runtime.Closure)
			if !ok {
				panic(fmt.Errorf("Handler for '%s' is not a function", path))
			}

			methodEnumType := handle.GetParams()[0].Type.(*checker.StructDef).Fields["method"].(*checker.Enum)

			// Convert HTTP method string to Method enum discriminant value
			var methodValue int
			switch r.Method {
			case "GET":
				methodValue = methodEnumType.Values[0].Value // Get variant
			case "POST":
				methodValue = methodEnumType.Values[1].Value // Post variant
			case "PUT":
				methodValue = methodEnumType.Values[2].Value // Put variant
			case "DELETE":
				methodValue = methodEnumType.Values[3].Value // Del variant
			case "PATCH":
				methodValue = methodEnumType.Values[4].Value // Patch variant
			case "OPTIONS":
				methodValue = methodEnumType.Values[5].Value // Options variant
			default:
				methodValue = methodEnumType.Values[0].Value // Default to Get
			}
			method := runtime.Make(methodValue, methodEnumType)

			requestMap := map[string]*runtime.Object{
				"method":  method,
				"url":     runtime.MakeStr(r.URL.String()),
				"headers": runtime.Make(headers, checker.MakeMap(checker.Str, checker.Str)),
				"body":    body,
				"raw":     runtime.MakeNone(checker.Dynamic).ToSome(r),
			}

			handlerMapType := args[1].Type().(*checker.Map)
			requestType := handlerMapType.Value().(*checker.FunctionDef).Parameters[0].Type
			request := runtime.MakeStruct(requestType, requestMap)

			// Call the Ard handler function
			// Create a copy of the closure with a new VM for isolation to prevent race conditions
			// This follows the same pattern as the async module
			response := handle.EvalIsolated(request)

			// Convert Ard Response to Go HTTP response
			respMap := response.AsMap()
			status := respMap["status"].AsInt()
			responseBody := respMap["body"].AsString()

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
		return runtime.MakeErr(runtime.MakeStr(err.Error()))
	}
	return runtime.MakeOk(runtime.Void())
}
