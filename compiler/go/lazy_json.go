package ardgo

import "strconv"

type jsonDynamic string

type jsonObjectDynamic struct {
	raw    jsonDynamic
	keys   [3]string
	values [3]any
	count  int
}

func validateLazyJSON(s string) bool {
	_, ok := validateLazyJSONWithObject(s)
	return ok
}

func validateLazyJSONWithObject(s string) (*jsonObjectDynamic, bool) {
	idx := skipJSONSpaces(s, 0)
	var object *jsonObjectDynamic
	var ok bool
	if idx < len(s) && s[idx] == '{' {
		object, ok = scanJSONObjectNamesWithCache(s, &idx)
	} else {
		ok = scanJSONValueNames(s, &idx)
	}
	if !ok {
		return nil, false
	}
	idx = skipJSONSpaces(s, idx)
	if idx != len(s) {
		return nil, false
	}
	return object, true
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
			idx++
			if idx >= len(s) {
				return -1
			}
			switch s[idx] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				idx++
			case 'u':
				if idx+4 >= len(s) {
					return -1
				}
				for i := idx + 1; i <= idx+4; i++ {
					if !isJSONHex(s[i]) {
						return -1
					}
				}
				idx += 5
			default:
				return -1
			}
		case '"':
			return idx + 1
		default:
			if s[idx] < 0x20 {
				return -1
			}
			idx++
		}
	}
	return -1
}

