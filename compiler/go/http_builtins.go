package ardgo

import (
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
)

func builtinHTTPMethodTag(method string) int {
	switch method {
	case http.MethodGet:
		return 0
	case http.MethodPost:
		return 1
	case http.MethodPut:
		return 2
	case http.MethodDelete:
		return 3
	case http.MethodPatch:
		return 4
	case http.MethodOptions:
		return 5
	default:
		return 0
	}
}

func builtinConvertHTTPPattern(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func builtinSetField(target reflect.Value, name string, value reflect.Value) error {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return nil
	}
	if !value.IsValid() {
		field.Set(reflect.Zero(field.Type()))
		return nil
	}
	if value.Type().AssignableTo(field.Type()) {
		field.Set(value)
		return nil
	}
	if value.Type().ConvertibleTo(field.Type()) {
		field.Set(value.Convert(field.Type()))
		return nil
	}
	return fmt.Errorf("cannot assign %s to field %s of type %s", value.Type(), name, field.Type())
}

func builtinBuildHTTPRequestArg(targetType reflect.Type, r *http.Request) (reflect.Value, error) {
	passPointer := targetType.Kind() == reflect.Pointer
	if passPointer {
		targetType = targetType.Elem()
	}
	if targetType.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("http handler request param must be struct, got %s", targetType)
	}

	requestValue := reflect.New(targetType).Elem()
	headers := make(map[string]string)
	for k, values := range r.Header {
		if len(values) > 0 {
			headers[k] = values[0]
		}
	}

	body := None[any]()
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil {
			return reflect.Value{}, err
		}
		if len(bodyBytes) > 0 {
			body = Some[any](string(bodyBytes))
		}
	}

	methodField := requestValue.FieldByName("Method")
	if methodField.IsValid() && methodField.CanSet() {
		methodValue := reflect.New(methodField.Type()).Elem()
		tagField := methodValue.FieldByName("Tag")
		if tagField.IsValid() && tagField.CanSet() {
			tagField.SetInt(int64(builtinHTTPMethodTag(r.Method)))
		}
		methodField.Set(methodValue)
	}
	if err := builtinSetField(requestValue, "Url", reflect.ValueOf(r.URL.String())); err != nil {
		return reflect.Value{}, err
	}
	if err := builtinSetField(requestValue, "Headers", reflect.ValueOf(headers)); err != nil {
		return reflect.Value{}, err
	}
	if err := builtinSetField(requestValue, "Body", reflect.ValueOf(body)); err != nil {
		return reflect.Value{}, err
	}
	if err := builtinSetField(requestValue, "Timeout", reflect.ValueOf(None[int]())); err != nil {
		return reflect.Value{}, err
	}
	if err := builtinSetField(requestValue, "Raw", reflect.ValueOf(Some[any](r))); err != nil {
		return reflect.Value{}, err
	}

	if passPointer {
		ptr := reflect.New(targetType)
		ptr.Elem().Set(requestValue)
		return ptr, nil
	}
	return requestValue, nil
}

func builtinBuildHTTPResponseArg(targetType reflect.Type) (reflect.Value, reflect.Value, error) {
	if targetType.Kind() != reflect.Pointer {
		return reflect.Value{}, reflect.Value{}, fmt.Errorf("http handler response param must be mutable pointer, got %s", targetType)
	}
	responseType := targetType.Elem()
	if responseType.Kind() != reflect.Struct {
		return reflect.Value{}, reflect.Value{}, fmt.Errorf("http handler response param must point to struct, got %s", responseType)
	}

	responsePtr := reflect.New(responseType)
	responseValue := responsePtr.Elem()
	if err := builtinSetField(responseValue, "Status", reflect.ValueOf(200)); err != nil {
		return reflect.Value{}, reflect.Value{}, err
	}
	if err := builtinSetField(responseValue, "Headers", reflect.ValueOf(map[string]string{})); err != nil {
		return reflect.Value{}, reflect.Value{}, err
	}
	if err := builtinSetField(responseValue, "Body", reflect.ValueOf("")); err != nil {
		return reflect.Value{}, reflect.Value{}, err
	}
	return responsePtr, responseValue, nil
}

func builtinWriteHTTPResponse(w http.ResponseWriter, responseValue reflect.Value) error {
	status := 200
	if field := responseValue.FieldByName("Status"); field.IsValid() && field.Kind() == reflect.Int {
		status = int(field.Int())
	}
	if field := responseValue.FieldByName("Headers"); field.IsValid() && field.Kind() == reflect.Map {
		iter := field.MapRange()
		for iter.Next() {
			if iter.Key().Kind() != reflect.String || iter.Value().Kind() != reflect.String {
				continue
			}
			w.Header().Set(iter.Key().String(), iter.Value().String())
		}
	}
	body := ""
	if field := responseValue.FieldByName("Body"); field.IsValid() && field.Kind() == reflect.String {
		body = field.String()
	}
	w.WriteHeader(status)
	_, err := w.Write([]byte(body))
	return err
}

func builtinCallHTTPHandler(handler reflect.Value, w http.ResponseWriter, r *http.Request) error {
	handlerType := handler.Type()
	if handlerType.Kind() != reflect.Func {
		return fmt.Errorf("http handler is not a function: %s", handlerType)
	}
	if handlerType.NumIn() != 2 {
		return fmt.Errorf("http handler must take 2 args, got %d", handlerType.NumIn())
	}

	requestArg, err := builtinBuildHTTPRequestArg(handlerType.In(0), r)
	if err != nil {
		return err
	}
	responseArg, responseValue, err := builtinBuildHTTPResponseArg(handlerType.In(1))
	if err != nil {
		return err
	}

	handler.Call([]reflect.Value{requestArg, responseArg})
	return builtinWriteHTTPResponse(w, responseValue)
}

func builtinHTTPServe(port int, handlers any) Result[struct{}, string] {
	handlersValue := reflect.ValueOf(handlers)
	if !handlersValue.IsValid() || handlersValue.Kind() != reflect.Map {
		return Err[struct{}, string]("HTTP_Serve expects a handler map")
	}

	mux := http.NewServeMux()
	iter := handlersValue.MapRange()
	for iter.Next() {
		pathValue := iter.Key()
		handlerValue := iter.Value()
		if pathValue.Kind() != reflect.String {
			return Err[struct{}, string]("HTTP_Serve handler map keys must be strings")
		}
		pattern := builtinConvertHTTPPattern(pathValue.String())
		handler := handlerValue
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := builtinCallHTTPHandler(handler, w, r); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
		})
	}

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux); err != nil {
		return Err[struct{}, string](err.Error())
	}
	return Ok[struct{}, string](struct{}{})
}
