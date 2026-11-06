package main

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

// trimGraphQL removes the fields that a query didn't ask for from a stored
// GraphQL response, because GitHub only sends what's asked for. One stored
// response can then serve gh versions that ask for different fields. Fields
// that the query asks for and the response lacks stay missing, and the schema
// check names them. Fields match by response key, whatever their arguments.
//
// schema can be nil. It's only needed to tell whether a fragment on an
// interface applies to an object. Without it, such fragments are kept. The
// body comes back unchanged if the query or the body can't be read.
func trimGraphQL(body []byte, gql *graphQLRequest, schema *ast.Schema) []byte {
	doc, err := parser.ParseQuery(&ast.Source{Name: "query", Input: gql.Query})
	if err != nil || len(doc.Operations) == 0 {
		return body
	}
	op := doc.Operations.ForName(gql.Operation)
	if op == nil {
		return body
	}
	v, err := decodeBody(body)
	if err != nil {
		return body
	}
	top, ok := v.(map[string]any)
	if !ok {
		return body
	}
	data, ok := top["data"].(map[string]any)
	if !ok {
		return body
	}

	t := &trimmer{doc: doc, schema: schema}
	t.vars, err = operationVariables(op, gql.Variables)
	if err != nil {
		return body
	}
	top["data"] = t.object(data, op.SelectionSet)
	trimmed, err := json.Marshal(top)
	if err != nil {
		return body
	}
	return trimmed
}

type trimmer struct {
	doc    *ast.QueryDocument
	schema *ast.Schema
	vars   map[string]any
}

func (t *trimmer) object(obj map[string]any, sel ast.SelectionSet) map[string]any {
	typename, _ := obj["__typename"].(string)
	var keys []string
	groups := map[string][]*ast.Field{}
	t.collect(sel, typename, map[string]bool{}, &keys, groups)
	if typename == "" {
		for _, key := range keys {
			if groups[key][0].Name == "__typename" {
				typename, _ = obj[key].(string)
				if typename != "" {
					break
				}
			}
		}
		if typename != "" {
			keys = nil
			groups = map[string][]*ast.Field{}
			t.collect(sel, typename, map[string]bool{}, &keys, groups)
		}
	}

	out := make(map[string]any, len(keys))
	for _, key := range keys {
		if v, ok := obj[key]; ok {
			out[key] = t.value(v, mergeSelections(groups[key]))
		}
	}
	return out
}

func (t *trimmer) value(v any, sel ast.SelectionSet) any {
	if len(sel) == 0 {
		return v
	}
	switch v := v.(type) {
	case map[string]any:
		return t.object(v, sel)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = t.value(item, sel)
		}
		return out
	}
	return v
}

// collect lists the response keys a selection asks for on an object whose
// __typename is typename, merging the selections of fields that share a key.
// visiting guards against fragments that spread themselves, which GitHub
// would reject.
func (t *trimmer) collect(sel ast.SelectionSet, typename string, visiting map[string]bool, keys *[]string, groups map[string][]*ast.Field) {
	for _, item := range sel {
		switch x := item.(type) {
		case *ast.Field:
			if skippedBy(x.Directives, t.vars) {
				continue
			}
			if _, seen := groups[x.Alias]; !seen {
				*keys = append(*keys, x.Alias)
			}
			groups[x.Alias] = append(groups[x.Alias], x)
		case *ast.InlineFragment:
			if !skippedBy(x.Directives, t.vars) && t.applies(x.TypeCondition, typename) {
				t.collect(x.SelectionSet, typename, visiting, keys, groups)
			}
		case *ast.FragmentSpread:
			def := t.doc.Fragments.ForName(x.Name)
			if def == nil || visiting[x.Name] || skippedBy(x.Directives, t.vars) || !t.applies(def.TypeCondition, typename) {
				continue
			}
			visiting[x.Name] = true
			t.collect(def.SelectionSet, typename, visiting, keys, groups)
			delete(visiting, x.Name)
		}
	}
}

// applies reports whether a fragment on cond applies to an object whose
// __typename is typename. Without a __typename, every fragment applies.
func (t *trimmer) applies(cond, typename string) bool {
	if cond == "" || typename == "" || cond == typename {
		return true
	}
	if t.schema == nil || t.schema.Types[typename] == nil {
		return true
	}
	return isA(t.schema, t.schema.Types[typename], cond)
}

// queryFields lists the response paths a query asks for, such as
// data.repository.issue.title. Fragments are flattened, as they are in the
// response, and nil means the query couldn't be read.
func queryFields(query, operation string) map[string]bool {
	doc, err := parser.ParseQuery(&ast.Source{Name: "query", Input: query})
	if err != nil || len(doc.Operations) == 0 {
		return nil
	}
	op := doc.Operations.ForName(operation)
	if op == nil {
		return nil
	}
	fields := map[string]bool{}
	var walk func(prefix string, sel ast.SelectionSet, visiting map[string]bool)
	walk = func(prefix string, sel ast.SelectionSet, visiting map[string]bool) {
		for _, item := range sel {
			switch x := item.(type) {
			case *ast.Field:
				path := prefix + "." + x.Alias
				fields[path] = true
				walk(path, x.SelectionSet, visiting)
			case *ast.InlineFragment:
				walk(prefix, x.SelectionSet, visiting)
			case *ast.FragmentSpread:
				def := doc.Fragments.ForName(x.Name)
				if def == nil || visiting[x.Name] {
					continue
				}
				visiting[x.Name] = true
				walk(prefix, def.SelectionSet, visiting)
				delete(visiting, x.Name)
			}
		}
	}
	walk("data", op.SelectionSet, map[string]bool{})
	return fields
}

// onlyIn lists the fields in a that b doesn't ask for. A new object is listed
// once, without the fields inside it.
func onlyIn(a, b map[string]bool) []string {
	var paths []string
	for path := range a {
		if b[path] {
			continue
		}
		parent := path[:strings.LastIndex(path, ".")]
		if parent == "data" || b[parent] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}
