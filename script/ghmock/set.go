package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// setOptions are the parsed flags of ghmock set.
type setOptions struct {
	name      string
	index     int
	appendNew bool
	status    int
	headers   []string
	body      string
	bodySet   bool
	bodyFile  string
	at        string
	value     string
	valueSet  bool
	offSchema bool
}

func parseSetArgs(args []string) (setOptions, error) {
	var opts setOptions
	fs := flag.NewFlagSet("set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&opts.index, "index", 0, "which response to change, starting at 1")
	fs.BoolVar(&opts.appendNew, "append", false, "add a new response after the existing ones")
	fs.IntVar(&opts.status, "status", 0, "HTTP status code")
	fs.Var((*stringList)(&opts.headers), "header", "'Name: value' to set, or 'Name:' to remove")
	fs.StringVar(&opts.body, "body", "", "JSON body that replaces the current one")
	fs.StringVar(&opts.bodyFile, "body-file", "", "file holding the JSON body")
	fs.StringVar(&opts.at, "at", "", "dotted path inside the body, such as data.repository.issue.title")
	fs.StringVar(&opts.value, "json", "", "JSON value to put at --at")
	fs.BoolVar(&opts.offSchema, "off-schema", false, "save the response even if it doesn't match GitHub's schema")

	name, err := parseWithName(fs, args)
	if err != nil {
		return setOptions{}, err
	}
	opts.name = name
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "body":
			opts.bodySet = true
		case "json":
			opts.valueSet = true
		}
	})

	if opts.name == "" {
		return setOptions{}, newUsageError("missing request name, such as \"GET /repos/octo/hello\" or \"graphql IssueByNumber\"")
	}
	if err := validateName(opts.name); err != nil {
		return setOptions{}, usageError{err}
	}
	switch {
	case opts.bodySet && opts.bodyFile != "":
		return setOptions{}, newUsageError("use either --body or --body-file, not both")
	case opts.at != "" && !opts.valueSet:
		return setOptions{}, newUsageError("--at needs --json")
	case opts.valueSet && opts.at == "":
		return setOptions{}, newUsageError("--json needs --at")
	case opts.appendNew && opts.index != 0:
		return setOptions{}, newUsageError("use either --append or --index, not both")
	case opts.index < 0:
		return setOptions{}, newUsageError("--index starts at 1")
	case opts.status != 0 && (opts.status < 100 || opts.status > 599):
		return setOptions{}, newUsageError("--status must be between 100 and 599")
	}
	for _, h := range opts.headers {
		if name, _, ok := strings.Cut(h, ":"); !ok || strings.TrimSpace(name) == "" {
			return setOptions{}, newUsageError("--header %q must look like 'Name: value'", h)
		}
	}
	changes := opts.status != 0 || len(opts.headers) > 0 || opts.bodySet || opts.bodyFile != "" || opts.at != "" || opts.offSchema
	if !changes && !opts.appendNew {
		return setOptions{}, newUsageError("nothing to change; pass --status, --header, --body, --body-file, --at or --off-schema")
	}
	return opts, nil
}

// parseWithName parses flags for a command whose name argument normally comes
// first, which would otherwise stop flag parsing. It returns the name.
func parseWithName(fs *flag.FlagSet, args []string) (string, error) {
	var name string
	rest := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		name, rest = args[0], args[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return "", usageError{err}
	}
	extra := fs.Args()
	if name == "" && len(extra) > 0 {
		name, extra = extra[0], extra[1:]
	}
	if len(extra) > 0 {
		hint := ""
		if !strings.Contains(name, " ") && strings.HasPrefix(extra[0], "/") {
			hint = fmt.Sprintf("; quote the request name, such as '%s %s'", name, extra[0])
		}
		return "", newUsageError("unexpected arguments: %s%s", strings.Join(extra, " "), hint)
	}
	return name, nil
}

