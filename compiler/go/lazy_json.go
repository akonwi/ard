package ardgo

import (
	"encoding/json"
	"strconv"
	"strings"
	"unsafe"
)

type jsonDynamic string

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
		end := skipJSONValue(s, idx)
		if end < 0 {
			return nil, false
		}
		intValue, ok := parseLazyJSONIntNumber(s[idx:end])
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
		valueEnd := skipJSONValue(s, valueStart)
		if valueEnd < 0 {
			return nil, false
		}
		intValue, ok := parseLazyJSONIntNumber(s[valueStart:valueEnd])
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

func parseLazyJSONIntNumber(raw string) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	sign := 1
	idx := 0
	if raw[0] == '-' {
		sign = -1
		idx = 1
		if idx == len(raw) {
			return 0, false
		}
	}
	value := 0
	for ; idx < len(raw); idx++ {
		ch := raw[idx]
		if ch < '0' || ch > '9' {
			floatValue, err := strconv.ParseFloat(raw, 64)
			if err != nil {
				return 0, false
			}
			intValue := int(floatValue)
			return intValue, floatValue == float64(intValue)
		}
		value = value*10 + int(ch-'0')
	}
	return sign * value, true
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
