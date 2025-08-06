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

func (m *HTTPModule) Handle(vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "serve":
		port := args[0].raw.(int)
		handlers := args[1].raw.(map[string]*object)

		_mux := http.NewServeMux()
		for path, handler := range handlers {
			_mux.HandleFunc(convertToGoPattern(path), func(w http.ResponseWriter, r *http.Request) {
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

				// Convert HTTP method string to Method enum
				var methodEnum *object
				switch r.Method {
				case "GET":
					methodEnum = &object{int8(0), checker.HttpMethodDef} // Get variant
				case "POST":
					methodEnum = &object{int8(1), checker.HttpMethodDef} // Post variant
				case "PUT":
					methodEnum = &object{int8(2), checker.HttpMethodDef} // Put variant
				case "DELETE":
					methodEnum = &object{int8(3), checker.HttpMethodDef} // Del variant
				case "PATCH":
					methodEnum = &object{int8(4), checker.HttpMethodDef} // Patch variant
				default:
					methodEnum = &object{int8(0), checker.HttpMethodDef} // Default to Get
				}

				requestMap := map[string]*object{
					"method":  methodEnum,
					"url":     {r.URL.String(), checker.Str},
					"headers": {headers, checker.MakeMap(checker.Str, checker.Str)},
					"body":    body,
					"path": {func() *object {
						return &object{r.URL.Path, checker.Str}
					}, checker.HttpRequestDef.Fields["path"]},
					"path_param": {func(args ...*object) *object {
						name := args[0].raw.(string)
						return &object{r.PathValue(name), checker.Str}
					}, checker.HttpRequestDef.Fields["path_param"]},
				}

				request := &object{requestMap, checker.HttpRequestDef}

				// Call the Ard handler function
				handle, ok := handler.raw.(*Closure)
				if !ok {
					panic(fmt.Errorf("Handler for '%s' is not a function", path))
				}
				response := handle.eval(request)

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

		// Convert Method enum to string
		methodEnum := requestMap["method"]
		var method string
		switch methodEnum.raw.(int8) {
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
		default:
			method = "GET"
		}
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

func (m *HTTPModule) HandleStatic(structName string, vm *VM, call *checker.FunctionCall, args []*object) *object {
	switch structName {
	case "Response":
		return m.handleResponseStatic(call, args)
	default:
		panic(fmt.Errorf("Unimplemented: http::%s::%s()", structName, call.Name))
	}
}

func (m *HTTPModule) handleResponseStatic(call *checker.FunctionCall, args []*object) *object {
	switch call.Name {
	case "new":
		// Response::new(status: Int, body: Str) -> Response
		if len(args) != 2 {
			panic(fmt.Errorf("Response::new expects 2 arguments, got %d", len(args)))
		}

		status := args[0].raw.(int)
		body := args[1].raw.(string)

		// Create response object structure
		respMap := map[string]*object{
			"status":  &object{status, checker.Int},
			"headers": &object{make(map[string]*object), checker.MakeMap(checker.Str, checker.Str)},
			"body":    &object{body, checker.Str},
		}

		return &object{respMap, checker.HttpResponseDef}
	default:
		panic(fmt.Errorf("Unimplemented: Response::%s()", call.Name))
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

			// Decode JSON response
			return mod.decodeJsonResponse(bodyStr, method.Type())
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
		if raw, ok := reqMap[method.Name]; ok {
			fn, ok := raw.raw.(func(...*object) *object)
			if !ok {
				panic(fmt.Errorf("HTTP Error: Request method is not a function: %s", method.Name))
			}
			return fn(args...)
		}
	}
	panic(fmt.Sprintf("Unsupported method on HTTP Response: %s", method.Name))
}
