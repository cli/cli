package api

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

const (
	keyStart     = '['
	keyEnd       = ']'
	keySeparator = '='
)

func parseFields(opts *ApiOptions) (map[string]any, error) {
	params := make(map[string]any)
	parseField := func(f string, isMagic bool) error {
		var valueIndex int
		var keystack []string
		keyStartAt := 0
	parseLoop:
		for i, r := range f {
			switch r {
			case keyStart:
				if keyStartAt == 0 {
					keystack = append(keystack, f[0:i])
				}
				keyStartAt = i + 1
			case keyEnd:
				keystack = append(keystack, f[keyStartAt:i])
			case keySeparator:
				if keyStartAt == 0 {
					keystack = append(keystack, f[0:i])
				}
				valueIndex = i + 1
				break parseLoop
			}
		}

		if len(keystack) == 0 {
			return fmt.Errorf("invalid key: %q", f)
		}

		key := f
		var value any = nil
		if valueIndex == 0 {
			if keystack[len(keystack)-1] != "" {
				return fmt.Errorf("field %q requires a value separated by an '=' sign", key)
			}
		} else {
			key = f[0 : valueIndex-1]
			value = f[valueIndex:]
		}

		if isMagic && value != nil {
			var err error
			value, err = magicFieldValue(value.(string), opts)
			if err != nil {
				return fmt.Errorf("error parsing %q value: %w", key, err)
			}
		}

		destMap := params
		isArray := false
		var subkey string
		for _, k := range keystack {
			if k == "" {
				isArray = true
				continue
			}
			if subkey != "" {
				var err error
				if isArray {
					destMap, err = addParamsSlice(destMap, subkey, k)
					isArray = false
				} else {
					destMap, err = addParamsMap(destMap, subkey)
				}
				if err != nil {
					return err
				}
			}
			subkey = k
		}

		if isArray {
			if value == nil {
				destMap[subkey] = []any{}
			} else {
				if v, exists := destMap[subkey]; exists {
					if existSlice, ok := v.([]any); ok {
						destMap[subkey] = append(existSlice, value)
					} else {
						return fmt.Errorf("expected array type under %q, got %T", subkey, v)
					}
				} else {
					destMap[subkey] = []any{value}
				}
			}
		} else {
			if _, exists := destMap[subkey]; exists {
				return fmt.Errorf("unexpected override existing field under %q", subkey)
			}
			destMap[subkey] = value
		}
		return nil
	}
	for _, f := range opts.RawFields {
		if err := parseField(f, false); err != nil {
			return params, err
		}
	}
	for _, f := range opts.MagicFields {
		if err := parseField(f, true); err != nil {
			return params, err
		}
	}
	return params, nil
}

func addParamsMap(m map[string]any, key string) (map[string]any, error) {
	if v, exists := m[key]; exists {
		if existMap, ok := v.(map[string]any); ok {
			return existMap, nil
		} else {
			return nil, fmt.Errorf("expected map type under %q, got %T", key, v)
		}
	}
	newMap := make(map[string]any)
	m[key] = newMap
	return newMap, nil
}

func addParamsSlice(m map[string]any, prevkey, newkey string) (map[string]any, error) {
	if v, exists := m[prevkey]; exists {
		if existSlice, ok := v.([]any); ok {
			if len(existSlice) > 0 {
				lastItem := existSlice[len(existSlice)-1]
				if lastMap, ok := lastItem.(map[string]any); ok {
					if _, keyExists := lastMap[newkey]; !keyExists {
						return lastMap, nil
					} else if reflect.TypeOf(lastMap[newkey]).Kind() == reflect.Slice {
						return lastMap, nil
					}
				}
			}
			newMap := make(map[string]any)
			m[prevkey] = append(existSlice, newMap)
			return newMap, nil
		} else {
			return nil, fmt.Errorf("expected array type under %q, got %T", prevkey, v)
		}
	}
	newMap := make(map[string]any)
	m[prevkey] = []any{newMap}
	return newMap, nil
}

func magicFieldValue(v string, opts *ApiOptions) (any, error) {
	if strings.HasPrefix(v, "@") {
		b, err := opts.IO.ReadUserFile(v[1:])
		if err != nil {
			return "", err
		}
		return string(b), nil
	}

	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}

	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return fillPlaceholders(v, opts)
	}
}
