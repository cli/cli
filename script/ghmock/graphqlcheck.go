package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/parser"
	"github.com/vektah/gqlparser/v2/validator"
	validatorrules "github.com/vektah/gqlparser/v2/validator/rules"
)

func loadGraphQLSchema(data []byte, name string) (*ast.Schema, error) {
	schema, err := gqlparser.LoadSchema(&ast.Source{Name: name, Input: string(data)})
	if err != nil {
		return nil, errors.New(graphQLErrorText(err))
	}
	return schema, nil
}

// checkGraphQLQuery parses gh's query and validates it against the schema. It
// returns a usable operation even when validation finds non-cyclic problems,
// so the response can still be checked where the query is valid.
func checkGraphQLQuery(schema *ast.Schema, query, operation string) (*ast.OperationDefinition, []string) {
	doc, err := parser.ParseQuery(&ast.Source{Name: "query", Input: query})
	if err != nil {
		return nil, []string{graphQLErrorText(err)}
	}

	rules := validatorrules.NewDefaultRules()
	// GitHub accepts fragments on different types that ask for one field name
	// with different types, such as state on both Issue and PullRequest. gh
	// relies on that; retain other overlapping-field checks.
	var problems []string
	cyclic := false
	for _, e := range validator.ValidateWithRules(schema, doc, rules) {
		if exclusiveTypeConflict(schema, doc, e) {
			continue
		}
		if e.Rule == validatorrules.NoFragmentCyclesRule.Name {
			cyclic = true
		}
		problems = append(problems, graphQLErrorText(e))
	}
	if cyclic {
		return nil, dedupe(problems)
	}

	if len(doc.Operations) == 0 {
		return nil, append(problems, "the query has no operation")
	}
	if operation == "" && len(doc.Operations) > 1 {
		return nil, append(problems, "operationName is required when the document has multiple operations")
	}
	op := doc.Operations.ForName(operation)
	if op == nil {
		return nil, append(problems, fmt.Sprintf("the query has no operation named %q", operation))
	}
	return op, dedupe(problems)
}

func exclusiveTypeConflict(schema *ast.Schema, doc *ast.QueryDocument, problem *gqlerror.Error) bool {
	if problem.Rule != validatorrules.OverlappingFieldsCanBeMergedRule.Name ||
		!strings.Contains(problem.Message, "conflicting types") || len(problem.Locations) != 1 {
		return false
	}
	var fields []*ast.Field
	var second *ast.Field
	var collect func(ast.SelectionSet)
	collect = func(sel ast.SelectionSet) {
		for _, item := range sel {
			switch field := item.(type) {
			case *ast.Field:
				fields = append(fields, field)
				if (gqlerror.Location{Line: field.Position.Line, Column: field.Position.Column}) == problem.Locations[0] {
					second = field
				}
				collect(field.SelectionSet)
			case *ast.InlineFragment:
				collect(field.SelectionSet)
			}
		}
	}
	for _, op := range doc.Operations {
		collect(op.SelectionSet)
	}
	for _, fragment := range doc.Fragments {
		collect(fragment.SelectionSet)
	}
	if second == nil || second.Definition == nil || second.ObjectDefinition.Kind != ast.Object ||
		!schema.Types[second.Definition.Type.Name()].IsLeafType() {
		return false
	}
	matched := false
	for _, first := range fields {
		if first == second || first.Definition == nil || first.Alias != second.Alias {
			continue
		}
		message := fmt.Sprintf(
			"Fields %q conflict because they return conflicting types %q and %q. Use different aliases on the fields to fetch both if this was intentional.",
			first.Alias, first.Definition.Type.String(), second.Definition.Type.String())
		if message != problem.Message {
			continue
		}
		// The pinned validator reports only the second field's location. Every
		// possible match must be the concrete-type exception, not an interface.
		if first.ObjectDefinition.Kind != ast.Object || first.ObjectDefinition.Name == second.ObjectDefinition.Name ||
			!schema.Types[first.Definition.Type.Name()].IsLeafType() {
			return false
		}
		matched = true
	}
	return matched
}

func graphQLErrorText(err error) string {
	if ge, ok := errors.AsType[*gqlerror.Error](err); ok {
		if len(ge.Locations) > 0 {
			return formatGraphQLError(ge.Message, ge.Locations[0].Line, ge.Locations[0].Column)
		}
		return ge.Message
	}
	return err.Error()
}

