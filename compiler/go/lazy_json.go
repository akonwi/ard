package ardgo

import (
	"encoding/json"
	"strconv"
	"unsafe"
)

type jsonDynamic string

type jsonObjectDynamic struct {
	raw    jsonDynamic
	keys   [3]string
	values [3]jsonDynamic
	count  int
}

func validateLazyJSON(s string) bool {
	return json.Valid(unsafeStringBytes(s)) && !hasDuplicateJSONNames(s)
}

func unsafeStringBytes(value string) []byte {
	if len(value) == 0 {
		return nil
	}
	return unsafe.Slice(unsafe.StringData(value), len(value))
}

func skipJSONSpaces(s string, idx int) int {
	for idx < len(s) {
		switch s[idx] {
		case ' ', '\n', '\r', '\t':
			idx++
		default:
			return idx
		}
	}
	return idx
}

func skipJSONString(s string, idx int) int {
	if idx >= len(s) || s[idx] != '"' {
		return -1
	}
	idx++
	for idx < len(s) {
		switch s[idx] {
		case '\\':
			idx += 2
		case '"':
			return idx + 1
		default:
			idx++
		}
	}
	return -1
}

func skipJSONValue(s string, idx int) int {
	idx = skipJSONSpaces(s, idx)
	if idx >= len(s) {
		return -1
	}
	if s[idx] == '"' {
		return skipJSONString(s, idx)
	}
	if s[idx] == '{' || s[idx] == '[' {
		open, close := s[idx], byte('}')
		if open == '[' {
			close = ']'
		}
		depth := 1
		idx++
		for idx < len(s) {
			switch s[idx] {
			case '"':
				idx = skipJSONString(s, idx)
				if idx < 0 {
					return -1
				}
				continue
			case open:
				depth++
			case close:
				depth--
				if depth == 0 {
					return idx + 1
				}
			}
			idx++
		}
		return -1
	}
	for idx < len(s) {
		switch s[idx] {
		case ',', '}', ']', ' ', '\n', '\r', '\t':
			return idx
		default:
			idx++
		}
	}
	return idx
}

func parseLazyJSONObjectSmall(s string) (jsonObjectDynamic, bool) {
	idx := skipJSONSpaces(s, 0)
	if idx >= len(s) || s[idx] != '{' {
		return jsonObjectDynamic{}, false
	}
	idx++
	out := jsonObjectDynamic{raw: jsonDynamic(s)}
	for {
		idx = skipJSONSpaces(s, idx)
		if idx >= len(s) {
			return jsonObjectDynamic{}, false
		}
		if s[idx] == '}' {
			return out, true
		}
		if out.count >= len(out.keys) {
			return jsonObjectDynamic{}, false
		}
		keyStart := idx
		keyEnd := skipJSONString(s, idx)
		if keyEnd < 0 {
			return jsonObjectDynamic{}, false
		}
		key, ok := jsonStringContent(s[keyStart:keyEnd])
		if !ok {
			return jsonObjectDynamic{}, false
		}
		idx = skipJSONSpaces(s, keyEnd)
		if idx >= len(s) || s[idx] != ':' {
			return jsonObjectDynamic{}, false
		}
		valueStart := skipJSONSpaces(s, idx+1)
		valueEnd := skipJSONValue(s, valueStart)
		if valueEnd < 0 {
			return jsonObjectDynamic{}, false
		}
		out.keys[out.count] = key
		out.values[out.count] = jsonDynamic(s[valueStart:valueEnd])
		out.count++
		idx = skipJSONSpaces(s, valueEnd)
		if idx < len(s) && s[idx] == ',' {
			idx++
		}
	}
}

