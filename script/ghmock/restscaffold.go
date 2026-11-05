package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

const maxSchemaDepth = 32

// scaffold builds a success response for a REST request from GitHub's
// description. It starts from GitHub's example. About one in five examples
// lag behind the description, so it fills their gaps with placeholders. It
// returns the response and says where the body came from.
func (idx *restIndex) scaffold(method, path string) (response, string, error) {
	route := idx.match(method, path)
	if route == nil {
		return response{}, "", fmt.Errorf("GitHub's REST description has no path matching %s", path)
	}
	op := route.item.GetOperation(method)
	if op == nil {
		return response{}, "", fmt.Errorf("GitHub documents no %s for %s", method, route.template)
	}
	code, ref := successStatus(op.Responses)
	if ref == nil || ref.Value == nil {
		return response{}, "", fmt.Errorf("GitHub documents no success status for %s %s", method, route.template)
	}
	status := 200
	if n, err := strconv.Atoi(code); err == nil {
		status = n
	}
	var resp response
	statusText := ""
	if status != 200 {
		resp.Status = status
		statusText = fmt.Sprintf(", status %d", status)
	}

	media := jsonMedia(ref.Value.Content)
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		if len(ref.Value.Content) > 0 {
			return response{}, "", fmt.Errorf("GitHub's response for status %s is %s; provide it with ghmock set --raw-file or --zip-dir",
				code, strings.Join(slices.Sorted(maps.Keys(ref.Value.Content)), ", "))
		}
		return resp, fmt.Sprintf(" with no body%s", statusText), nil
	}

	if example, ok := pickExample(media); ok {
		f := &restFiller{}
		filled := f.fill(media.Schema, example, "", 0)
		f.filled += encodeContent(filled)
		body, err := json.Marshal(filled)
		if err == nil {
			if problems, _ := idx.check(method, path, &response{Status: status, Body: body}); len(problems) == 0 {
				resp.Body = body
				detail := " from GitHub's example"
				if f.filled > 0 {
					detail += fmt.Sprintf(", with %s added or fixed", plural(f.filled, "value"))
				}
				return resp, detail + statusText, nil
			}
		}
	}

	f := &restFiller{}
	body, err := json.Marshal(f.generate(media.Schema, "", 0))
	if err != nil {
		return response{}, "", err
	}
	if problems, _ := idx.check(method, path, &response{Status: status, Body: body}); len(problems) > 0 {
		return response{}, "", fmt.Errorf("couldn't build a body that matches GitHub's description:\n%sSet one with ghmock set --off-schema",
			indentProblems(problems, "  ", maxShownProblems))
	}
	resp.Body = body
	return resp, " with placeholder values" + statusText, nil
}

// successStatus picks the first documented 2xx status.
func successStatus(responses *openapi3.Responses) (string, *openapi3.ResponseRef) {
	for _, code := range documentedStatuses(responses) {
		if strings.HasPrefix(code, "2") {
			return code, responses.Value(code)
		}
	}
	return "", nil
}

// pickExample returns a copy of the example for a response body, preferring
// the one named default.
func pickExample(media *openapi3.MediaType) (any, bool) {
	value := media.Example
	if value == nil && len(media.Examples) > 0 {
		name := "default"
		if media.Examples[name] == nil {
			name = slices.Sorted(maps.Keys(media.Examples))[0]
		}
		if ex := media.Examples[name]; ex != nil && ex.Value != nil {
			value = ex.Value.Value
		}
	}
	if value == nil {
		return nil, false
	}
	// The filler changes values in place, and the example belongs to the
	// loaded description.
	return cloneJSON(value), true
}

// restFiller makes a JSON value match a REST schema. It adds required
// properties that are missing and replaces values of the wrong type, and
// counts what it changed. Generated values for properties that can be null
// are null, because in REST responses null usually means "none", such as an
// issue's closed_at.
type restFiller struct {
	filled int
}