// validateName checks that a request name looks like "graphql Operation" or
// "METHOD /path".
func validateName(name string) error {
	method, target, ok := strings.Cut(name, " ")
	if !ok || target == "" || strings.ContainsAny(target, " \t") {
		return fmt.Errorf("request name %q must look like \"GET /repos/octo/hello\" or \"graphql IssueByNumber\"", name)
	}
	if method == "graphql" {
		return nil
	}
	if strings.ToUpper(method) != method || strings.Trim(method, "ABCDEFGHIJKLMNOPQRSTUVWXYZ") != "" {
		return fmt.Errorf("request name %q must start with an uppercase HTTP method or \"graphql\"", name)
	}
	if !strings.Contains(target, "/") {
		return fmt.Errorf("request name %q must include a path, such as /repos/octo/hello", name)
	}
	return nil
}

func cmdSet(args []string, stdout, stderr io.Writer) error {
	opts, err := parseSetArgs(args)
	if err != nil {
		return err
	}
	if opts.bodyFile != "" {
		data, err := os.ReadFile(opts.bodyFile)
		if err != nil {
			return err
		}
		opts.body, opts.bodySet = string(data), true
	}

	st, err := openState()
	if err != nil {
		return err
	}
	responses, err := st.loadResponses()
	if err != nil {
		return err
	}
	pos, err := applySet(responses, opts)
	if err != nil {
		return err
	}
	result, err := checkBeforeSave(st, newChecker(stderr), opts.name, &responses[opts.name][pos], opts.offSchema)
	if err != nil {
		return err
	}
	if err := st.saveResponses(responses); err != nil {
		return err
	}
	fmt.Fprintln(stdout, describeResponse(opts.name, responses[opts.name], pos))
	fmt.Fprintln(stdout, result)
	return nil
}

// checkBeforeSave checks a changed response against GitHub's schema before it
// is saved. It returns a line saying what the check did, or an error listing
// the problems. A GraphQL response is checked against the query gh last sent
// for it, so it isn't checked until gh has sent one.
func checkBeforeSave(st *state, chk *checker, name string, resp *response, offSchema bool) (string, error) {
	if offSchema {
		resp.OffSchema = true
		return "Schema check: skipped, saved as off-schema", nil
	}
	resp.OffSchema = false

	method, target, _ := strings.Cut(name, " ")
	e := loggedRequest{Name: name, Method: method, Host: apiHost, Path: target}
	if method == "graphql" {
		if _, err := chk.graphQL(); err != nil {
			return fmt.Sprintf("Schema check: skipped, GraphQL checks are off: %v", err), nil
		}
		query, err := lastQuery(st, name)
		if err != nil {
			return "", err
		}
		if query == "" {
			return fmt.Sprintf("Schema check: not yet, because gh hasn't sent %s; the next run checks it", name), nil
		}
		e = loggedRequest{Name: name, Method: http.MethodPost, Host: apiHost, Path: "/graphql", GraphQL: &graphQLRequest{Query: query}}
	} else {
		if host, path, ok := strings.Cut(target, "/"); ok && host != "" {
			e.Host, e.Path = host, "/"+path
		}
		if e.Host == apiHost {
			if _, err := chk.rest(); err != nil {
				return fmt.Sprintf("Schema check: skipped, REST checks are off: %v", err), nil
			}
		}
	}

	res := chk.checkRequest(e, resp)
	switch {
	case len(res.responseProblems) > 0:
		return "", fmt.Errorf("the response doesn't match GitHub's schema, so it wasn't saved:\n%sFix it, or pass --off-schema to save it anyway",
			indentProblems(res.responseProblems, "  ", 0))
	case res.note != "":
		return "Schema check: skipped, " + res.note, nil
	default:
		return "Schema check: passed", nil
	}
}

// lastQuery returns the query gh most recently sent for a GraphQL request
// name, or "" if it hasn't sent one.
func lastQuery(st *state, name string) (string, error) {
	entries, err := st.readLog()
	if err != nil {
		return "", err
	}
	for _, entrie := range slices.Backward(entries) {
		if entrie.Name == name && entrie.GraphQL != nil {
			return entrie.GraphQL.Query, nil
		}
	}
	return "", nil
}

