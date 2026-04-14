package ardgo

import "strconv"

type ToString interface {
	ToStr() string
}

type Encodable interface {
	ToDyn() any
}

type stringToString string

type intToString int

type floatToString float64

type boolToString bool

type stringEncodable string

type intEncodable int

type floatEncodable float64

type boolEncodable bool

func (v stringToString) ToStr() string { return string(v) }
func (v intToString) ToStr() string    { return strconv.Itoa(int(v)) }
func (v floatToString) ToStr() string  { return strconv.FormatFloat(float64(v), 'f', 2, 64) }
func (v boolToString) ToStr() string   { return strconv.FormatBool(bool(v)) }

func (v stringEncodable) ToDyn() any { return string(v) }
func (v intEncodable) ToDyn() any    { return int(v) }
func (v floatEncodable) ToDyn() any  { return float64(v) }
func (v boolEncodable) ToDyn() any   { return bool(v) }

func AsToString(value any) ToString {
	switch v := value.(type) {
	case ToString:
		return v
	case string:
		return stringToString(v)
	case int:
		return intToString(v)
	case float64:
		return floatToString(v)
	case bool:
		return boolToString(v)
	default:
		panic("value does not implement ToString")
	}
}

func AsEncodable(value any) Encodable {
	switch v := value.(type) {
	case Encodable:
		return v
	case string:
		return stringEncodable(v)
	case int:
		return intEncodable(v)
	case float64:
		return floatEncodable(v)
	case bool:
		return boolEncodable(v)
	default:
		panic("value does not implement Encodable")
	}
}