func (f *restFiller) fill(ref *openapi3.SchemaRef, v any, name string, depth int) any {
	if ref == nil || ref.Value == nil || depth > maxSchemaDepth {
		return v
	}
	s := ref.Value
	if v == nil {
		if allowsNull(s) {
			return nil
		}
		f.filled++
		return f.generate(ref, name, depth)
	}

	for _, sub := range s.AllOf {
		v = f.fill(sub, v, name, depth+1)
	}
	if alts := alternatives(s); len(alts) > 0 {
		if matchesSchema(ref, v) {
			return v
		}
		if d := s.Discriminator; d != nil {
			if obj, ok := v.(map[string]any); ok {
				for _, alt := range alts {
					candidate, _ := cloneJSON(obj).(map[string]any)
					trial := &restFiller{}
					if _, has := candidate[d.PropertyName]; !has {
						candidate[d.PropertyName] = discriminatorValue(d, alt)
						trial.filled++
					}
					filled := trial.fill(alt, candidate, name, depth+1)
					if matchesSchema(ref, filled) {
						f.filled += trial.filled
						return filled
					}
				}
			}
		}
		for _, alt := range alts {
			if alt.Value != nil && typeMatches(alt.Value, v) {
				return f.fill(alt, v, name, depth+1)
			}
		}
		f.filled++
		return f.generate(alts[0], name, depth+1)
	}

	switch {
	case isObjectSchema(s):
		obj, ok := v.(map[string]any)
		if !ok {
			f.filled++
			return f.generate(ref, name, depth)
		}
		for _, key := range slices.Sorted(maps.Keys(s.Properties)) {
			if val, ok := obj[key]; ok {
				obj[key] = f.fill(s.Properties[key], val, key, depth+1)
			}
		}
		for _, key := range s.Required {
			if _, ok := obj[key]; !ok {
				f.filled++
				obj[key] = f.generate(s.Properties[key], key, depth+1)
			}
		}
		for key, val := range obj {
			if _, known := s.Properties[key]; known {
				continue
			}
			switch extra := s.AdditionalProperties; {
			case extra.Schema != nil:
				obj[key] = f.fill(extra.Schema, val, key, depth+1)
			case extra.Has != nil && !*extra.Has:
				f.filled++
				delete(obj, key)
			}
		}
		return obj
	case s.Type.Is("array"):
		list, ok := v.([]any)
		if !ok {
			f.filled++
			return f.generate(ref, name, depth)
		}
		for i := range list {
			list[i] = f.fill(s.Items, list[i], name, depth+1)
		}
		return list
	case len(s.Type.Slice()) > 0:
		if !typeMatches(s, v) || (len(s.Enum) > 0 && !slices.ContainsFunc(s.Enum, func(e any) bool { return jsonText(e) == jsonText(v) })) {
			f.filled++
			return f.generate(ref, name, depth)
		}
	}
	return v
}

// generate makes a placeholder value for a schema. Objects get only their
// required properties. A required property with no schema is null: GitHub's
// description requires archived_at on pull request labels without defining
// it, and GitHub sends null.
func (f *restFiller) generate(ref *openapi3.SchemaRef, name string, depth int) any {
	if ref == nil || ref.Value == nil || depth > maxSchemaDepth {
		return nil
	}
	s := ref.Value
	if depth > 0 && allowsNull(s) {
		return nil
	}
	if len(s.Enum) > 0 {
		values := make([]string, 0, len(s.Enum))
		for _, e := range s.Enum {
			if str, ok := e.(string); ok {
				values = append(values, str)
			}
		}
		if len(values) == len(s.Enum) {
			return enumPlaceholder(values)
		}
		return s.Enum[0]
	}
	if len(s.AllOf) > 0 {
		var v any = map[string]any{}
		for _, sub := range s.AllOf {
			v = f.fill(sub, v, name, depth+1)
		}
		if isObjectSchema(s) {
			v = f.fill(&openapi3.SchemaRef{Value: withoutAllOf(s)}, v, name, depth+1)
		}
		return v
	}
	if alts := alternatives(s); len(alts) > 0 {
		for _, alt := range alts {
			if alt.Value == nil || alt.Value.Type.Is("null") {
				continue
			}
			v := f.generate(alt, name, depth+1)
			if obj, ok := v.(map[string]any); ok && s.Discriminator != nil {
				obj[s.Discriminator.PropertyName] = discriminatorValue(s.Discriminator, alt)
			}
			return v
		}
		return nil
	}

	switch {
	case isObjectSchema(s):
		obj := map[string]any{}
		for _, key := range s.Required {
			obj[key] = f.generate(s.Properties[key], key, depth+1)
		}
		return obj
	case s.Type.Is("array"):
		list := []any{}
		for range s.MinItems {
			list = append(list, f.generate(s.Items, name, depth+1))
		}
		return list
	case s.Type.Is("string"):
		return stringPlaceholder(s.Format, name)
	case s.Type.Is("integer"), s.Type.Is("number"):
		n := 1.0
		if s.Min != nil && n < *s.Min {
			n = math.Ceil(*s.Min)
		}
		if s.Max != nil && n > *s.Max {
			n = math.Floor(*s.Max)
		}
		return n
	case s.Type.Is("boolean"):
		return false
	}
	return map[string]any{}
}

