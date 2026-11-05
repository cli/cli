package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// Placeholder values. They pass GitHub's schema and look fake on purpose.
const (
	placeholderTime = "2026-01-01T00:00:00Z"
	placeholderDate = "2026-01-01"
	placeholderURL  = "https://github.localhost/owner/name"
	placeholderOID  = "0123456789abcdef0123456789abcdef01234567"
)

// preferredEnumValues describe an ordinary, healthy object. Without them, a
// placeholder takes an enum's first value, which can send gh down an error
// path: gh pr merge refuses a pull request whose mergeStateStatus is BEHIND,
// the first value of MergeStateStatus.
var preferredEnumValues = []string{"OPEN", "PUBLIC", "CLEAN", "MERGEABLE", "SUCCESS"}

func enumPlaceholder(values []string) string {
	for _, want := range preferredEnumValues {
		for _, v := range values {
			if strings.EqualFold(v, want) {
				return v
			}
		}
	}
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// gqlScaffolder builds a GraphQL response for a query, or adds the fields that
// a stored response lacks. It never changes a value that's already there.
//
// Every field gets a value, never null, and every list gets one item. Taking
// a value away is one ghmock set, but building a valid one by hand takes
// many. It also keeps gh on its usual path: lookups such as repository find
// something, and gh pr checks finds a commit to check. Numbers are 1, so
// counts such as totalCount match the lists.
//
// Introspection fields such as __type get GitHub's real answers, because gh
// uses them to decide which queries to send.
type gqlScaffolder struct {
	schema       *ast.Schema
	vars         map[string]any
	ids          map[string]int
	added        []string
	placeholders bool
}

func newGQLScaffolder(schema *ast.Schema) *gqlScaffolder {
	return &gqlScaffolder{schema: schema, ids: map[string]int{}}
}

// useVariables sets the variables gh sent, which @include and @skip read.
func (s *gqlScaffolder) useVariables(raw json.RawMessage) {
	s.vars = nil
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s.vars)
	}
}

// fillObject adds the fields that sel asks for and obj lacks. With record set,
// it remembers where it added them, so they can be reported.
func (s *gqlScaffolder) fillObject(path string, obj map[string]any, def *ast.Definition, sel ast.SelectionSet, record bool) {
	concrete := s.concreteType(def, obj, sel)
	var keys []string
	groups := map[string][]*ast.Field{}
	s.collect(sel, concrete, &keys, groups)

	for _, key := range keys {
		fields := groups[key]
		f := fields[0]
		child := path + "." + key
		existing, has := obj[key]
		switch {
		case f.Name == "__typename":
			if !has {
				obj[key] = concrete.Name
			}
		case f.Definition == nil:
			// The schema has no such field. The query check reports it.
			continue
		case strings.HasPrefix(f.Name, "__"):
			if !has {
				obj[key] = s.introspect(f, mergeSelections(fields))
			}
		case has:
			obj[key] = s.fill(child, existing, f.Definition.Type, mergeSelections(fields))
		default:
			obj[key] = s.generate(f.Definition.Type, mergeSelections(fields), f.Name, concrete.Name)
		}
		if !has && record {
			s.added = append(s.added, child)
		}
	}
}

// fill walks a value that's already there, adding missing fields inside it.
// null stays null, because that can be what the response is testing.
func (s *gqlScaffolder) fill(path string, v any, typ *ast.Type, sel ast.SelectionSet) any {
	if v == nil {
		return nil
	}
	if typ.Elem != nil {
		list, ok := v.([]any)
		if !ok {
			return v
		}
		for i, item := range list {
			list[i] = s.fill(fmt.Sprintf("%s.%d", path, i), item, typ.Elem, sel)
		}
		return list
	}
	def := s.schema.Types[typ.NamedType]
	if def == nil {
		return v
	}
	switch def.Kind {
	case ast.Object, ast.Interface, ast.Union:
		if obj, ok := v.(map[string]any); ok {
			s.fillObject(path, obj, def, sel, true)
		}
	}
	return v
}

// generate makes a placeholder value of a type. parent is the type of the
// object that holds the field.
func (s *gqlScaffolder) generate(typ *ast.Type, sel ast.SelectionSet, field, parent string) any {
	if typ.Elem != nil {
		s.placeholders = true
		return []any{s.generate(typ.Elem, sel, field, parent)}
	}
	def := s.schema.Types[typ.NamedType]
	if def == nil {
		return nil
	}
	switch def.Kind {
	case ast.Scalar:
		s.placeholders = true
		return s.scalar(def.Name, field, parent)
	case ast.Enum:
		s.placeholders = true
		var values []string
		for _, v := range def.EnumValues {
			if !deprecated(v.Directives) {
				values = append(values, v.Name)
			}
		}
		if len(values) == 0 {
			values = enumValueNames(def)
		}
		return enumPlaceholder(values)
	case ast.Object, ast.Interface, ast.Union:
		obj := map[string]any{}
		s.fillObject("", obj, def, sel, false)
		return obj
	}
	return nil
}

