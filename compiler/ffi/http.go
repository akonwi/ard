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

func requestBodyMaybe(r *http.Request) *runtime.Object {
	body := runtime.MakeNone(checker.Dynamic)
	if r.Body == nil {
		return body
	}

	bodyBytes, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		return body
	}

	if len(bodyBytes) == 0 {
		return body
	}

	return body.ToSome(string(bodyBytes))
}

// fn (req: Dynamic) Str
// GetReqPath extracts the URL path from an opaque *http.Request handle.
func GetReqPath(handle any) string {
	req := handle.(*http.Request)
	return req.URL.Path
}

// GetPathValue extracts a path parameter from an opaque *http.Request handle.
func GetPathValue(handle any, name string) string {
	req := handle.(*http.Request)
	return req.PathValue(name)
}

// GetQueryParam extracts a query parameter from an opaque *http.Request handle.
func GetQueryParam(handle any, name string) string {
	req := handle.(*http.Request)
	return req.URL.Query().Get(name)
}

// HTTP_Do executes an HTTP request and returns an opaque *http.Response handle.
// The caller must close the response using HTTP_ResponseClose.
func HTTP_Do(method, url string, body any, headers map[string]string, timeout *int) (any, error) {
	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = strings.NewReader(string(v))
		default:
			bodyReader = strings.NewReader(fmt.Sprintf("%v", v))
		}
	} else {
		bodyReader = strings.NewReader("")
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{}
	if timeout != nil {
		client.Timeout = time.Duration(*timeout) * time.Second
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// HTTP_ResponseStatus returns the HTTP status code from a response handle.
func HTTP_ResponseStatus(handle any) int {
	resp := handle.(*http.Response)
	return resp.StatusCode
}

// HTTP_ResponseHeader returns a single header value from a response handle.
// Returns nil if the header is not present.
func HTTP_ResponseHeader(handle any, name string) *string {
	resp := handle.(*http.Response)
	values := resp.Header.Values(name)
	if len(values) == 0 {
		return nil
	}
	return &values[0]
}

// HTTP_ResponseHeaders returns all headers from a response handle.
func HTTP_ResponseHeaders(handle any) map[string]string {
	resp := handle.(*http.Response)
	headers := make(map[string]string)
	for k, v := range resp.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers
}

// HTTP_ResponseBody reads and returns the response body.
// This consumes the body; subsequent calls will return an error.
func HTTP_ResponseBody(handle any) (string, error) {
	resp := handle.(*http.Response)
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(bodyBytes), nil
}

// HTTP_ResponseClose closes a response handle, freeing resources.
func HTTP_ResponseClose(handle any) {
	resp := handle.(*http.Response)
	_ = resp.Body.Close()
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

// fn serve(port: Int, handlers: [Str:fn(Request, mut Response)])
func HTTP_Serve(args []*runtime.Object) *runtime.Object {
	port := args[0].AsInt()
	handlers := args[1].AsMap()

	_mux := http.NewServeMux()
	for path, handler := range handlers {
		_mux.HandleFunc(convertToGoPattern(path), func(w http.ResponseWriter, r *http.Request) {
			// Convert Go request to http::Request
			headers := make(map[string]*runtime.Object)
			for k, v := range r.Header {
				if len(v) > 0 {
					headers[k] = runtime.MakeStr(v[0])
				}
			}

			body := requestBodyMaybe(r)

			handle, ok := handler.Raw().(runtime.Closure)
			if !ok {
				panic(fmt.Errorf("Handler for '%s' is not a function", path))
			}

			var methodEnumType *checker.Enum
			params := handle.GetParams()
			if len(params) > 0 {
				if structType, ok := params[0].Type.(*checker.StructDef); ok {
					if field, ok := structType.Fields["method"]; ok {
						if enumType, ok := field.(*checker.Enum); ok {
							methodEnumType = enumType
						}
					}
				}
			}
			if methodEnumType == nil {
				if mod, ok := checker.FindEmbeddedModule("ard/http"); ok {
					sym := mod.Get("Method")
					if sym.Type != nil {
						if enumType, ok := sym.Type.(*checker.Enum); ok {
							methodEnumType = enumType
						}
					}
				}
			}
			if methodEnumType == nil {
				panic(fmt.Errorf("Handler for '%s' missing http::Request method type", path))
			}

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
				"timeout": runtime.MakeNone(checker.Int),
				"raw":     runtime.MakeNone(checker.Dynamic).ToSome(r),
			}

			handlerMapType := args[1].Type().(*checker.Map)
			requestType := handlerMapType.Value().(*checker.FunctionDef).Parameters[0].Type
			request := runtime.MakeStruct(requestType, requestMap)

			// Create a default Response object with 200 status and empty body
			responseType := handlerMapType.Value().(*checker.FunctionDef).Parameters[1].Type
			responseMap := map[string]*runtime.Object{
				"status":  runtime.MakeInt(200),
				"headers": runtime.Make(make(map[string]*runtime.Object), checker.MakeMap(checker.Str, checker.Str)),
				"body":    runtime.MakeStr(""),
			}
			response := runtime.MakeStruct(responseType, responseMap)

			// Call the Ard handler function with request and mutable response
			// Create a copy of the closure with a new VM for isolation to prevent race conditions
			handle.EvalIsolated(request, response)

			// Get the final response values
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