// encodeContent fixes file contents in GitHub's examples, which say they're
// base64 but hold plain text such as "encoded content ...". gh decodes them,
// so gh repo view fails on the README. It returns how many it fixed.
func encodeContent(v any) int {
	fixed := 0
	switch v := v.(type) {
	case map[string]any:
		if content, ok := v["content"].(string); ok && v["encoding"] == "base64" {
			if _, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", "")); err != nil {
				v["content"] = base64.StdEncoding.EncodeToString([]byte(content))
				fixed++
			}
		}
		for _, child := range v {
			fixed += encodeContent(child)
		}
	case []any:
		for _, child := range v {
			fixed += encodeContent(child)
		}
	}
	return fixed
}

func stringPlaceholder(format, name string) string {
	switch format {
	case "date-time":
		return placeholderTime
	case "date":
		return placeholderDate
	case "uri", "url":
		return placeholderURL
	case "email":
		return "monalisa@example.com"
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	}
	if name == "" {
		return "string"
	}
	return name
}

// discriminatorValue is the value that selects alt, such as "private" for
// GitHub's private-user schema.
func discriminatorValue(d *openapi3.Discriminator, alt *openapi3.SchemaRef) string {
	for _, key := range slices.Sorted(maps.Keys(d.Mapping)) {
		if d.Mapping[key].Ref == alt.Ref {
			return key
		}
	}
	if alt.Value != nil {
		if p := alt.Value.Properties[d.PropertyName]; p != nil && p.Value != nil && len(p.Value.Enum) > 0 {
			if v, ok := p.Value.Enum[0].(string); ok {
				return v
			}
		}
	}
	return refName(alt)
}

func cloneJSON(v any) any {
	data, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var copied any
	if err := json.Unmarshal(data, &copied); err != nil {
		return v
	}
	return copied
}

func alternatives(s *openapi3.Schema) openapi3.SchemaRefs {
	return append(append(openapi3.SchemaRefs{}, s.OneOf...), s.AnyOf...)
}

func withoutAllOf(s *openapi3.Schema) *openapi3.Schema {
	copied := *s
	copied.AllOf = nil
	return &copied
}

func allowsNull(s *openapi3.Schema) bool {
	return s.Nullable || s.Type.Includes("null")
}

func isObjectSchema(s *openapi3.Schema) bool {
	return s.Type.Is("object") || (len(s.Type.Slice()) == 0 && len(s.Properties) > 0)
}

// typeMatches reports whether a decoded JSON value has one of a schema's types.
func typeMatches(s *openapi3.Schema, v any) bool {
	types := s.Type.Slice()
	if len(types) == 0 {
		return !isObjectSchema(s) || isMap(v)
	}
	for _, t := range types {
		switch t {
		case "object":
			if isMap(v) {
				return true
			}
		case "array":
			if _, ok := v.([]any); ok {
				return true
			}
		case "string":
			if _, ok := v.(string); ok {
				return true
			}
		case "integer":
			if n, ok := v.(float64); ok && n == math.Trunc(n) {
				return true
			}
		case "number":
			if _, ok := v.(float64); ok {
				return true
			}
		case "boolean":
			if _, ok := v.(bool); ok {
				return true
			}
		case "null":
			if v == nil {
				return true
			}
		}
	}
	return false
}

func isMap(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}

// matchesSchema reports whether a value passes the same check as responses.
func matchesSchema(ref *openapi3.SchemaRef, v any) bool {
	if ref == nil || ref.Value == nil {
		return true
	}
	return len(restProblems(ref.Value.VisitJSON(v, openapi3.MultiErrors(), openapi3.VisitAsResponse()))) == 0
}
