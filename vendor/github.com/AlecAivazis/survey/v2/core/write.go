package core

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// the tag used to denote the name of the question
const tagName = "survey"

// Settables allow for configuration when assigning answers
type Settable interface {
	WriteAnswer(field string, value interface{}) error
}

// OptionAnswer is the return type of Selects/MultiSelects that lets the appropriate information
// get copied to the user's struct
type OptionAnswer struct {
	Value string
	Index int
}

func OptionAnswerList(incoming []string) []OptionAnswer {
	list := []OptionAnswer{}
	for i, opt := range incoming {
		list = append(list, OptionAnswer{Value: opt, Index: i})
	}
	return list
}

func WriteAnswer(t interface{}, name string, v interface{}) (err error) {
	// if the field is a custom type
	if s, ok := t.(Settable); ok {
		// use the interface method
		return s.WriteAnswer(name, v)
	}

	// the target to write to
	target := reflect.ValueOf(t)
	// the value to write from
	value := reflect.ValueOf(v)

	// make sure we are writing to a pointer
	if target.Kind() != reflect.Ptr {
		return errors.New("you must pass a pointer as the target of a Write operation")
	}
	// the object "inside" of the target pointer
	elem := target.Elem()

	// handle the special types
	switch elem.Kind() {
	// if we are writing to a struct
	case reflect.Struct:
		// if we are writing to an option answer than we want to treat
		// it like a single thing and not a place to deposit answers
		if elem.Type().Name() == "OptionAnswer" {
			// copy the value over to the normal struct
			return copy(elem, value)
		}

		// get the name of the field that matches the string we  were given
		fieldIndex, err := findFieldIndex(elem, name)
		// if something went wrong
		if err != nil {
			// bubble up
			return err
		}
		field := elem.Field(fieldIndex)
		// handle references to the Settable interface aswell
		if s, ok := field.Interface().(Settable); ok {
			// use the interface method
			return s.WriteAnswer(name, v)
		}
		if field.CanAddr() {
			if s, ok := field.Addr().Interface().(Settable); ok {
				// use the interface method
				return s.WriteAnswer(name, v)
			}
		}

		// copy the value over to the normal struct
		return copy(field, value)
	case reflect.Map:
		mapType := reflect.TypeOf(t).Elem()
		if mapType.Key().Kind() != reflect.String || mapType.Elem().Kind() != reflect.Interface {
			return errors.New("answer maps must be of type map[string]interface")
		}
		mt := *t.(*map[string]interface{})
		mt[name] = value.Interface()
		return nil
	}
	// otherwise just copy the value to the target
	return copy(elem, value)
}

// BUG(AlecAivazis): the current implementation might cause weird conflicts if there are
// two fields with same name that only differ by casing.
func findFieldIndex(s reflect.Value, name string) (int, error) {
	// the type of the value
	sType := s.Type()

	// first look for matching tags so we can overwrite matching field names
	for i := 0; i < sType.NumField(); i++ {
		// the field we are current scanning
		field := sType.Field(i)

		// the value of the survey tag
		tag := field.Tag.Get(tagName)
		// if the tag matches the name we are looking for
		if tag != "" && tag == name {
			// then we found our index
			return i, nil
		}
	}

	// then look for matching names
	for i := 0; i < sType.NumField(); i++ {
		// the field we are current scanning
		field := sType.Field(i)

		// if the name of the field matches what we're looking for
		if strings.ToLower(field.Name) == strings.ToLower(name) {
			return i, nil
		}
	}

	// we didn't find the field
	return -1, fmt.Errorf("could not find field matching %v", name)
}

// isList returns true if the element is something we can Len()
func isList(v reflect.Value) bool {
	switch v.Type().Kind() {
	case reflect.Array, reflect.Slice:
		return true
	default:
		return false
	}
}

