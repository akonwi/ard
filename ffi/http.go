package ffi

import (
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
func HTTP_Send(args []*runtime.Object, _ checker.Type) *runtime.Object {
	method := args[0].AsString()
	url := args[1].AsString()
	body := strings.NewReader(args[2].AsString())
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

	return runtime.MakeOk(runtime.MakeStruct(checker.HttpResponseDef, respMap))
}