// checkGraphQLResponse compares a response body with the query gh sent. Paths
// in problems use the same dotted form as ghmock set --at.
func checkGraphQLResponse(schema *ast.Schema, op *ast.OperationDefinition, body []byte, variables map[string]any) []string {
	if len(bytes.TrimSpace(body)) == 0 {
		return []string{"body: missing, but a GraphQL response needs data or errors"}
	}
	v, err := decodeBody(body)
	if err != nil {
		return []string{fmt.Sprintf("body: not valid JSON: %v", err)}
	}
	top, ok := v.(map[string]any)
	if !ok {
		return []string{fmt.Sprintf("body: should be an object, but is %s", describeJSON(v))}
	}

	w := &gqlWalker{schema: schema, variables: variables}
	errs, hasErrors := top["errors"]
	if hasErrors {
		w.checkErrors(errs)
	}
	data := top["data"]
	if data == nil {
		if !hasErrors {
			w.addf("body", "has neither data nor errors")
		}
		return dedupe(w.problems)
	}

	root := rootType(schema, op.Operation)
	if root == nil {
		w.addf("data", "GitHub's schema has no %s type", op.Operation)
		return dedupe(w.problems)
	}
	obj, ok := data.(map[string]any)
	if !ok {
		w.addf("data", "should be an object, but is %s", describeJSON(data))
		return dedupe(w.problems)
	}
	w.checkObject("data", obj, root, op.SelectionSet, true)
	return dedupe(w.problems)
}

func rootType(schema *ast.Schema, op ast.Operation) *ast.Definition {
	switch op {
	case ast.Mutation:
		return schema.Mutation
	case ast.Subscription:
		return schema.Subscription
	default:
		return schema.Query
	}
}

// gqlWalker walks a response alongside the query that asked for it.
type gqlWalker struct {
	schema    *ast.Schema
	variables map[string]any
	problems  []string
}

func (w *gqlWalker) addf(path, format string, args ...any) {
	w.problems = append(w.problems, path+": "+fmt.Sprintf(format, args...))
}

func (w *gqlWalker) checkErrors(v any) {
	list, ok := v.([]any)
	if !ok {
		w.addf("errors", "should be a list, but is %s", describeJSON(v))
		return
	}
	if len(list) == 0 {
		w.addf("errors", "must contain at least one error")
	}
	for i, item := range list {
		path := fmt.Sprintf("errors.%d", i)
		obj, ok := item.(map[string]any)
		if !ok {
			w.addf(path, "should be an object, but is %s", describeJSON(item))
			continue
		}
		if _, ok := obj["message"].(string); !ok {
			w.addf(path+".message", "should be a string, but is %s", describeJSON(obj["message"]))
		}
	}
}

// selected is a field the query asked for. Fields in fragments or directives
// that might not apply aren't required. Fields in
// fragments for a type the response doesn't name aren't checked either,
// because two fragments can ask for one key with different types.
type selected struct {
	field    *ast.Field
	required bool
	checked  bool
}

// collectFields lists the fields a selection set asks for on an object of type
// concrete. concrete is nil when the object's type isn't known, which happens
// for interfaces and unions when the response has no __typename.
func (w *gqlWalker) collectFields(sel ast.SelectionSet, concrete *ast.Definition, required, checked bool, out *[]selected) {
	for _, s := range sel {
		switch s := s.(type) {
		case *ast.Field:
			included, known := directiveCondition(s.Directives, w.variables)
			if included {
				*out = append(*out, selected{field: s, required: required && known, checked: checked})
			}
		case *ast.InlineFragment:
			w.collectFragment(s.TypeCondition, s.Directives, s.SelectionSet, concrete, required, checked, out)
		case *ast.FragmentSpread:
			if s.Definition != nil {
				w.collectFragment(s.Definition.TypeCondition, s.Directives, s.Definition.SelectionSet, concrete, required, checked, out)
			}
		}
	}
}

func (w *gqlWalker) collectFragment(cond string, dirs ast.DirectiveList, sel ast.SelectionSet, concrete *ast.Definition, required, checked bool, out *[]selected) {
	included, known := directiveCondition(dirs, w.variables)
	if !included {
		return
	}
	required = required && known
	switch {
	case cond == "":
	case concrete == nil:
		required, checked = false, false
	case !w.isA(concrete, cond):
		return
	}
	w.collectFields(sel, concrete, required, checked, out)
}

func directiveCondition(dirs ast.DirectiveList, variables map[string]any) (included, known bool) {
	known = true
	for _, name := range []string{"include", "skip"} {
		d := dirs.ForName(name)
		if d == nil {
			continue
		}
		arg := d.Arguments.ForName("if")
		if arg == nil {
			known = false
			continue
		}
		if variables == nil && arg.Value.Kind == ast.Variable {
			known = false
			continue
		}
		value, err := arg.Value.Value(variables)
		condition, ok := value.(bool)
		if err != nil || !ok {
			known = false
			continue
		}
		if name == "skip" && condition || name == "include" && !condition {
			return false, true
		}
	}
	return true, known
}

// isA reports whether an object of type concrete is also of type name, which
// can be the same type, an interface it implements or a union it belongs to.
func (w *gqlWalker) isA(concrete *ast.Definition, name string) bool {
	if concrete.Name == name {
		return true
	}
	def := w.schema.Types[name]
	if def == nil {
		return false
	}
	for _, t := range w.schema.GetPossibleTypes(def) {
		if t.Name == concrete.Name {
			return true
		}
	}
	return false
}

