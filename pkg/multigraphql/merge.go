package multigraphql

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// PreparedQuery represents a Query with associated variable values
type PreparedQuery struct {
	variableValues map[string]interface{}
	Query
}

var identifier = regexp.MustCompile(`\$[a-zA-Z]\w*`)

// Merge combines multiple queries into one while avoiding variable collisions
func Merge(queries ...PreparedQuery) (string, map[string]interface{}) {
	out := &bytes.Buffer{}
	queryStrings := []string{}
	allVariables := map[string]string{}
	allValues := map[string]interface{}{}
	seenFragments := map[string]struct{}{"": {}}

	for i, q := range queries {
		renames := mergeVariables(allVariables, q.variables, func(k string) string {
			return fmt.Sprintf("%s_%03d", k, i)
		})

		for key, value := range q.variableValues {
			if newKey, exists := renames[key]; exists {
				key = newKey
			}
			allValues[key] = value
		}

		if _, seen := seenFragments[q.fragments]; !seen {
			fmt.Fprintln(out, q.fragments)
			seenFragments[q.fragments] = struct{}{}
		}

		finalQuery := renameVariables(q.query, renames)
		queryStrings = append(queryStrings, fmt.Sprintf("multi_%03d: %s", i, finalQuery))
	}

	fmt.Fprint(out, "query")
	writeVariables(out, allVariables)
	fmt.Fprintf(out, " {\n\t%s\n}", strings.Join(queryStrings, "\n\t"))

	return out.String(), allValues
}

func mergeVariables(dest, src map[string]string, keyGen func(string) string) map[string]string {
	renames := map[string]string{}
	for key, value := range src {
		if _, exists := dest[key]; exists {
			newKey := keyGen(key)
			renames[key] = newKey
			key = newKey
		}
		dest[key] = value
	}
	return renames
}

func renameVariables(q string, dictionary map[string]string) string {
	return identifier.ReplaceAllStringFunc(q, func(v string) string {
		if newName, exists := dictionary[v[1:]]; exists {
			return "$" + newName
		}
		return v
	})
}

func writeVariables(out io.Writer, variables map[string]string) {
	if len(variables) == 0 {
		return
	}

	vars := []string{}
	for key, value := range variables {
		vars = append(vars, fmt.Sprintf("$%s: %s", key, value))
	}
	sort.Sort(sort.StringSlice(vars))

	fmt.Fprint(out, "(\n")
	for _, v := range vars {
		fmt.Fprintf(out, "\t%s\n", v)
	}
	fmt.Fprint(out, ")")
}