func extractLazyJSONField(s, name string) (string, bool, bool) {
	idx := skipJSONSpaces(s, 0)
	if idx >= len(s) || s[idx] != '{' {
		return "", false, false
	}
	idx++
	for {
		idx = skipJSONSpaces(s, idx)
		if idx >= len(s) {
			return "", false, false
		}
		if s[idx] == '}' {
			return "", false, true
		}
		keyStart := idx
		keyEnd := skipJSONString(s, idx)
		if keyEnd < 0 {
			return "", false, false
		}
		key, ok := jsonStringContent(s[keyStart:keyEnd])
		if !ok {
			return "", false, false
		}
		idx = skipJSONSpaces(s, keyEnd)
		if idx >= len(s) || s[idx] != ':' {
			return "", false, false
		}
		valueStart := skipJSONSpaces(s, idx+1)
		valueEnd := skipJSONValue(s, valueStart)
		if valueEnd < 0 {
			return "", false, false
		}
		if key == name {
			return s[valueStart:valueEnd], true, true
		}
		idx = skipJSONSpaces(s, valueEnd)
		if idx < len(s) && s[idx] == ',' {
			idx++
		}
	}
}

func decodeLazyJSONIntList(s string) ([]int, bool) {
	idx := skipJSONSpaces(s, 0)
	if idx >= len(s) || s[idx] != '[' {
		return nil, false
	}
	idx++
	out := make([]int, 0, 12)
	for {
		idx = skipJSONSpaces(s, idx)
		if idx >= len(s) {
			return nil, false
		}
		if s[idx] == ']' {
			return out, true
		}
		intValue, end, ok := parseLazyJSONIntAt(s, idx)
		if !ok {
			return nil, false
		}
		out = append(out, intValue)
		idx = skipJSONSpaces(s, end)
		if idx < len(s) && s[idx] == ',' {
			idx++
		}
	}
}

func decodeLazyJSONStringIntMap(s string) (map[string]int, bool) {
	idx := skipJSONSpaces(s, 0)
	if idx >= len(s) || s[idx] != '{' {
		return nil, false
	}
	idx++
	out := make(map[string]int)
	for {
		idx = skipJSONSpaces(s, idx)
		if idx >= len(s) {
			return nil, false
		}
		if s[idx] == '}' {
			return out, true
		}
		keyStart := idx
		keyEnd := skipJSONString(s, idx)
		if keyEnd < 0 {
			return nil, false
		}
		key, ok := jsonStringContent(s[keyStart:keyEnd])
		if !ok {
			return nil, false
		}
		idx = skipJSONSpaces(s, keyEnd)
		if idx >= len(s) || s[idx] != ':' {
			return nil, false
		}
		valueStart := skipJSONSpaces(s, idx+1)
		intValue, valueEnd, ok := parseLazyJSONIntAt(s, valueStart)
		if !ok {
			return nil, false
		}
		out[key] = intValue
		idx = skipJSONSpaces(s, valueEnd)
		if idx < len(s) && s[idx] == ',' {
			idx++
		}
	}
}

func parseLazyJSONIntAt(s string, idx int) (int, int, bool) {
	if idx >= len(s) {
		return 0, idx, false
	}
	start := idx
	if s[idx] >= '0' && s[idx] <= '9' {
		value := 0
		for idx < len(s) {
			ch := s[idx]
			if ch < '0' || ch > '9' {
				if isJSONValueEnd(ch) {
					return value, idx, true
				}
				end := skipJSONValue(s, start)
				if end < 0 {
					return 0, idx, false
				}
				parsed, ok := parseLazyJSONFloatInt(s[start:end])
				return parsed, end, ok
			}
			value = value*10 + int(ch-'0')
			idx++
		}
		return value, idx, true
	}
	if s[idx] == '-' {
		idx++
		if idx >= len(s) {
			return 0, idx, false
		}
		value := 0
		for idx < len(s) {
			ch := s[idx]
			if ch < '0' || ch > '9' {
				if isJSONValueEnd(ch) {
					return -value, idx, true
				}
				end := skipJSONValue(s, start)
				if end < 0 {
					return 0, idx, false
				}
				parsed, ok := parseLazyJSONFloatInt(s[start:end])
				return parsed, end, ok
			}
			value = value*10 + int(ch-'0')
			idx++
		}
		return -value, idx, true
	}
	end := skipJSONValue(s, start)
	if end < 0 {
		return 0, idx, false
	}
	parsed, ok := parseLazyJSONFloatInt(s[start:end])
	return parsed, end, ok
}