func (w *gqlWalker) checkObject(path string, obj map[string]any, def *ast.Definition, sel ast.SelectionSet, required bool) {
	concrete := def
	if def.IsAbstractType() {
		concrete = nil
		keys := []string{"__typename"}
		var candidates []selected
		w.collectFields(sel, nil, false, false, &candidates)
		for _, candidate := range candidates {
			if candidate.field.Name == "__typename" && candidate.field.Alias != "__typename" {
				keys = append(keys, candidate.field.Alias)
			}
		}
		for _, key := range keys {
			name, ok := obj[key].(string)
			if !ok {
				continue
			}
			if t := w.schema.Types[name]; t != nil && t.Kind == ast.Object && w.isA(t, def.Name) {
				if concrete != nil && concrete != t {
					w.addf(path+"."+key, "is %q, but this object's type is %s", name, concrete.Name)
					continue
				}
				concrete = t
			} else {
				w.addf(path+"."+key, "%q isn't a type %s can be; it can be %s",
					name, def.Name, listNames(possibleTypeNames(w.schema, def), 10))
			}
		}
	}

	var fields []selected
	w.collectFields(sel, concrete, required, true, &fields)
	for _, f := range fields {
		if f.field.Definition == nil && f.field.Name != "__typename" {
			// The query asks for a field the schema doesn't have. That's
			// reported as a query problem.
			continue
		}
		key := f.field.Alias
		child := path + "." + key
		val, ok := obj[key]
		if !ok {
			if f.required {
				w.addf(child, "missing, but gh asked for it")
			}
			continue
		}
		if !f.checked {
			continue
		}
		if f.field.Name == "__typename" {
			if _, ok := val.(string); !ok {
				w.addf(child, "should be a string, but is %s", describeJSON(val))
			} else if !def.IsAbstractType() && val != def.Name {
				w.addf(child, "is %q, but this object's type is %s", val, def.Name)
			}
			continue
		}
		w.checkValue(child, val, f.field.Definition.Type, f.field.SelectionSet, f.required)
	}
}

func (w *gqlWalker) checkValue(path string, val any, typ *ast.Type, sel ast.SelectionSet, required bool) {
	if val == nil {
		if typ.NonNull {
			w.addf(path, "is null, but GitHub's schema says it can't be (%s)", typ.String())
		}
		return
	}
	if typ.Elem != nil {
		list, ok := val.([]any)
		if !ok {
			w.addf(path, "should be a list (%s), but is %s", typ.String(), describeJSON(val))
			return
		}
		for i, item := range list {
			w.checkValue(fmt.Sprintf("%s.%d", path, i), item, typ.Elem, sel, required)
		}
		return
	}

	def := w.schema.Types[typ.NamedType]
	if def == nil {
		return
	}
	switch def.Kind {
	case ast.Scalar:
		w.checkScalar(path, val, def.Name)
	case ast.Enum:
		if s, ok := val.(string); !ok || def.EnumValues.ForName(s) == nil {
			w.addf(path, "%s isn't a value of %s; it can be %s", jsonText(val), def.Name, listNames(enumValueNames(def), 10))
		}
	case ast.Object, ast.Interface, ast.Union:
		obj, ok := val.(map[string]any)
		if !ok {
			w.addf(path, "should be an object (%s), but is %s", def.Name, describeJSON(val))
			return
		}
		w.checkObject(path, obj, def, sel, required)
	}
}

// stringScalars are the scalars GitHub always sends as JSON strings. Others,
// such as CustomPropertyValue, can hold more than one JSON type.
var stringScalars = map[string]bool{
	"String": true, "ID": true, "Base64String": true, "BigInt": true, "Date": true,
	"DateTime": true, "GitObjectID": true, "GitRefname": true, "GitSSHRemote": true,
	"GitTimestamp": true, "HTML": true, "PreciseDateTime": true, "URI": true,
	"X509Certificate": true,
}

func (w *gqlWalker) checkScalar(path string, val any, name string) {
	switch {
	case stringScalars[name]:
		if _, ok := val.(string); !ok {
			w.addf(path, "should be a string (%s), but is %s", name, describeJSON(val))
		}
	case name == "Int":
		n, ok := val.(json.Number)
		if !ok {
			w.addf(path, "should be a whole number (Int), but is %s", describeJSON(val))
			return
		}
		if i, err := n.Int64(); err != nil || i < math.MinInt32 || i > math.MaxInt32 {
			w.addf(path, "%s isn't a 32-bit whole number (Int)", n)
		}
	case name == "Float":
		if _, ok := val.(json.Number); !ok {
			w.addf(path, "should be a number (Float), but is %s", describeJSON(val))
		}
	case name == "Boolean":
		if _, ok := val.(bool); !ok {
			w.addf(path, "should be true or false (Boolean), but is %s", describeJSON(val))
		}
	}
}

func possibleTypeNames(schema *ast.Schema, def *ast.Definition) []string {
	var names []string
	for _, t := range schema.GetPossibleTypes(def) {
		names = append(names, t.Name)
	}
	return names
}

func enumValueNames(def *ast.Definition) []string {
	var names []string
	for _, v := range def.EnumValues {
		names = append(names, v.Name)
	}
	return names
}

func jsonText(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}
