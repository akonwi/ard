package ffi

import (
	"bufio"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/akonwi/ard/runtime"
)

var HostFunctions = Host{
	Base64Decode:        Base64Decode,
	Base64DecodeURL:     Base64DecodeURL,
	Base64Encode:        Base64Encode,
	Base64EncodeURL:     Base64EncodeURL,
	BoolToDynamic:       BoolToDynamic,
	DecodeBool:          DecodeBool,
	DecodeFloat:         DecodeFloat,
	DecodeInt:           DecodeInt,
	DecodeString:        DecodeString,
	DynamicToList:       DynamicToList,
	DynamicToMap:        DynamicToMap,
	ExtractField:        ExtractField,
	FloatFloor:          FloatFloor,
	FloatFromInt:        FloatFromInt,
	FloatFromStr:        FloatFromStr,
	FloatToDynamic:      FloatToDynamic,
	GetPathValue:        GetPathValue,
	GetQueryParam:       GetQueryParam,
	GetReqPath:          GetReqPath,
	HTTPDo:              HTTPDo,
	HTTPResponseBody:    HTTPResponseBody,
	HTTPResponseClose:   HTTPResponseClose,
	HTTPResponseHeaders: HTTPResponseHeaders,
	HTTPResponseStatus:  HTTPResponseStatus,
	HTTPServe:           HTTPServe,
	HexDecode:           HexDecode,
	HexEncode:           HexEncode,
	IntFromStr:          IntFromStr,
	IntToDynamic:        IntToDynamic,
	IsNil:               IsNil,
	JsonEncode:          JsonEncode,
	JsonToDynamic:       JsonToDynamic,
	ListToDynamic:       ListToDynamic,
	MapToDynamic:        MapToDynamic,
	OsArgs:              OsArgs,
	Print:               Print,
	ReadLine:            ReadLine,
	Sleep:               Sleep,
	StrToDynamic:        StrToDynamic,
	VoidToDynamic:       VoidToDynamic,
}.Functions()

func OsArgs() []string {
	return runtime.CurrentOSArgs()
}

func Print(str string) {
	fmt.Println(str)
}

var (
	stdinReaderMu sync.Mutex
	stdinReader   *bufio.Reader
	stdinSource   *os.File
)