func isJSONHex(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

func scanJSONStringContent(s string, idx int) (string, int, bool) {
	if idx >= len(s) || s[idx] != '"' {
		return "", idx, false
	}
	start := idx
	hasEscape := false
	idx++
	for idx < len(s) {
		switch s[idx] {
		case '\\':
			hasEscape = true
			idx++
			if idx >= len(s) {
				return "", idx, false
			}
			switch s[idx] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				idx++
			case 'u':
				if idx+4 >= len(s) {
					return "", idx, false
				}
				for i := idx + 1; i <= idx+4; i++ {
					if !isJSONHex(s[i]) {
						return "", idx, false
					}
				}
				idx += 5
			default:
				return "", idx, false
			}
		case '"':
			end := idx + 1
			if hasEscape {
				value, err := strconv.Unquote(s[start:end])
				return value, end, err == nil
			}
			return s[start+1 : idx], end, true
		default:
			if s[idx] < 0x20 {
				return "", idx, false
			}
			idx++
		}
	}
	return "", idx, false
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
		key, keyEnd, ok := scanJSONStringContent(s, idx)
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

func scanJSONIntArray(s string, idx *int) ([]int, bool) {
	*idx = skipJSONSpaces(s, *idx)
	if *idx >= len(s) || s[*idx] != '[' {
		return nil, false
	}
	*idx = *idx + 1
	out := make([]int, 0, 12)
	for {
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == ']' {
			*idx = *idx + 1
			return out, true
		}
		intValue, end, ok := parseLazyJSONIntAt(s, *idx)
		if !ok {
			return nil, false
		}
		out = append(out, intValue)
		*idx = skipJSONSpaces(s, end)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == ',' {
			*idx = skipJSONSpaces(s, *idx+1)
			if *idx >= len(s) || s[*idx] == ']' {
				return nil, false
			}
			continue
		}
		if s[*idx] != ']' {
			return nil, false
		}
	}
}

func scanJSONStringIntMap(s string, idx *int) (map[string]int, bool) {
	*idx = skipJSONSpaces(s, *idx)
	if *idx >= len(s) || s[*idx] != '{' {
		return nil, false
	}
	var seenSmall [8]string
	seenCount := 0
	var seenMap map[string]struct{}
	*idx = *idx + 1
	out := make(map[string]int, 4)
	for {
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == '}' {
			*idx = *idx + 1
			return out, true
		}
		key, keyEnd, ok := scanJSONStringContent(s, *idx)
		if !ok {
			return nil, false
		}
		if seenMap != nil {
			if _, ok := seenMap[key]; ok {
				return nil, false
			}
			seenMap[key] = struct{}{}
		} else {
			for i := 0; i < seenCount; i++ {
				if seenSmall[i] == key {
					return nil, false
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
			return nil, false
		}
		*idx = skipJSONSpaces(s, *idx+1)
		intValue, valueEnd, ok := parseLazyJSONIntAt(s, *idx)
		if !ok {
			return nil, false
		}
		out[key] = intValue
		*idx = skipJSONSpaces(s, valueEnd)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == ',' {
			*idx = skipJSONSpaces(s, *idx+1)
			if *idx >= len(s) || s[*idx] == '}' {
				return nil, false
			}
			continue
		}
		if s[*idx] != '}' {
			return nil, false
		}
	}
}

func decodeLazyJSONStringIntMap(s string) (map[string]int, bool) {
	idx := skipJSONSpaces(s, 0)
	if idx >= len(s) || s[idx] != '{' {
		return nil, false
	}
	idx++
	out := make(map[string]int, 4)
	for {
		idx = skipJSONSpaces(s, idx)
		if idx >= len(s) {
			return nil, false
		}
		if s[idx] == '}' {
			return out, true
		}
		key, keyEnd, ok := scanJSONStringContent(s, idx)
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

func skipJSONAtom(s string, idx int) int {
	if idx >= len(s) {
		return -1
	}
	switch s[idx] {
	case 't':
		if idx+4 <= len(s) && s[idx:idx+4] == "true" {
			return idx + 4
		}
		return -1
	case 'f':
		if idx+5 <= len(s) && s[idx:idx+5] == "false" {
			return idx + 5
		}
		return -1
	case 'n':
		if idx+4 <= len(s) && s[idx:idx+4] == "null" {
			return idx + 4
		}
		return -1
	}
	if s[idx] == '-' {
		idx++
		if idx >= len(s) {
			return -1
		}
	}
	if s[idx] == '0' {
		idx++
	} else if s[idx] >= '1' && s[idx] <= '9' {
		idx++
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			idx++
		}
	} else {
		return -1
	}
	if idx < len(s) && s[idx] == '.' {
		idx++
		if idx >= len(s) || s[idx] < '0' || s[idx] > '9' {
			return -1
		}
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			idx++
		}
	}
	if idx < len(s) && (s[idx] == 'e' || s[idx] == 'E') {
		idx++
		if idx < len(s) && (s[idx] == '+' || s[idx] == '-') {
			idx++
		}
		if idx >= len(s) || s[idx] < '0' || s[idx] > '9' {
			return -1
		}
		for idx < len(s) && s[idx] >= '0' && s[idx] <= '9' {
			idx++
		}
	}
	return idx
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
		end := skipJSONAtom(s, *idx)
		if end < 0 {
			return false
		}
		*idx = end
		return true
	}
}

func scanJSONObjectNamesWithCache(s string, idx *int) (*jsonObjectDynamic, bool) {
	var seenSmall [16]string
	seenCount := 0
	var seenMap map[string]struct{}
	object := &jsonObjectDynamic{raw: jsonDynamic(s)}
	cacheable := true
	*idx = *idx + 1
	for {
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == '}' {
			*idx = *idx + 1
			if cacheable {
				return object, true
			}
			return nil, true
		}
		key, keyEnd, ok := scanJSONStringContent(s, *idx)
		if !ok {
			return nil, false
		}
		if seenMap != nil {
			if _, ok := seenMap[key]; ok {
				return nil, false
			}
			seenMap[key] = struct{}{}
		} else {
			for i := 0; i < seenCount; i++ {
				if seenSmall[i] == key {
					return nil, false
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
			return nil, false
		}
		valueStart := skipJSONSpaces(s, *idx+1)
		*idx = valueStart
		var cached any
		if cacheable {
			switch s[valueStart] {
			case '[':
				if value, ok := scanJSONIntArray(s, idx); ok {
					cached = value
				} else {
					*idx = valueStart
				}
			case '{':
				if value, ok := scanJSONStringIntMap(s, idx); ok {
					cached = value
				} else {
					*idx = valueStart
				}
			}
		}
		if cached == nil {
			if !scanJSONValueNames(s, idx) {
				return nil, false
			}
		}
		if cacheable {
			if object.count < len(object.keys) {
				object.keys[object.count] = key
				if cached != nil {
					object.values[object.count] = cached
				} else {
					object.values[object.count] = jsonDynamic(s[valueStart:*idx])
				}
				object.count++
			} else {
				cacheable = false
			}
		}
		*idx = skipJSONSpaces(s, *idx)
		if *idx >= len(s) {
			return nil, false
		}
		if s[*idx] == ',' {
			*idx = skipJSONSpaces(s, *idx+1)
			if *idx >= len(s) || s[*idx] == '}' {
				return nil, false
			}
			continue
		}
		if s[*idx] != '}' {
			return nil, false
		}
	}
}

func scanJSONObjectNames(s string, idx *int) bool {
	var seenSmall [8]string
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
		key, keyEnd, ok := scanJSONStringContent(s, *idx)
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
		if *idx >= len(s) {
			return false
		}
		if s[*idx] == ',' {
			*idx = skipJSONSpaces(s, *idx+1)
			if *idx >= len(s) || s[*idx] == '}' {
				return false
			}
			continue
		}
		if s[*idx] != '}' {
			return false
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
		if *idx >= len(s) {
			return false
		}
		if s[*idx] == ',' {
			*idx = skipJSONSpaces(s, *idx+1)
			if *idx >= len(s) || s[*idx] == ']' {
				return false
			}
			continue
		}
		if s[*idx] != ']' {
			return false
		}
	}
}