func isJSONValueEnd(ch byte) bool {
	switch ch {
	case ',', '}', ']', ' ', '\n', '\r', '\t':
		return true
	default:
		return false
	}
}

func parseLazyJSONIntNumber(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	if raw[0] >= '0' && raw[0] <= '9' {
		value := 0
		for idx := 0; idx < len(raw); idx++ {
			ch := raw[idx]
			if ch < '0' || ch > '9' {
				return parseLazyJSONFloatInt(raw)
			}
			value = value*10 + int(ch-'0')
		}
		return value, true
	}
	if raw[0] == '-' {
		if len(raw) == 1 {
			return 0, false
		}
		value := 0
		for idx := 1; idx < len(raw); idx++ {
			ch := raw[idx]
			if ch < '0' || ch > '9' {
				return parseLazyJSONFloatInt(raw)
			}
			value = value*10 + int(ch-'0')
		}
		return -value, true
	}
	return parseLazyJSONFloatInt(raw)
}

func parseLazyJSONFloatInt(raw string) (int, bool) {
	floatValue, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	intValue := int(floatValue)
	return intValue, floatValue == float64(intValue)
}

func jsonStringContent(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	for idx := 1; idx < len(raw)-1; idx++ {
		if raw[idx] == '\\' {
			value, err := strconv.Unquote(raw)
			return value, err == nil
		}
	}
	return raw[1 : len(raw)-1], true
}

func hasDuplicateJSONNames(s string) bool {
	idx := skipJSONSpaces(s, 0)
	if !scanJSONValueNames(s, &idx) {
		return true
	}
	idx = skipJSONSpaces(s, idx)
	return idx != len(s)
}

func scanJSONValueNames(s string, idx *int) bool {
	*idx = skipJSONSpaces(s, *idx)
	if *idx >= len(s) {
		return false
	}
	switch s[*idx] {
	case '{':
		return scanJSONObjectNames(s, idx)
	case '[':
		return scanJSONArrayNames(s, idx)
	case '"':
		end := skipJSONString(s, *idx)
		if end < 0 {
			return false
		}
		*idx = end
		return true
	default:
		end := skipJSONValue(s, *idx)
		if end < 0 {
			return false
		}
		*idx = end
		return true
	}
}

func scanJSONObjectNames(s string, idx *int) bool {
	var seenSmall [16]string
	seenCount := 0
	var seenMap map[string]struct{}
	*idx = *idx + 1
	for {
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return false
		}
		if s[*idx] == '}' {
			*idx = *idx + 1
			return true
		}
		keyStart := *idx
		keyEnd := skipJSONString(s, keyStart)
		if keyEnd < 0 {
			return false
		}
		key, ok := jsonStringContent(s[keyStart:keyEnd])
		if !ok {
			return false
		}
		if seenMap != nil {
			if _, ok := seenMap[key]; ok {
				return false
			}
			seenMap[key] = struct{}{}
		} else {
			for i := 0; i < seenCount; i++ {
				if seenSmall[i] == key {
					return false
				}
			}
			if seenCount < len(seenSmall) {
				seenSmall[seenCount] = key
				seenCount++
			} else {
				seenMap = make(map[string]struct{}, len(seenSmall)*2)
				for _, existing := range seenSmall {
					seenMap[existing] = struct{}{}
				}
				seenMap[key] = struct{}{}
			}
		}
		*idx = skipJSONSpaces(s, keyEnd)
		if *idx >= len(s) || s[*idx] != ':' {
			return false
		}
		*idx = *idx + 1
		if !scanJSONValueNames(s, idx) {
			return false
		}
		*idx = skipJSONSpaces(s, *idx)
		if *idx < len(s) && s[*idx] == ',' {
			*idx = *idx + 1
		}
	}
}

func scanJSONArrayNames(s string, idx *int) bool {
	*idx = *idx + 1
	for {
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return false
		}
		if s[*idx] == ']' {
			*idx = *idx + 1
			return true
		}
		if !scanJSONValueNames(s, idx) {
			return false
		}
		*idx = skipJSONSpaces(s, *idx)
		if *idx < len(s) && s[*idx] == ',' {
			*idx = *idx + 1
		}
	}
}