// scalar makes a placeholder for a scalar field. Strings hold the field's
// name, so gh's output shows where each value came from.
func (s *gqlScaffolder) scalar(scalar, field, parent string) any {
	switch scalar {
	case "ID":
		s.ids[parent]++
		return fmt.Sprintf("%s_%d", parent, s.ids[parent])
	case "Int", "Float":
		return 1
	case "BigInt":
		return "1"
	case "Boolean":
		// Fields such as hasIssuesEnabled turn a feature on. gh issue list
		// and gh issue create stop when hasIssuesEnabled is false.
		return strings.HasPrefix(field, "has") && strings.HasSuffix(field, "Enabled")
	case "DateTime", "GitTimestamp":
		return placeholderTime
	case "PreciseDateTime":
		return "2026-01-01T00:00:00.000Z"
	case "Date":
		return placeholderDate
	case "URI":
		return placeholderURL
	case "GitObjectID":
		return placeholderOID
	case "GitRefname":
		return "refs/heads/main"
	case "GitSSHRemote":
		return "git@github.localhost:owner/name.git"
	case "HTML":
		return "<p>" + field + "</p>"
	case "Base64String":
		return base64.StdEncoding.EncodeToString([]byte(field))
	}
	if field == "nameWithOwner" {
		// gh splits this at the slash.
		return "owner/name"
	}
	return field
}

// concreteType picks the object type to build for a field whose type is an
// interface or union: the type in __typename, else the first fragment's type,
// else the first possible type by name.
func (s *gqlScaffolder) concreteType(def *ast.Definition, obj map[string]any, sel ast.SelectionSet) *ast.Definition {
	if !def.IsAbstractType() {
		return def
	}
	possible := s.schema.GetPossibleTypes(def)
	find := func(name string) *ast.Definition {
		for _, t := range possible {
			if t.Name == name {
				return t
			}
		}
		return nil
	}
	if name, ok := obj["__typename"].(string); ok {
		if t := find(name); t != nil {
			return t
		}
	}
	if t := s.fragmentType(sel, find); t != nil {
		return t
	}
	if len(possible) == 0 {
		return def
	}
	sorted := append([]*ast.Definition(nil), possible...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	return sorted[0]
}

// fragmentType finds the first fragment, in query order, on a possible type.
func (s *gqlScaffolder) fragmentType(sel ast.SelectionSet, find func(string) *ast.Definition) *ast.Definition {
	for _, item := range sel {
		var cond string
		var inner ast.SelectionSet
		switch x := item.(type) {
		case *ast.InlineFragment:
			if s.skipped(x.Directives) {
				continue
			}
			cond, inner = x.TypeCondition, x.SelectionSet
		case *ast.FragmentSpread:
			if x.Definition == nil || s.skipped(x.Directives) {
				continue
			}
			cond, inner = x.Definition.TypeCondition, x.Definition.SelectionSet
		default:
			continue
		}
		if t := find(cond); t != nil {
			return t
		}
		if t := s.fragmentType(inner, find); t != nil {
			return t
		}
	}
	return nil
}

// collect groups the fields a selection asks for on an object of type
// concrete by response key, in query order. Fields that share a key are
// merged into one value, as GraphQL does.
func (s *gqlScaffolder) collect(sel ast.SelectionSet, concrete *ast.Definition, keys *[]string, groups map[string][]*ast.Field) {
	for _, item := range sel {
		switch x := item.(type) {
		case *ast.Field:
			if s.skipped(x.Directives) {
				continue
			}
			if _, seen := groups[x.Alias]; !seen {
				*keys = append(*keys, x.Alias)
			}
			groups[x.Alias] = append(groups[x.Alias], x)
		case *ast.InlineFragment:
			if s.skipped(x.Directives) || !s.applies(x.TypeCondition, concrete) {
				continue
			}
			s.collect(x.SelectionSet, concrete, keys, groups)
		case *ast.FragmentSpread:
			if x.Definition == nil || s.skipped(x.Directives) || !s.applies(x.Definition.TypeCondition, concrete) {
				continue
			}
			s.collect(x.Definition.SelectionSet, concrete, keys, groups)
		}
	}
}

func (s *gqlScaffolder) applies(cond string, concrete *ast.Definition) bool {
	return cond == "" || concrete == nil || isA(s.schema, concrete, cond)
}

func (s *gqlScaffolder) skipped(dirs ast.DirectiveList) bool {
	return skippedBy(dirs, s.vars)
}

// skippedBy evaluates @skip and @include with the variables gh sent.
func skippedBy(dirs ast.DirectiveList, vars map[string]any) bool {
	if d := dirs.ForName("skip"); d != nil && condition(d, vars) {
		return true
	}
	if d := dirs.ForName("include"); d != nil && !condition(d, vars) {
		return true
	}
	return false
}

func condition(d *ast.Directive, vars map[string]any) bool {
	arg := d.Arguments.ForName("if")
	if arg == nil {
		return false
	}
	v, err := arg.Value.Value(vars)
	b, ok := v.(bool)
	return err == nil && ok && b
}

// arguments evaluates a field's arguments, filling in defaults.
func (s *gqlScaffolder) arguments(f *ast.Field) map[string]any {
	args := map[string]any{}
	if f.Definition == nil {
		return args
	}
	for _, def := range f.Definition.Arguments {
		if a := f.Arguments.ForName(def.Name); a != nil {
			if v, err := a.Value.Value(s.vars); err == nil {
				args[def.Name] = v
			}
		} else if def.DefaultValue != nil {
			if v, err := def.DefaultValue.Value(nil); err == nil {
				args[def.Name] = v
			}
		}
	}
	return args
}

func mergeSelections(fields []*ast.Field) ast.SelectionSet {
	if len(fields) == 1 {
		return fields[0].SelectionSet
	}
	var merged ast.SelectionSet
	for _, f := range fields {
		merged = append(merged, f.SelectionSet...)
	}
	return merged
}

func deprecated(dirs ast.DirectiveList) bool {
	return dirs.ForName("deprecated") != nil
}
