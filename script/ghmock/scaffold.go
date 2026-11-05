package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/vektah/gqlparser/v2/ast"
)

// scaffoldOptions are the parsed flags of ghmock scaffold.
type scaffoldOptions struct {
	name    string
	missing bool
}

func parseScaffoldArgs(args []string) (scaffoldOptions, error) {
	var opts scaffoldOptions
	fs := flag.NewFlagSet("scaffold", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.missing, "missing", false, "scaffold every request in the last run that's missing a response or fields")
	name, err := parseWithName(fs, args)
	if err != nil {
		return scaffoldOptions{}, err
	}
	opts.name = name

	switch {
	case opts.name == "" && !opts.missing:
		return scaffoldOptions{}, newUsageError("pass a request name, such as \"graphql IssueByNumber\", or --missing")
	case opts.name != "" && opts.missing:
		return scaffoldOptions{}, newUsageError("use either a request name or --missing, not both")
	}
	if opts.name != "" {
		if err := validateName(opts.name); err != nil {
			return scaffoldOptions{}, usageError{err}
		}
	}
	return opts, nil
}

func cmdScaffold(args []string, stdout, stderr io.Writer) error {
	opts, err := parseScaffoldArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	entries, err := st.readLog()
	if err != nil {
		return err
	}
	responses, err := st.loadResponses()
	if err != nil {
		return err
	}

	targets := []string{opts.name}
	if opts.missing {
		last, err := st.latestRequests(entries)
		if err != nil {
			return err
		}
		if len(last) == 0 {
			if len(entries) > 0 {
				return fmt.Errorf("no requests logged in the latest run; run gh with ghmock run first")
			}
			return fmt.Errorf("no requests logged yet; run gh with ghmock run first")
		}
		entries = last
		targets = requestNames(entries)
	}

	chk := newChecker(stderr)
	if err := requireSchemas(chk, targets, responses); err != nil {
		return err
	}

	before, err := json.Marshal(responses)
	if err != nil {
		return err
	}
	var lines []string
	for _, name := range targets {
		var out []string
		var err error
		if strings.HasPrefix(name, "graphql ") {
			out, err = scaffoldGraphQL(chk, name, responses, entries, !opts.missing)
		} else {
			out, err = scaffoldREST(chk, name, responses, !opts.missing)
		}
		if err != nil {
			if !opts.missing {
				return err
			}
			out = []string{fmt.Sprintf("%s: can't scaffold: %v", name, err)}
		}
		lines = append(lines, out...)
	}

	after, err := json.Marshal(responses)
	if err != nil {
		return err
	}
	if string(before) != string(after) {
		if err := st.saveResponses(responses); err != nil {
			return err
		}
	}
	if len(lines) == 0 {
		lines = []string{"Nothing to add: every request in the last run has a response, with every field gh asked for."}
	}
	for _, line := range lines {
		fmt.Fprintln(stdout, line)
	}
	return nil
}

// requestNames lists the names of the requests gh made, once each, in order.
func requestNames(entries []loggedRequest) []string {
	seen := map[string]bool{}
	var names []string
	for _, e := range entries {
		if e.Blocked || e.Name == "" || seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		names = append(names, e.Name)
	}
	return names
}

// requireSchemas loads the schemas that scaffolding the targets needs. Unlike
// checks, scaffolding can't go ahead without them.
func requireSchemas(chk *checker, targets []string, responses responseSet) error {
	for _, name := range targets {
		method, target, _ := strings.Cut(name, " ")
		switch {
		case method == "graphql":
			if _, err := chk.graphQL(); err != nil {
				return fmt.Errorf("scaffolding needs GitHub's GraphQL schema: %w", err)
			}
		case strings.HasPrefix(target, "/") && len(responses[name]) == 0:
			if _, err := chk.rest(); err != nil {
				return fmt.Errorf("scaffolding needs GitHub's REST description: %w", err)
			}
		}
	}
	return nil
}

// sentQuery is a GraphQL query gh sent, with its variables.
type sentQuery struct {
	query string
	vars  json.RawMessage
}

func (q sentQuery) request(name string) loggedRequest {
	operation := strings.TrimPrefix(name, "graphql ")
	if operation == "<anonymous>" {
		operation = ""
	}
	return loggedRequest{Name: name, Method: http.MethodPost, Host: dotcomAPIHost, Path: "/graphql",
		GraphQL: &graphQLRequest{Query: q.query, Variables: q.vars, Operation: operation}}
}

// queriesFor returns the queries gh sent for a GraphQL request in the most
// recent run that sent it, once each. Comparing versions can send more than
// one, and a stored response needs the fields of all of them.
func queriesFor(entries []loggedRequest, name string) []sentQuery {
	last := -1
	for i, entrie := range slices.Backward(entries) {
		if entrie.Name == name && entrie.GraphQL != nil {
			last = i
			break
		}
	}
	if last < 0 {
		return nil
	}
	run := entries[last].Run
	seen := map[string]bool{}
	var queries []sentQuery
	for _, e := range entries {
		if e.Run != run || e.Name != name || e.GraphQL == nil {
			continue
		}
		key := e.GraphQL.Query + "\x00" + compactJSON(e.GraphQL.Variables)
		if seen[key] {
			continue
		}
		seen[key] = true
		queries = append(queries, sentQuery{e.GraphQL.Query, e.GraphQL.Variables})
	}
	return queries
}