// Write takes a value and copies it to the target
func copy(t reflect.Value, v reflect.Value) (err error) {
	// if something ends up panicing we need to catch it in a deferred func
	defer func() {
		if r := recover(); r != nil {
			// if we paniced with an error
			if _, ok := r.(error); ok {
				// cast the result to an error object
				err = r.(error)
			} else if _, ok := r.(string); ok {
				// otherwise we could have paniced with a string so wrap it in an error
				err = errors.New(r.(string))
			}
		}
	}()

	// if we are copying from a string result to something else
	if v.Kind() == reflect.String && v.Type() != t.Type() {
		var castVal interface{}
		var casterr error
		vString := v.Interface().(string)

		switch t.Kind() {
		case reflect.Bool:
			castVal, casterr = strconv.ParseBool(vString)
		case reflect.Int:
			castVal, casterr = strconv.Atoi(vString)
		case reflect.Int8:
			var val64 int64
			val64, casterr = strconv.ParseInt(vString, 10, 8)
			if casterr == nil {
				castVal = int8(val64)
			}
		case reflect.Int16:
			var val64 int64
			val64, casterr = strconv.ParseInt(vString, 10, 16)
			if casterr == nil {
				castVal = int16(val64)
			}
		case reflect.Int32:
			var val64 int64
			val64, casterr = strconv.ParseInt(vString, 10, 32)
			if casterr == nil {
				castVal = int32(val64)
			}
		case reflect.Int64:
			if t.Type() == reflect.TypeOf(time.Duration(0)) {
				castVal, casterr = time.ParseDuration(vString)
			} else {
				castVal, casterr = strconv.ParseInt(vString, 10, 64)
			}
		case reflect.Uint:
			var val64 uint64
			val64, casterr = strconv.ParseUint(vString, 10, 8)
			if casterr == nil {
				castVal = uint(val64)
			}
		case reflect.Uint8:
			var val64 uint64
			val64, casterr = strconv.ParseUint(vString, 10, 8)
			if casterr == nil {
				castVal = uint8(val64)
			}
		case reflect.Uint16:
			var val64 uint64
			val64, casterr = strconv.ParseUint(vString, 10, 16)
			if casterr == nil {
				castVal = uint16(val64)
			}
		case reflect.Uint32:
			var val64 uint64
			val64, casterr = strconv.ParseUint(vString, 10, 32)
			if casterr == nil {
				castVal = uint32(val64)
			}
		case reflect.Uint64:
			castVal, casterr = strconv.ParseUint(vString, 10, 64)
		case reflect.Float32:
			var val64 float64
			val64, casterr = strconv.ParseFloat(vString, 32)
			if casterr == nil {
				castVal = float32(val64)
			}
		case reflect.Float64:
			castVal, casterr = strconv.ParseFloat(vString, 64)
		default:
			return fmt.Errorf("Unable to convert from string to type %s", t.Kind())
		}

		if casterr != nil {
			return casterr
		}

		t.Set(reflect.ValueOf(castVal))
		return
	}

	// if we are copying from an OptionAnswer to something
	if v.Type().Name() == "OptionAnswer" {
		// copying an option answer to a string
		if t.Kind() == reflect.String {
			// copies the Value field of the struct
			t.Set(reflect.ValueOf(v.FieldByName("Value").Interface()))
			return
		}

		// copying an option answer to an int
		if t.Kind() == reflect.Int {
			// copies the Index field of the struct
			t.Set(reflect.ValueOf(v.FieldByName("Index").Interface()))
			return
		}

		// copying an OptionAnswer to an OptionAnswer
		if t.Type().Name() == "OptionAnswer" {
			t.Set(v)
			return
		}

		// we're copying an option answer to an incorrect type
		return fmt.Errorf("Unable to convert from OptionAnswer to type %s", t.Kind())
	}

	// if we are copying from one slice or array to another
	if isList(v) && isList(t) {
		// loop over every item in the desired value
		for i := 0; i < v.Len(); i++ {
			// write to the target given its kind
			switch t.Kind() {
			// if its a slice
			case reflect.Slice:
				// an object of the correct type
				obj := reflect.Indirect(reflect.New(t.Type().Elem()))

				// write the appropriate value to the obj and catch any errors
				if err := copy(obj, v.Index(i)); err != nil {
					return err
				}

				// just append the value to the end
				t.Set(reflect.Append(t, obj))
			// otherwise it could be an array
			case reflect.Array:
				// set the index to the appropriate value
				copy(t.Slice(i, i+1).Index(0), v.Index(i))
			}
		}
	} else {
		// set the value to the target
		t.Set(v)
	}

	// we're done
	return
}
