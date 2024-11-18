package structmap

import (
	"fmt"
	"reflect"
	"strings"
)

type MappedStruct map[string]interface{}

const (
	OPTION_IGNORE    = "-"
	OPTION_OMITEMPTY = "omitempty"
	OPTION_DIVE      = "dive"
	OPTION_WILDCARD  = "wildcard"
	OPTION_DOTTED    = "dotted"

	method_results_total = 2
)

const (
	FLAG_IGNORE = 1 << iota
	FLAG_OMITEMPTY
	FLAG_DIVE
	FLAG_WILDCARD
	FLAG_DOTTED
)

// StructToMap maps a struct by its tag.
//
// Key can be specified by tag, LIKE `json:"tag"`, or `map:"tag"`. Whatever you want.
// Specify the tag in the second parameter. Options are:
//   - `omitempty` to omit empty fields
//   - `dive` to dive into the struct and map the fields
//   - `wildcard` to add `%` to the string value
//   - `dotted` to add a dot `.` to the key
//
// Notes:
// Dive options will map the child's struct fields directly to the parent map.
// Example:
//
//	type A struct {
//		AA string `json:"aa"`
//		B B `json:"b,dive"`
//	}
//	type B struct {
//		C string `json:"c"`
//	}
//
//	Result:
//	map[aa:string c:string]
//
// While dive options will map the child's struct fields directly to the parent map.
// Dotted options will add a dot `.` to the child's struct tag, followed by it's fields tag. Example:
//
//	type A struct {
//		AA string `json:"aa"`
//		B B `json:"b,dotted"`
//	}
//	type B struct {
//		C string `json:"c"`
//	}
//
//	Result:
//	map[aa:string b.c:string]
func StructToMap(data interface{}, tag string, method string) (MappedStruct, error) {
	result := make(MappedStruct)
	reflectedValue := reflect.ValueOf(data)

	if reflectedValue.Kind() == reflect.Pointer {
		if reflectedValue.IsNil() {
			return nil, fmt.Errorf("%s is a nil pointer", reflectedValue.Kind().String())
		}
		reflectedValue = reflectedValue.Elem()
	}
	if reflectedValue.Kind() != reflect.Struct {
		return nil, fmt.Errorf("data is not a struct but %s", reflectedValue.Kind().String())
	}

	reflectType := reflectedValue.Type()
	for i := 0; i < reflectType.NumField(); i++ {
		fieldType := reflectType.Field(i)

		// ignore unexported field
		if fieldType.PkgPath != "" {
			continue
		}

		tagVal, flag := tagsReader(fieldType, tag)
		if flag&FLAG_IGNORE != 0 {
			continue
		}

		fieldValue := reflectedValue.Field(i)
		if flag&FLAG_OMITEMPTY != 0 && fieldValue.IsZero() {
			continue
		}
		if fieldValue.Kind() == reflect.Pointer {
			if fieldValue.IsNil() {
				continue
			}
			fieldValue = fieldValue.Elem()
		}

		key, value, err := assignValueWithMethod(fieldValue, method)
		if err != nil {
			return nil, err
		}
		if key != "" {
			result[key] = value
			continue
		}

		switch fieldValue.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.Chan:
			result[tagVal] = fieldValue
		case reflect.Struct:
			deepRes, deepErr := StructToMap(fieldValue.Interface(), tag, method)
			if deepErr != nil {
				return nil, deepErr
			}
			if flag&FLAG_DIVE != 0 {
				for k, v := range deepRes {
					result[k] = v
				}
			} else if flag&FLAG_DOTTED != 0 {
				for k, v := range deepRes {
					result[tagVal+"."+k] = v
				}
			} else {
				result[tagVal] = deepRes
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			result[tagVal] = fieldValue.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			result[tagVal] = fieldValue.Uint()
		case reflect.Float32, reflect.Float64:
			result[tagVal] = fieldValue.Float()
		case reflect.String:
			if flag&FLAG_WILDCARD != 0 {
				result[tagVal] = "%" + fieldValue.String() + "%"
			} else {
				result[tagVal] = fieldValue.String()
			}
		case reflect.Bool:
			result[tagVal] = fieldValue.Bool()
		case reflect.Complex64, reflect.Complex128:
			result[tagVal] = fieldValue.Complex()
		case reflect.Interface:
			result[tagVal] = fieldValue.Interface()
		}
	}

	return result, nil
}

// tagsReader read tag with format `json:"name,omitempty"` or `json:"-"`
// For now, only supports above format
func tagsReader(structField reflect.StructField, tag string) (string, int) {
	var (
		flag int    = 0
		fTag string = ""
	)

	tagValue, ok := structField.Tag.Lookup(tag)
	if !ok {
		// if tag not found, ignore the field.
		// returns empty string and ignore flag
		flag |= FLAG_IGNORE
		return fTag, flag
	}

	opts := strings.Split(tagValue, ",")
	fTag = opts[0]

	for i := 0; i < len(opts); i++ {
		switch opts[i] {
		case OPTION_IGNORE:
			flag |= FLAG_IGNORE
		case OPTION_OMITEMPTY:
			flag |= FLAG_OMITEMPTY
		case OPTION_DIVE:
			flag |= FLAG_DIVE
		case OPTION_WILDCARD:
			flag |= FLAG_WILDCARD
		case OPTION_DOTTED:
			flag |= FLAG_DOTTED
		}
	}

	return fTag, flag
}

func assignValueWithMethod(reflectedValue reflect.Value, method string) (key string, value interface{}, err error) {
	if method == "" {
		return "", nil, nil
	}

	_, ok := reflectedValue.Type().MethodByName(method)
	if !ok {
		return "", nil, nil
	}

	key, value, err = callFunc(reflectedValue, method)
	if err != nil {
		return "", nil, err
	}

	return key, value, nil
}

// callFunc calls the method and returns the key and value.
// The method should have 2 outputs: (string,interface{}).
func callFunc(reflectedValue reflect.Value, method string) (key string, value interface{}, err error) {
	methodResults := reflectedValue.MethodByName(method).Call([]reflect.Value{})
	if len(methodResults) != method_results_total {
		return "", nil, fmt.Errorf("wrong method %s, should have 2 output: (string,interface{})", method)
	}
	if methodResults[0].Kind() != reflect.String {
		return "", nil, fmt.Errorf("wrong method %s, first output should be string", method)
	}

	key = methodResults[0].String()
	value = methodResults[1].Interface()

	return key, value, nil
}