// scaffoldGraphQL adds a response built from gh's query, or adds the fields
// gh asked for to stored responses that lack them. explicit is true when the
// caller named this request, so it reports when there's nothing to do.
func scaffoldGraphQL(chk *checker, name string, responses responseSet, entries []loggedRequest, explicit bool) ([]string, error) {
	schema, err := chk.graphQL()
	if err != nil {
		return nil, err
	}
	queries := queriesFor(entries, name)
	if len(queries) == 0 {
		return nil, fmt.Errorf("gh hasn't sent %s, so there's no query to build it from; run gh first", name)
	}
	type parsed struct {
		sentQuery
		op *ast.OperationDefinition
	}
	var ops []parsed
	for _, q := range queries {
		op, _ := checkGraphQLQuery(schema, q.query, q.request(name).GraphQL.Operation)
		if op == nil {
			return nil, fmt.Errorf("gh's query couldn't be read; ghmock log --full shows it")
		}
		if rootType(schema, op.Operation) == nil {
			return nil, fmt.Errorf("GitHub's schema has no %s type", op.Operation)
		}
		ops = append(ops, parsed{q, op})
	}

	list := responses[name]
	if len(list) == 0 {
		s := newGQLScaffolder(schema)
		data := map[string]any{}
		for _, p := range ops {
			if err := s.useVariables(p.op, p.vars); err != nil {
				return nil, err
			}
			s.fillObject("data", data, rootType(schema, p.op.Operation), p.op.SelectionSet, false)
		}
		body, err := json.Marshal(map[string]any{"data": data})
		if err != nil {
			return nil, err
		}
		resp := response{Body: body}
		for _, p := range ops {
			if res := chk.checkRequest(p.request(name), &resp); len(res.responseProblems) > 0 {
				return nil, fmt.Errorf("the placeholder response doesn't match GitHub's schema, which is a ghmock bug:\n%s",
					indentProblems(res.responseProblems, "  ", maxShownProblems))
			}
		}
		responses[name] = []response{resp}
		detail := "with placeholder values"
		if !s.placeholders {
			detail = "from GitHub's schema"
		}
		return []string{fmt.Sprintf("%s: added response 1 of 1 %s", name, detail)}, nil
	}

	var lines []string
	for i := range list {
		r := list[i]
		if r.status() != http.StatusOK {
			continue
		}
		v, err := decodeBody(r.Body)
		if err != nil {
			continue
		}
		top, ok := v.(map[string]any)
		if !ok {
			continue
		}
		data, ok := top["data"].(map[string]any)
		if !ok {
			continue
		}
		s := newGQLScaffolder(schema)
		for _, p := range ops {
			if err := s.useVariables(p.op, p.vars); err != nil {
				return nil, err
			}
			s.fillObject("data", data, rootType(schema, p.op.Operation), p.op.SelectionSet, true)
		}
		if len(s.added) == 0 {
			continue
		}
		where := fmt.Sprintf("%s: response %d of %d", name, i+1, len(list))
		added := addedPaths(s.added)
		if r.OffSchema {
			lines = append(lines, fmt.Sprintf("%s is off-schema, so it was left as is; it lacks %s", where, added))
			continue
		}
		body, err := json.Marshal(top)
		if err != nil {
			return nil, err
		}
		r.Body = body
		list[i] = r
		lines = append(lines, fmt.Sprintf("%s: added %s", where, added))
		var problems []string
		for _, p := range ops {
			problems = append(problems, chk.checkRequest(p.request(name), &r).responseProblems...)
		}
		if problems = dedupe(problems); len(problems) > 0 {
			lines = append(lines, fmt.Sprintf("%s still doesn't match GitHub's schema:\n%s", where,
				strings.TrimSuffix(indentProblems(problems, "  ", maxShownProblems), "\n")))
		}
	}
	if len(lines) == 0 && explicit {
		lines = append(lines, fmt.Sprintf("%s: nothing to add; every stored response has every field gh asked for", name))
	}
	return lines, nil
}

var listIndex = regexp.MustCompile(`\.\d+(\.|$)`)

// addedPaths lists where fields were added. Indexes become *, so a field added
// to every item of a list is listed once.
func addedPaths(paths []string) string {
	for i, p := range paths {
		for listIndex.MatchString(p) {
			p = listIndex.ReplaceAllString(p, ".*$1")
		}
		paths[i] = p
	}
	return joinLimited(dedupe(paths), maxShownProblems)
}

// scaffoldREST adds a response for a REST request from GitHub's description.
// A request that already has a response is left alone: ghmock set checks REST
// responses in full, so a stored one has everything GitHub requires.
func scaffoldREST(chk *checker, name string, responses responseSet, explicit bool) ([]string, error) {
	method, target, _ := strings.Cut(name, " ")
	if !strings.HasPrefix(target, "/") {
		return nil, fmt.Errorf("%s isn't on the REST API host, so GitHub's REST description doesn't cover it", target)
	}
	if len(responses[name]) > 0 {
		if explicit {
			return []string{fmt.Sprintf("%s: already has a response, so nothing was added", name)}, nil
		}
		return nil, nil
	}
	idx, err := chk.rest()
	if err != nil {
		return nil, err
	}
	resp, detail, err := idx.scaffold(method, target)
	if err != nil {
		return nil, err
	}
	responses[name] = []response{resp}
	return []string{fmt.Sprintf("%s: added response 1 of 1%s", name, detail)}, nil
}
