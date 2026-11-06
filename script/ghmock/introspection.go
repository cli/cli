package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// introObject is an object that GraphQL introspection returns, such as a
// __Type or a __Field, answered from GitHub's schema.
type introObject interface {
	typeName() string
	resolve(s *gqlScaffolder, field string, args map[string]any) any
}

// introspect answers __type and __schema as GitHub would. gh asks which
// fields a type has to decide which queries to send. For example, gh issue
// develop stops if the LinkedBranch type has no fields, so placeholders here
// would change what gh does.
func (s *gqlScaffolder) introspect(f *ast.Field, sel ast.SelectionSet) any {
	return s.introValue(s.introspectionRoot(f), sel)
}

func (s *gqlScaffolder) introspectionRoot(f *ast.Field) introObject {
	switch f.Name {
	case "__schema":
		return introSchema{}
	case "__type":
		name, _ := s.arguments(f)["name"].(string)
		if s.schema.Types[name] == nil {
			return nil
		}
		return introType{ast.NamedType(name, nil)}
	}
	return nil
}

func (s *gqlScaffolder) mergeIntrospection(path string, existing, resolved any, sel ast.SelectionSet, record bool) any {
	switch source := resolved.(type) {
	case introObject:
		dst, ok := existing.(map[string]any)
		if !ok {
			return existing
		}
		var keys []string
		groups := map[string][]*ast.Field{}
		s.collect(sel, s.schema.Types[source.typeName()], &keys, groups)
		for _, key := range keys {
			field := groups[key][0]
			child := path + "." + key
			value := source.resolve(s, field.Name, s.arguments(field))
			if field.Name == "__typename" {
				value = source.typeName()
			}
			selections := mergeSelections(groups[key])
			if previous, found := dst[key]; found {
				dst[key] = s.mergeIntrospection(child, previous, value, selections, record)
			} else {
				dst[key] = s.introValue(value, selections)
				if record {
					s.added = append(s.added, child)
				}
			}
		}
	case []introObject:
		dst, ok := existing.([]any)
		if !ok {
			return existing
		}
		nameKeys := []string{"name"}
		if len(source) > 0 {
			var keys []string
			groups := map[string][]*ast.Field{}
			s.collect(sel, s.schema.Types[source[0].typeName()], &keys, groups)
			for _, key := range keys {
				if groups[key][0].Name == "name" && key != "name" {
					nameKeys = append(nameKeys, key)
				}
			}
		}
		for i, value := range dst {
			index := i
			if object, ok := value.(map[string]any); ok {
				for _, key := range nameKeys {
					name, ok := object[key].(string)
					if !ok {
						continue
					}
					index = -1
					for j, candidate := range source {
						if candidate.resolve(s, "name", nil) == name {
							index = j
							break
						}
					}
					break
				}
			}
			if index >= 0 && index < len(source) {
				dst[i] = s.mergeIntrospection(fmt.Sprintf("%s.%d", path, i), value, source[index], sel, record)
			}
		}
	}
	return existing
}

// introValue turns a resolved value into JSON, following the selection.
func (s *gqlScaffolder) introValue(v any, sel ast.SelectionSet) any {
	switch v := v.(type) {
	case introObject:
		var keys []string
		groups := map[string][]*ast.Field{}
		s.collect(sel, s.schema.Types[v.typeName()], &keys, groups)
		obj := map[string]any{}
		for _, key := range keys {
			f := groups[key][0]
			if f.Name == "__typename" {
				obj[key] = v.typeName()
				continue
			}
			obj[key] = s.introValue(v.resolve(s, f.Name, s.arguments(f)), mergeSelections(groups[key]))
		}
		return obj
	case []introObject:
		if v == nil {
			return nil
		}
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = s.introValue(item, sel)
		}
		return list
	default:
		return v
	}
}

type introSchema struct{}

func (introSchema) typeName() string { return "__Schema" }

func (introSchema) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	named := func(def *ast.Definition) any {
		if def == nil {
			return nil
		}
		return introType{ast.NamedType(def.Name, nil)}
	}
	switch field {
	case "description":
		return optional(s.schema.Description)
	case "types":
		names := make([]string, 0, len(s.schema.Types))
		for name := range s.schema.Types {
			names = append(names, name)
		}
		sort.Strings(names)
		list := []introObject{}
		for _, name := range names {
			list = append(list, introType{ast.NamedType(name, nil)})
		}
		return list
	case "queryType":
		return named(s.schema.Query)
	case "mutationType":
		return named(s.schema.Mutation)
	case "subscriptionType":
		return named(s.schema.Subscription)
	case "directives":
		names := make([]string, 0, len(s.schema.Directives))
		for name := range s.schema.Directives {
			names = append(names, name)
		}
		sort.Strings(names)
		list := []introObject{}
		for _, name := range names {
			list = append(list, introDirective{s.schema.Directives[name]})
		}
		return list
	}
	return nil
}

// introType is a type reference. A non-null or list type wraps another type.
type introType struct{ t *ast.Type }

func (introType) typeName() string { return "__Type" }

