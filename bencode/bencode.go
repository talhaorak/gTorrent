package bencode

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
)

type DataType = int

// Types enum
const (
	INVALID DataType = iota
	STRING
	INTEGER
	LIST
	DICT
)

type Data struct {
	Type  DataType
	Value interface{}
}

// func NewData(t int) Data {
// 	data := Data{
// 		Type: t,
// 	}
// 	return data
// }

func NewData(v any) *Data {
	d := Data{}
	d.SetValueAndType(v)
	return &d
}

func (d *Data) SetValue(v any) {
	d.Value = v
}

func (d *Data) SetValueAndType(val any) {
	switch v := val.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		d.Type = INTEGER
		d.Value = int64(reflect.ValueOf(v).Int())
	case []byte:
		d.Type = STRING
		d.Value = v
	case string:
		d.Type = STRING
		d.Value = []byte(v)
	case []interface{}:
		list := make([]*Data, len(v))
		for i, elem := range v {
			list[i] = NewData(elem)
		}
		d.Type = LIST
		d.Value = list
	case []*Data:
		d.Type = LIST
		d.Value = v
	case map[string]interface{}:
		dict := make(map[string]*Data, len(v))
		for key, value := range v {
			dict[key] = NewData(value)
		}
		d.Type = DICT
		d.Value = dict
	case map[string]*Data:
		d.Type = DICT
		d.Value = v
	default:
		d.Type = INVALID
	}
}

func (d Data) AsString() string {
	return string(d.Value.([]byte))
}

func (d Data) AsBytes() []byte {
	return d.Value.([]byte)
}

func (d Data) AsInt() int64 {
	return d.Value.(int64)
}

func (d Data) AsList() []*Data {
	return d.Value.([]*Data)
}

func (d Data) AsDict() map[string]*Data {
	return d.Value.(map[string]*Data)
}

func (d Data) String() string {
	typeStr := ""
	switch d.Type {
	case STRING:
		typeStr = "STRING"
	case INTEGER:
		typeStr = "NUMBER"
	case LIST:
		typeStr = "LIST"
	case DICT:
		typeStr = "DICT"
	default:
		return "INVALID"
	}
	valStr := fmt.Sprintf("%v", d.Value)
	switch d.Type {
	case LIST:
		valStr = "["
		for i, elem := range d.AsList() {
			valStr += elem.String()
			if i < len(d.AsList())-1 {
				valStr += ", "
			}
		}
		valStr += "]"
	case DICT:
		valStr = "{"
		for key, elem := range d.AsDict() {
			valStr += fmt.Sprintf("%s: %s", key, elem.String())
		}
		valStr += "}"
	}

	return fmt.Sprintf("{Type: %s, Value: %s}", typeStr, valStr)
}

func (d Data) ToBytes() []byte {
	return Encode(&d)
}

func (d Data) ToJSON() string {
	// convert to object first, then convert to JSON
	var val interface{}
	switch d.Type {
	case STRING:
		val = d.AsString()
	case INTEGER:
		val = d.AsInt()
	case LIST:
		val = []interface{}{}
		for _, elem := range d.AsList() {
			val = append(val.([]interface{}), elem.ToJSON())
		}
	case DICT:
		val = map[string]interface{}{}
		for key, elem := range d.AsDict() {
			val.(map[string]interface{})[key] = elem.ToJSON()
		}
	}

	jsonVal, err := json.MarshalIndent(val, "", "  ")
	if err != nil {
		return ""
	}
	return string(jsonVal)
}

// Takes a byte slice and returns a Data struct, count of read bytes and an error
func Decode(content []byte) (*Data, int, error) {
	if len(content) == 0 {
		return nil, 0, nil
	}
	firstByte := content[0]
	switch firstByte {
	case 'i': // Integer
		// read until 'e'
		for i := 1; i < len(content); i++ {
			if content[i] == 'e' {
				intStr := string(content[1:i])

				fVal, err := strconv.ParseInt(intStr, 10, 64)
				if err != nil {
					return nil, i + 1, nil
				}
				return NewData(fVal), i + 1, nil
			}
		}
		return NewData(nil), len(content), fmt.Errorf("invalid integer")
	case 'l': // List
		list := make([]*Data, 0)
		for i := 1; i < len(content); i++ {
			if content[i] == 'e' {
				return NewData(list), i + 1, nil
			}
			elem, count, err := Decode(content[i:])
			if err != nil {
				return NewData(list), count, err
			}
			list = append(list, elem)
			i += count - 1
		}
		return NewData(list), len(content), fmt.Errorf("invalid list")

	case 'd': // Dictionary
		dict := make(map[string]*Data)
		for i := 1; i < len(content); i++ {
			if content[i] == 'e' {
				return NewData(dict), i + 1, nil
			}
			key, count, err := Decode(content[i:])
			if err != nil {
				return NewData(dict), count, err
			}
			if key.Type != STRING {
				return NewData(dict), count, fmt.Errorf("invalid dictionary key")
			}
			// if key.AsString() == "pieces" {
			// 	fmt.Printf("pieces\n")
			// }
			i += count
			val, count, err := Decode(content[i:])
			if err != nil {
				return NewData(dict), count, err
			}
			i += count - 1

			dict[key.AsString()] = val
		}
		return NewData(dict), len(content), fmt.Errorf("invalid dictionary")
	default: // String
		// read until ':'
		for i := 0; i < len(content); i++ {
			if content[i] == ':' {
				strLen, err := strconv.Atoi(string(content[:i]))
				if err != nil {
					return nil, i + 1, fmt.Errorf("invalid string length")
				}
				strVal := content[i+1 : i+1+strLen]

				return NewData(strVal), i + 1 + strLen, nil
			}
		}
		return nil, len(content), fmt.Errorf("invalid string")

	}
}

func Encode(data *Data) []byte {
	switch data.Type {
	case STRING:
		str := data.AsString()
		return []byte(fmt.Sprintf("%d:%s", len(str), str))
	case INTEGER:
		return []byte(fmt.Sprintf("i%de", data.Value))
	case LIST:
		list := data.AsList()
		encoded := []byte("l")
		for _, elem := range list {
			encoded = append(encoded, Encode(elem)...)
		}
		encoded = append(encoded, 'e')
		return encoded
	case DICT:
		dict := data.AsDict()
		encoded := []byte("d")
		// sort keys in lexical order
		keys := make([]string, 0, len(dict))
		for key := range dict {
			keys = append(keys, key)
		}
		slices.Sort(keys)

		for _, key := range keys {
			encoded = append(encoded, Encode(NewData(key))...)
			encoded = append(encoded, Encode(dict[key])...)
		}
		encoded = append(encoded, 'e')
		return encoded
	default:
		return []byte{}
	}
}