// applySet changes one stored response and returns its position.
func applySet(responses responseSet, opts setOptions) (int, error) {
	list := responses[opts.name]
	var pos int
	if opts.appendNew || (len(list) == 0 && opts.index <= 1) {
		list = append(list, response{})
		pos = len(list) - 1
	} else {
		pos = max(opts.index, 1) - 1
		if pos >= len(list) {
			return 0, fmt.Errorf("%s has %d response(s); use --append to add another", opts.name, len(list))
		}
	}

	r := list[pos]
	if opts.status != 0 {
		r.Status = opts.status
	}
	if len(opts.headers) > 0 {
		headers := make(map[string]string, len(r.Headers)+len(opts.headers))
		for k, v := range r.Headers {
			headers[http.CanonicalHeaderKey(k)] = v
		}
		for _, h := range opts.headers {
			name, value, _ := strings.Cut(h, ":")
			name, value = http.CanonicalHeaderKey(strings.TrimSpace(name)), strings.TrimSpace(value)
			if value == "" {
				delete(headers, name)
			} else {
				headers[name] = value
			}
		}
		if len(headers) == 0 {
			headers = nil
		}
		r.Headers = headers
	}
	if opts.bodySet {
		body := bytes.TrimSpace([]byte(opts.body))
		if len(body) > 0 && !json.Valid(body) {
			return 0, fmt.Errorf("body is not valid JSON")
		}
		if len(body) == 0 {
			body = nil
		}
		r.Body = body
	}
	if opts.at != "" {
		body, err := setAtPath(r.Body, opts.at, opts.value)
		if err != nil {
			return 0, err
		}
		r.Body = body
	}

	list[pos] = r
	responses[opts.name] = list
	return pos, nil
}

// setAtPath puts a JSON value at a dotted path inside a JSON body. Numeric
// segments index arrays. Missing objects along the way are created.
func setAtPath(body json.RawMessage, path, value string) (json.RawMessage, error) {
	var root any = map[string]any{}
	if len(bytes.TrimSpace(body)) > 0 {
		if err := decodeJSON(body, &root); err != nil {
			return nil, fmt.Errorf("current body is not valid JSON: %w", err)
		}
	}
	var val any
	if err := decodeJSON([]byte(value), &val); err != nil {
		return nil, fmt.Errorf("--json is not valid JSON: %w", err)
	}
	segments := strings.Split(path, ".")
	if slices.Contains(segments, "") {
		return nil, fmt.Errorf("--at %q has an empty segment", path)
	}
	root, err := setPath(root, segments, val, "")
	if err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

func setPath(node any, segments []string, val any, walked string) (any, error) {
	if len(segments) == 0 {
		return val, nil
	}
	seg := segments[0]
	here := seg
	if walked != "" {
		here = walked + "." + seg
	}
	switch n := node.(type) {
	case map[string]any:
		child, err := setPath(n[seg], segments[1:], val, here)
		if err != nil {
			return nil, err
		}
		n[seg] = child
		return n, nil
	case []any:
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(n) {
			return nil, fmt.Errorf("%s: %q is not an index of this %d-item array", where(walked), seg, len(n))
		}
		child, err := setPath(n[i], segments[1:], val, here)
		if err != nil {
			return nil, err
		}
		n[i] = child
		return n, nil
	case nil:
		return setPath(map[string]any{}, segments, val, walked)
	default:
		return nil, fmt.Errorf("%s: cannot set %q inside a %s", where(walked), seg, jsonKind(n))
	}
}

// where names a position in the body for error messages.
func where(walked string) string {
	if walked == "" {
		return "body"
	}
	return walked
}

func decodeJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("unexpected data after the JSON value")
	}
	return nil
}

func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func describeResponse(name string, list []response, pos int) string {
	r := list[pos]
	headers := make([]string, 0, len(r.Headers))
	for k := range r.Headers {
		headers = append(headers, k)
	}
	sort.Strings(headers)
	headerText := "no headers"
	if len(headers) > 0 {
		headerText = "headers " + strings.Join(headers, ", ")
	}
	offSchema := ""
	if r.OffSchema {
		offSchema = ", off-schema"
	}
	return fmt.Sprintf("%s: response %d of %d: status %d, %s, body %d bytes%s",
		name, pos+1, len(list), r.status(), headerText, len(r.Body), offSchema)
}