func (it introType) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	t := it.t
	if t.NonNull || t.Elem != nil {
		switch field {
		case "kind":
			if t.NonNull {
				return "NON_NULL"
			}
			return "LIST"
		case "ofType":
			if t.NonNull {
				inner := *t
				inner.NonNull = false
				return introType{&inner}
			}
			return introType{t.Elem}
		}
		return nil
	}

	def := s.schema.Types[t.NamedType]
	if def == nil {
		return nil
	}
	includeDeprecated, _ := args["includeDeprecated"].(bool)
	switch field {
	case "kind":
		return string(def.Kind)
	case "name":
		return def.Name
	case "description":
		return optional(def.Description)
	case "specifiedByURL":
		if d := def.Directives.ForName("specifiedBy"); d != nil {
			if a := d.Arguments.ForName("url"); a != nil {
				return a.Value.Raw
			}
		}
		return nil
	case "fields":
		if def.Kind != ast.Object && def.Kind != ast.Interface {
			return nil
		}
		list := []introObject{}
		for _, f := range def.Fields {
			if strings.HasPrefix(f.Name, "__") || (!includeDeprecated && deprecated(f.Directives)) {
				continue
			}
			list = append(list, introField{f})
		}
		return list
	case "interfaces":
		if def.Kind != ast.Object && def.Kind != ast.Interface {
			return nil
		}
		list := []introObject{}
		for _, name := range def.Interfaces {
			list = append(list, introType{ast.NamedType(name, nil)})
		}
		return list
	case "possibleTypes":
		if !def.IsAbstractType() {
			return nil
		}
		list := []introObject{}
		for _, p := range s.schema.GetPossibleTypes(def) {
			list = append(list, introType{ast.NamedType(p.Name, nil)})
		}
		return list
	case "enumValues":
		if def.Kind != ast.Enum {
			return nil
		}
		list := []introObject{}
		for _, v := range def.EnumValues {
			if includeDeprecated || !deprecated(v.Directives) {
				list = append(list, introEnumValue{v})
			}
		}
		return list
	case "inputFields":
		if def.Kind != ast.InputObject {
			return nil
		}
		list := []introObject{}
		for _, f := range def.Fields {
			if includeDeprecated || !deprecated(f.Directives) {
				list = append(list, introInputValue{f.Name, f.Description, f.Type, f.DefaultValue, f.Directives})
			}
		}
		return list
	case "isOneOf":
		if def.Kind != ast.InputObject {
			return nil
		}
		return def.Directives.ForName("oneOf") != nil
	}
	return nil
}

type introField struct{ f *ast.FieldDefinition }

func (introField) typeName() string { return "__Field" }

func (x introField) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	switch field {
	case "name":
		return x.f.Name
	case "description":
		return optional(x.f.Description)
	case "args":
		return introArgs(x.f.Arguments, args)
	case "type":
		return introType{x.f.Type}
	case "isDeprecated":
		return deprecated(x.f.Directives)
	case "deprecationReason":
		return deprecationReason(x.f.Directives)
	}
	return nil
}

type introInputValue struct {
	name         string
	description  string
	typ          *ast.Type
	defaultValue *ast.Value
	directives   ast.DirectiveList
}

func (introInputValue) typeName() string { return "__InputValue" }

func (x introInputValue) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	switch field {
	case "name":
		return x.name
	case "description":
		return optional(x.description)
	case "type":
		return introType{x.typ}
	case "defaultValue":
		if x.defaultValue == nil {
			return nil
		}
		return x.defaultValue.String()
	case "isDeprecated":
		return deprecated(x.directives)
	case "deprecationReason":
		return deprecationReason(x.directives)
	}
	return nil
}

func introArgs(defs ast.ArgumentDefinitionList, args map[string]any) []introObject {
	includeDeprecated, _ := args["includeDeprecated"].(bool)
	list := []introObject{}
	for _, a := range defs {
		if includeDeprecated || !deprecated(a.Directives) {
			list = append(list, introInputValue{a.Name, a.Description, a.Type, a.DefaultValue, a.Directives})
		}
	}
	return list
}

type introEnumValue struct{ v *ast.EnumValueDefinition }

func (introEnumValue) typeName() string { return "__EnumValue" }

func (x introEnumValue) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	switch field {
	case "name":
		return x.v.Name
	case "description":
		return optional(x.v.Description)
	case "isDeprecated":
		return deprecated(x.v.Directives)
	case "deprecationReason":
		return deprecationReason(x.v.Directives)
	}
	return nil
}

type introDirective struct{ d *ast.DirectiveDefinition }

func (introDirective) typeName() string { return "__Directive" }

func (x introDirective) resolve(s *gqlScaffolder, field string, args map[string]any) any {
	switch field {
	case "name":
		return x.d.Name
	case "description":
		return optional(x.d.Description)
	case "isRepeatable":
		return x.d.IsRepeatable
	case "locations":
		locations := make([]any, len(x.d.Locations))
		for i, l := range x.d.Locations {
			locations[i] = string(l)
		}
		return locations
	case "args":
		return introArgs(x.d.Arguments, args)
	}
	return nil
}

func deprecationReason(dirs ast.DirectiveList) any {
	d := dirs.ForName("deprecated")
	if d == nil {
		return nil
	}
	if a := d.Arguments.ForName("reason"); a != nil {
		return a.Value.Raw
	}
	return "No longer supported"
}

// optional turns an empty description into null, as GitHub returns it.
func optional(text string) any {
	if text == "" {
		return nil
	}
	return text
}
