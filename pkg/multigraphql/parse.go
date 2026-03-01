package multigraphql

import (
	"regexp"
	"strings"
)

// A Query is a parsed GraphQL query
type Query struct {
	query     string
	fragments string
	variables map[string]string
}

var (
	// matches opening `query(...) {`
	queryStart = regexp.MustCompile(`(?m)^\s*(?:query(?:\s*\(([^)]+)\))?\s*)?\{[ \t]*(\r?\n)?`)
	// matches trailing `}`
	queryEnd = regexp.MustCompile(`\s*\}\s*$`)
)

// Parse splits a GraphQL query into parts
func Parse(q string) Query {
	var fragments string
	queryStr := q

	m := queryStart.FindStringSubmatchIndex(q)
	if len(m) > 0 {
		fragments = q[0:m[0]]
		queryStr = queryEnd.ReplaceAllLiteralString(q[m[1]:], "")
	}

	result := Query{
		query:     queryStr,
		fragments: fragments,
		variables: make(map[string]string),
	}

	if len(m) > 2 && m[2] > -1 {
		parseVariables(result.variables, q[m[2]:m[3]])
	}

	return result
}

func parseVariables(dest map[string]string, vars string) {
	// parse query variables, e.g. `$foo: String!, $bar: Int = 5`
	for _, v := range strings.Split(vars, "$") {
		keyValue := strings.SplitN(v, ":", 2)
		key := graphqlTrim(keyValue[0])
		if key == "" || len(keyValue) < 2 {
			continue
		}
		value := graphqlTrim(keyValue[1])
		dest[key] = value
	}
}

func graphqlTrim(s string) string {
	return strings.Trim(s, " \t\r\n,")
}