func ReadLine() (string, error) {
	stdinReaderMu.Lock()
	defer stdinReaderMu.Unlock()

	if stdinReader == nil || stdinSource != os.Stdin {
		stdinSource = os.Stdin
		stdinReader = bufio.NewReader(os.Stdin)
	}

	line, err := stdinReader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func Sleep(ns int) {
	time.Sleep(time.Duration(ns))
}

func Base64Encode(input string, noPad Maybe[bool]) string {
	if noPad.Some && noPad.Value {
		return base64.RawStdEncoding.EncodeToString([]byte(input))
	}
	return base64.StdEncoding.EncodeToString([]byte(input))
}

func Base64Decode(input string, noPad Maybe[bool]) (string, error) {
	enc := base64.StdEncoding
	if noPad.Some && noPad.Value {
		enc = base64.RawStdEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func Base64EncodeURL(input string, noPad Maybe[bool]) string {
	if noPad.Some && noPad.Value {
		return base64.RawURLEncoding.EncodeToString([]byte(input))
	}
	return base64.URLEncoding.EncodeToString([]byte(input))
}

func Base64DecodeURL(input string, noPad Maybe[bool]) (string, error) {
	enc := base64.URLEncoding
	if noPad.Some && noPad.Value {
		enc = base64.RawURLEncoding
	}
	decoded, err := enc.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func HexEncode(input string) string {
	return hex.EncodeToString([]byte(input))
}

func HexDecode(input string) (string, error) {
	decoded, err := hex.DecodeString(input)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func FloatFromStr(str string) Maybe[float64] {
	value, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return None[float64]()
	}
	return Some(value)
}

func FloatFromInt(value int) float64 {
	return float64(value)
}

func FloatFloor(value float64) float64 {
	return math.Floor(value)
}

func IntFromStr(str string) Maybe[int] {
	value, err := strconv.Atoi(str)
	if err != nil {
		return None[int]()
	}
	return Some(value)
}

func StrToDynamic(value string) any {
	return value
}

func IntToDynamic(value int) any {
	return value
}

func FloatToDynamic(value float64) any {
	return value
}

func BoolToDynamic(value bool) any {
	return value
}

func VoidToDynamic() any {
	return nil
}

func ListToDynamic(list []any) any {
	return list
}

func MapToDynamic(from map[string]any) any {
	return from
}

func IsNil(data any) bool {
	return data == nil
}

func JsonToDynamic(input string) (any, error) {
	var out any
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func JsonEncode(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func DecodeString(data any) Result[string, Error] {
	if value, ok := data.(string); ok {
		return Ok[string, Error](value)
	}
	return Err[string](decodeError("String", dynamicKind(data)))
}

func DecodeInt(data any) Result[int, Error] {
	switch value := data.(type) {
	case int:
		return Ok[int, Error](value)
	case int64:
		return Ok[int, Error](int(value))
	case float64:
		if math.Trunc(value) == value {
			return Ok[int, Error](int(value))
		}
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return Ok[int, Error](int(parsed))
		}
	}
	return Err[int](decodeError("Int", dynamicKind(data)))
}

func DecodeFloat(data any) Result[float64, Error] {
	switch value := data.(type) {
	case float64:
		return Ok[float64, Error](value)
	case int:
		return Ok[float64, Error](float64(value))
	case int64:
		return Ok[float64, Error](float64(value))
	case json.Number:
		if parsed, err := value.Float64(); err == nil {
			return Ok[float64, Error](parsed)
		}
	}
	return Err[float64](decodeError("Float", dynamicKind(data)))
}

func DecodeBool(data any) Result[bool, Error] {
	if value, ok := data.(bool); ok {
		return Ok[bool, Error](value)
	}
	return Err[bool](decodeError("Bool", dynamicKind(data)))
}

func DynamicToList(data any) ([]any, error) {
	if data == nil {
		return nil, fmt.Errorf("Void")
	}
	if values, ok := data.([]any); ok {
		return values, nil
	}
	return nil, fmt.Errorf("%s", dynamicKind(data))
}

func DynamicToMap(data any) (map[any]any, error) {
	if data == nil {
		return nil, fmt.Errorf("Void")
	}
	if values, ok := data.(map[any]any); ok {
		return values, nil
	}
	if values, ok := data.(map[string]any); ok {
		out := make(map[any]any, len(values))
		for key, value := range values {
			out[key] = value
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s", dynamicKind(data))
}

func ExtractField(data any, name string) (any, error) {
	if values, ok := data.(map[string]any); ok {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("Missing field %q", name)
		}
		return value, nil
	}
	if values, ok := data.(map[any]any); ok {
		value, ok := values[name]
		if !ok {
			return nil, fmt.Errorf("Missing field %q", name)
		}
		return value, nil
	}
	return nil, fmt.Errorf("%s", dynamicKind(data))
}

func HTTPDo(method string, url string, body any, headers map[string]string, timeout Maybe[int]) (RawResponse, error) {
	var bodyReader io.Reader = strings.NewReader("")
	if body != nil {
		switch value := body.(type) {
		case string:
			bodyReader = strings.NewReader(value)
		case []byte:
			bodyReader = strings.NewReader(string(value))
		default:
			encoded, err := json.Marshal(value)
			if err != nil {
				return RawResponse{}, err
			}
			bodyReader = strings.NewReader(string(encoded))
		}
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return RawResponse{}, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{}
	if timeout.Some {
		client.Timeout = time.Duration(timeout.Value) * time.Second
	}
	resp, err := client.Do(req)
	if err != nil {
		return RawResponse{}, err
	}
	return RawResponse{Handle: resp}, nil
}

func HTTPResponseStatus(resp RawResponse) int {
	if response, ok := resp.Handle.(*http.Response); ok {
		return response.StatusCode
	}
	return 0
}

func HTTPResponseHeaders(resp RawResponse) map[string]string {
	response, ok := resp.Handle.(*http.Response)
	if !ok {
		return map[string]string{}
	}
	headers := make(map[string]string, len(response.Header))
	for key, values := range response.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

func HTTPResponseBody(resp RawResponse) (string, error) {
	response, ok := resp.Handle.(*http.Response)
	if !ok {
		return "", fmt.Errorf("invalid HTTP response handle")
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func HTTPResponseClose(resp RawResponse) {
	if response, ok := resp.Handle.(*http.Response); ok && response.Body != nil {
		_ = response.Body.Close()
	}
}

func GetReqPath(req RawRequest) string {
	if request, ok := req.Handle.(*http.Request); ok && request.URL != nil {
		return request.URL.Path
	}
	return ""
}

func GetPathValue(req RawRequest, name string) string {
	if request, ok := req.Handle.(*http.Request); ok {
		return request.PathValue(name)
	}
	return ""
}

func GetQueryParam(req RawRequest, name string) string {
	if request, ok := req.Handle.(*http.Request); ok && request.URL != nil {
		return request.URL.Query().Get(name)
	}
	return ""
}

func HTTPServe(port int, handlers map[string]Callback2[Request, *Response, struct{}]) error {
	mux := http.NewServeMux()
	for path, handler := range handlers {
		path := path
		handler := handler
		mux.HandleFunc(convertHTTPPattern(path), func(writer http.ResponseWriter, req *http.Request) {
			ardReq := Request{
				Method:  methodFromHTTPRequest(req.Method),
				Url:     req.URL.String(),
				Headers: requestHeaders(req),
				Body:    requestBody(req),
				Raw:     Some(RawRequest{Handle: req}),
			}
			ardRes := Response{
				Status:  200,
				Headers: map[string]string{},
			}
			if _, err := handler.Call(ardReq, &ardRes); err != nil {
				http.Error(writer, err.Error(), http.StatusInternalServerError)
				return
			}
			for key, value := range ardRes.Headers {
				writer.Header().Set(key, value)
			}
			status := ardRes.Status
			if status == 0 {
				status = 200
			}
			writer.WriteHeader(status)
			_, _ = io.WriteString(writer, ardRes.Body)
		})
	}
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

func convertHTTPPattern(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func methodFromHTTPRequest(method string) Method {
	switch method {
	case "GET":
		return Method(0)
	case "POST":
		return Method(1)
	case "PUT":
		return Method(2)
	case "DELETE":
		return Method(3)
	case "PATCH":
		return Method(4)
	case "OPTIONS":
		return Method(5)
	default:
		return Method(0)
	}
}

func requestHeaders(req *http.Request) map[string]string {
	headers := make(map[string]string, len(req.Header))
	for key, values := range req.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}
	return headers
}

func requestBody(req *http.Request) Maybe[any] {
	if req.Body == nil {
		return None[any]()
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil || len(body) == 0 {
		return None[any]()
	}
	return Some[any](string(body))
}

func decodeError(expected string, found string) Error {
	return Error{Expected: expected, Found: found}
}

func dynamicKind(data any) string {
	switch data.(type) {
	case nil:
		return "Void"
	case string:
		return "String"
	case bool:
		return "Bool"
	case int, int64:
		return "Int"
	case float64, json.Number:
		return "Number"
	case []any:
		return "List"
	case map[string]any, map[any]any:
		return "Map"
	default:
		return fmt.Sprintf("%T", data)
	}
}
