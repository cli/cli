package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/vektah/gqlparser/v2/ast"
)

// schemaOptions are the parsed flags of ghmock schema. A name with a space is
// a REST request, such as "GET /repos/octo/hello". Anything else is a GraphQL
// type, such as Issue, or a field, such as Issue.title.
type schemaOptions struct {
	name   string
	at     string
	status int
}

func (o schemaOptions) isREST() bool {
	return strings.Contains(o.name, " ")
}

func parseSchemaArgs(args []string) (schemaOptions, error) {
	var opts schemaOptions
	fs := flag.NewFlagSet("schema", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.at, "at", "", "dotted path inside a REST body, such as user.login")
	fs.IntVar(&opts.status, "status", 0, "which documented REST status to describe")
	name, err := parseWithName(fs, args)
	if err != nil {
		return schemaOptions{}, err
	}
	opts.name = name

	switch {
	case opts.name == "":
		return schemaOptions{}, newUsageError("missing name, such as Issue, Issue.title or \"GET /repos/octo/hello\"")
	case strings.HasPrefix(opts.name, "graphql "):
		return schemaOptions{}, newUsageError("look up GraphQL types and fields by name, such as Issue or Issue.title")
	case !opts.isREST() && (opts.at != "" || opts.status != 0):
		return schemaOptions{}, newUsageError("--at and --status are for REST requests; look up a GraphQL field as Type.field")
	case opts.status != 0 && (opts.status < 100 || opts.status > 599):
		return schemaOptions{}, newUsageError("--status must be between 100 and 599")
	}
	if opts.isREST() {
		if err := validateName(opts.name); err != nil {
			return schemaOptions{}, usageError{err}
		}
	}
	return opts, nil
}

func cmdSchema(args []string, stdout, stderr io.Writer) error {
	opts, err := parseSchemaArgs(args)
	if err != nil {
		return err
	}
	chk := newChecker(stderr)
	if opts.isREST() {
		idx, err := chk.rest()
		if err != nil {
			return err
		}
		return describeREST(stdout, idx, opts)
	}
	schema, err := chk.graphQL()
	if err != nil {
		return err
	}
	return describeGraphQL(stdout, schema, opts.name)
}

// describeGraphQL writes what GitHub's GraphQL schema says about a type or a
// field.
func describeGraphQL(w io.Writer, schema *ast.Schema, name string) error {
	typeName, fieldName, isField := strings.Cut(name, ".")
	def := schema.Types[typeName]
	if def == nil || strings.HasPrefix(typeName, "__") {
		return notFound("GitHub's GraphQL schema has no type named "+typeName, typeName, graphQLTypeNames(schema))
	}
	if !isField {
		writeGraphQLType(w, schema, def)
		return nil
	}
	switch def.Kind {
	case ast.Enum, ast.Union, ast.Scalar:
		return fmt.Errorf("%s is %s, so it has no fields; ghmock schema %s describes it", def.Name, kindLabel(def.Kind), def.Name)
	}
	field := def.Fields.ForName(fieldName)
	if field == nil || strings.HasPrefix(fieldName, "__") {
		return notFound(fmt.Sprintf("%s has no field named %s", def.Name, fieldName), fieldName, graphQLFieldNames(def))
	}
	writeGraphQLField(w, schema, def, field)
	return nil
}

func writeGraphQLType(w io.Writer, schema *ast.Schema, def *ast.Definition) {
	fmt.Fprintf(w, "%s: %s\n", def.Name, kindLabel(def.Kind))
	writeDescription(w, def.Description)
	switch def.Kind {
	case ast.Object, ast.Interface:
		if len(def.Interfaces) > 0 {
			fmt.Fprintf(w, "Implements: %s\n", strings.Join(sortedCopy(def.Interfaces), ", "))
		}
		if def.Kind == ast.Interface {
			fmt.Fprintf(w, "Implemented by: %s\n", strings.Join(sortedCopy(possibleTypeNames(schema, def)), ", "))
		}
		fmt.Fprintln(w, "Fields (a type ending in ! is never null):")
		writeGraphQLFields(w, def.Fields)
	case ast.InputObject:
		fmt.Fprintln(w, "Fields (a type ending in ! is required):")
		writeGraphQLFields(w, def.Fields)
	case ast.Union:
		fmt.Fprintf(w, "Can be: %s\n", strings.Join(sortedCopy(possibleTypeNames(schema, def)), ", "))
	case ast.Enum:
		fmt.Fprintln(w, "Values:")
		width := 0
		for _, v := range def.EnumValues {
			width = max(width, len(v.Name))
		}
		for _, v := range def.EnumValues {
			line := "  " + v.Name
			if d := firstLine(v.Description); d != "" {
				line += strings.Repeat(" ", width-len(v.Name)+2) + d
			}
			if v.Directives.ForName("deprecated") != nil {
				line += " (deprecated)"
			}
			fmt.Fprintln(w, line)
		}
	case ast.Scalar:
		fmt.Fprintf(w, "Sent as %s.\n", scalarJSON(def.Name))
	}
}

func writeGraphQLFields(w io.Writer, fields ast.FieldList) {
	for _, f := range fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		line := fmt.Sprintf("  %s: %s", f.Name, f.Type.String())
		if f.Directives.ForName("deprecated") != nil {
			line += " (deprecated)"
		}
		fmt.Fprintln(w, line)
	}
}

func writeGraphQLField(w io.Writer, schema *ast.Schema, def *ast.Definition, f *ast.FieldDefinition) {
	input := def.Kind == ast.InputObject
	fmt.Fprintf(w, "%s.%s: %s\n", def.Name, f.Name, f.Type.String())
	fmt.Fprintln(w, nullability(f.Type, input))
	writeDescription(w, f.Description)
	if d := f.Directives.ForName("deprecated"); d != nil {
		reason := "No longer supported"
		if arg := d.Arguments.ForName("reason"); arg != nil && arg.Value != nil {
			reason = arg.Value.Raw
		}
		fmt.Fprintf(w, "Deprecated: %s\n", firstLine(reason))
	}
	if len(f.Arguments) > 0 {
		fmt.Fprintln(w, "Arguments (a type ending in ! is required):")
		for _, a := range f.Arguments {
			line := fmt.Sprintf("  %s: %s", a.Name, a.Type.String())
			if a.DefaultValue != nil {
				line += " = " + a.DefaultValue.String()
			}
			fmt.Fprintln(w, line)
		}
	}
	if t := schema.Types[f.Type.Name()]; t != nil && t.Kind != ast.Scalar {
		fmt.Fprintf(w, "See also: ghmock schema %s\n", t.Name)
	}
}

// nullability says whether a field's value can be null, or for input fields,
// whether the field is required.
func nullability(t *ast.Type, input bool) string {
	if input {
		if t.NonNull {
			return "Required."
		}
		return "Optional."
	}
	s := "Can be null."
	if t.NonNull {
		s = "Never null."
	}
	if t.Elem != nil {
		if t.Elem.NonNull {
			s += " Items are never null."
		} else {
			s += " Items can be null."
		}
	}
	return s
}

func kindLabel(k ast.DefinitionKind) string {
	switch k {
	case ast.Object:
		return "an object"
	case ast.Interface:
		return "an interface"
	case ast.Union:
		return "a union"
	case ast.Enum:
		return "an enum"
	case ast.InputObject:
		return "an input object"
	default:
		return "a scalar"
	}
}

// scalarJSON says how a scalar appears in a JSON response.
func scalarJSON(name string) string {
	switch {
	case stringScalars[name]:
		return "a JSON string"
	case name == "Int":
		return "a 32-bit whole number"
	case name == "Float":
		return "a JSON number"
	case name == "Boolean":
		return "true or false"
	default:
		return "any JSON value"
	}
}

func graphQLTypeNames(schema *ast.Schema) []string {
	var names []string
	for name := range schema.Types {
		if !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	return names
}

func graphQLFieldNames(def *ast.Definition) []string {
	var names []string
	for _, f := range def.Fields {
		if !strings.HasPrefix(f.Name, "__") {
			names = append(names, f.Name)
		}
	}
	return names
}

// describeREST writes what GitHub's REST description says about a request and
// the body of one of its responses.
func describeREST(w io.Writer, idx *restIndex, opts schemaOptions) error {
	method, target, _ := strings.Cut(opts.name, " ")
	if !strings.HasPrefix(target, "/") {
		return fmt.Errorf("%s isn't on the REST API host, so GitHub's REST description doesn't cover it", target)
	}
	path, _, _ := strings.Cut(target, "?")
	route := idx.match(path)
	if route == nil {
		return fmt.Errorf("GitHub's REST description has no path matching %s", path)
	}
	op := route.item.GetOperation(method)
	if op == nil {
		return fmt.Errorf("GitHub documents no %s for %s; it documents %s", method, route.template, strings.Join(routeMethods(route.item), ", "))
	}

	fmt.Fprintf(w, "%s %s\n", method, route.template)
	switch {
	case op.Summary != "" && op.OperationID != "":
		fmt.Fprintf(w, "%s (%s)\n", op.Summary, op.OperationID)
	case op.Summary != "":
		fmt.Fprintln(w, op.Summary)
	case op.OperationID != "":
		fmt.Fprintln(w, op.OperationID)
	}
	if op.Deprecated {
		fmt.Fprintln(w, "Deprecated.")
	}
	statuses := documentedStatuses(op.Responses)
	if len(statuses) == 0 {
		fmt.Fprintln(w, "Documented statuses: none")
		return nil
	}
	fmt.Fprintf(w, "Documented statuses: %s\n", strings.Join(statuses, ", "))

	status, ref := pickStatus(op.Responses, opts.status, statuses)
	if ref == nil {
		return fmt.Errorf("GitHub doesn't document status %d for %s %s; it documents %s",
			opts.status, method, route.template, strings.Join(statuses, ", "))
	}
	var media *openapi3.MediaType
	if ref.Value != nil {
		media = jsonMedia(ref.Value.Content)
	}
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		if opts.at != "" {
			return fmt.Errorf("status %s has no JSON body", status)
		}
		fmt.Fprintf(w, "Body for %s: none\n", status)
		return nil
	}

	schema, err := schemaAt(media.Schema, opts.at)
	if err != nil {
		return err
	}
	label := "Body"
	if opts.at != "" {
		label = opts.at + ", in the body"
	}
	fmt.Fprintf(w, "%s for %s: %s\n", label, status, describeSchemaType(schema))
	if schema.Value != nil {
		writeDescription(w, schema.Value.Description)
		writeProperties(w, schema.Value)
	}
	return nil
}

func routeMethods(item *openapi3.PathItem) []string {
	var methods []string
	for m := range item.Operations() {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	return methods
}

// documentedStatuses lists a response's status codes in numeric order, with
// "default" last.
func documentedStatuses(responses *openapi3.Responses) []string {
	var statuses []string
	if responses == nil {
		return statuses
	}
	for code := range responses.Map() {
		statuses = append(statuses, code)
	}
	sort.Slice(statuses, func(i, j int) bool {
		a, b := statuses[i], statuses[j]
		if (a == "default") != (b == "default") {
			return b == "default"
		}
		return a < b
	})
	return statuses
}

// pickStatus finds the response to describe: the one asked for, or else the
// first success. It returns a nil response if there's no such status.
func pickStatus(responses *openapi3.Responses, want int, statuses []string) (string, *openapi3.ResponseRef) {
	if want != 0 {
		return strconv.Itoa(want), responses.Status(want)
	}
	if len(statuses) == 0 {
		return "", nil
	}
	pick := statuses[0]
	for _, s := range statuses {
		if strings.HasPrefix(s, "2") {
			pick = s
			break
		}
	}
	return pick, responses.Value(pick)
}

// schemaAt follows a dotted path, as used by ghmock set --at, into a schema.
func schemaAt(ref *openapi3.SchemaRef, at string) (*openapi3.SchemaRef, error) {
	if at == "" {
		return ref, nil
	}
	walked := ""
	for seg := range strings.SplitSeq(at, ".") {
		if seg == "" {
			return nil, newUsageError("--at %q has an empty segment", at)
		}
		next := childSchema(ref, seg)
		if next == nil {
			return nil, missingChild(ref, walked, seg)
		}
		ref = next
		if walked != "" {
			walked += "."
		}
		walked += seg
	}
	return ref, nil
}

// childSchema finds the schema for one step into a value: an array index or
// an object key. It looks through allOf, oneOf and anyOf.
func childSchema(ref *openapi3.SchemaRef, seg string) *openapi3.SchemaRef {
	if ref == nil || ref.Value == nil {
		return nil
	}
	s := ref.Value
	if s.Items != nil {
		if _, err := strconv.Atoi(seg); err == nil {
			return s.Items
		}
	}
	if p := s.Properties[seg]; p != nil {
		return p
	}
	for _, group := range []openapi3.SchemaRefs{s.AllOf, s.OneOf, s.AnyOf} {
		for _, alt := range group {
			if c := childSchema(alt, seg); c != nil {
				return c
			}
		}
	}
	return s.AdditionalProperties.Schema
}

func missingChild(ref *openapi3.SchemaRef, walked, seg string) error {
	place := where(walked)
	if ref.Value != nil && ref.Value.Items != nil {
		return fmt.Errorf("%s is a list; put an index first, such as %s.0.%s", place, place, seg)
	}
	names := propertyNames(ref.Value)
	if len(names) == 0 {
		return fmt.Errorf("%s (%s) has nothing inside it", place, describeSchemaType(ref))
	}
	return notFound(fmt.Sprintf("%s has no property named %s", place, seg), seg, names)
}

// describeSchemaType names the JSON a schema allows, such as "string or null"
// or "object (simple-user)".
func describeSchemaType(ref *openapi3.SchemaRef) string {
	if ref == nil || ref.Value == nil {
		return "any JSON value"
	}
	s := ref.Value
	var names []string
	nullable := s.Nullable
	switch {
	case len(s.OneOf) > 0 || len(s.AnyOf) > 0:
		for _, alt := range append(append(openapi3.SchemaRefs{}, s.OneOf...), s.AnyOf...) {
			names = append(names, describeSchemaType(alt))
		}
	default:
		types := s.Type.Slice()
		if len(types) == 0 && (len(s.Properties) > 0 || len(s.AllOf) > 0) {
			types = []string{"object"}
		}
		for _, t := range types {
			if t == "null" {
				nullable = true
				continue
			}
			names = append(names, schemaTypeName(t, ref))
		}
	}
	if len(names) == 0 {
		names = []string{"any JSON value"}
	}
	desc := strings.Join(names, " or ")
	if nullable && !strings.HasSuffix(desc, " or null") {
		desc += " or null"
	}
	if len(s.Enum) > 0 {
		values := make([]string, len(s.Enum))
		for i, v := range s.Enum {
			values[i] = jsonText(v)
		}
		desc += ", one of " + strings.Join(values, ", ")
	}
	return desc
}

func schemaTypeName(t string, ref *openapi3.SchemaRef) string {
	s := ref.Value
	switch t {
	case "array":
		if s.Items == nil {
			return "list"
		}
		item := describeSchemaType(s.Items)
		if strings.Contains(item, " or ") || strings.Contains(item, ", one of ") {
			item = "(" + item + ")"
		}
		return "list of " + item
	case "object":
		if name := refName(ref); name != "" {
			return "object (" + name + ")"
		}
		return "object"
	case "string":
		if s.Format != "" {
			return "string (" + s.Format + ")"
		}
		return "string"
	default:
		return t
	}
}

// refName is the last part of a schema's $ref, such as simple-user.
func refName(ref *openapi3.SchemaRef) string {
	if i := strings.LastIndex(ref.Ref, "/"); i >= 0 {
		return ref.Ref[i+1:]
	}
	return ref.Ref
}

// writeProperties lists an object's properties, or those of each item in a
// list of objects, with the ones a response must include first.
func writeProperties(w io.Writer, s *openapi3.Schema) {
	suffix := ":"
	if s.Items != nil && s.Items.Value != nil {
		s = s.Items.Value
		suffix = " of each item:"
	}
	props, required := mergedProperties(s)
	var must, may []string
	for name := range props {
		if required[name] {
			must = append(must, name)
		} else {
			may = append(may, name)
		}
	}
	for _, group := range []struct {
		label string
		names []string
	}{{"Required properties", must}, {"Optional properties", may}} {
		if len(group.names) == 0 {
			continue
		}
		sort.Strings(group.names)
		fmt.Fprintln(w, group.label+suffix)
		for _, name := range group.names {
			fmt.Fprintf(w, "  %s: %s\n", name, describeSchemaType(props[name]))
		}
	}
}

// mergedProperties collects an object's properties and required names,
// including those from allOf.
func mergedProperties(s *openapi3.Schema) (map[string]*openapi3.SchemaRef, map[string]bool) {
	props := map[string]*openapi3.SchemaRef{}
	required := map[string]bool{}
	var add func(*openapi3.Schema)
	add = func(s *openapi3.Schema) {
		if s == nil {
			return
		}
		for name, p := range s.Properties {
			if _, ok := props[name]; !ok {
				props[name] = p
			}
		}
		for _, r := range s.Required {
			required[r] = true
		}
		for _, a := range s.AllOf {
			add(a.Value)
		}
	}
	add(s)
	return props, required
}

// propertyNames lists every key a schema allows, including in oneOf and anyOf
// alternatives.
func propertyNames(s *openapi3.Schema) []string {
	if s == nil {
		return nil
	}
	seen := map[string]bool{}
	var names []string
	var add func(*openapi3.Schema)
	add = func(s *openapi3.Schema) {
		if s == nil {
			return
		}
		for name := range s.Properties {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
		for _, group := range []openapi3.SchemaRefs{s.AllOf, s.OneOf, s.AnyOf} {
			for _, alt := range group {
				add(alt.Value)
			}
		}
	}
	add(s)
	return names
}

func writeDescription(w io.Writer, text string) {
	if text = strings.TrimSpace(text); text != "" {
		fmt.Fprintln(w, text)
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return strings.TrimSpace(line)
}

func sortedCopy(names []string) []string {
	out := append([]string(nil), names...)
	sort.Strings(out)
	return out
}

// notFound is an error for a name that doesn't exist. It suggests close
// matches.
func notFound(msg, name string, candidates []string) error {
	if matches := closeMatches(name, candidates, 5); len(matches) > 0 {
		return fmt.Errorf("%s; did you mean %s?", msg, strings.Join(matches, ", "))
	}
	return errors.New(msg)
}

// closeMatches finds candidates that differ from name only in case, by a typo
// or two, or that contain it.
func closeMatches(name string, candidates []string, limit int) []string {
	type match struct {
		name string
		rank int
	}
	lower := strings.ToLower(name)
	var matches []match
	for _, c := range candidates {
		lc := strings.ToLower(c)
		switch {
		case lc == lower:
			matches = append(matches, match{c, 0})
		case editDistance(lower, lc) <= 2:
			matches = append(matches, match{c, 1})
		case len(lower) >= 3 && strings.Contains(lc, lower):
			matches = append(matches, match{c, 2})
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if len(a.name) != len(b.name) {
			return len(a.name) < len(b.name)
		}
		return a.name < b.name
	})
	var out []string
	for i := 0; i < len(matches) && i < limit; i++ {
		out = append(out, matches[i].name)
	}
	return out
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// cmdSchemas runs ghmock schemas update, which downloads both schema files
// again.
func cmdSchemas(args []string, stdout, stderr io.Writer) error {
	switch {
	case len(args) == 0:
		return newUsageError("missing subcommand; use ghmock schemas update")
	case args[0] != "update":
		return newUsageError("unknown subcommand %q; use ghmock schemas update", args[0])
	case len(args) > 1:
		return newUsageError("unexpected arguments: %s", strings.Join(args[1:], " "))
	}
	dir, err := schemaCacheDir()
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range []schemaSource{graphQLSource, restSource} {
		p, err := s.download(dir, stderr)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		size := ""
		if info, err := os.Stat(p); err == nil {
			size = " (" + formatSize(info.Size()) + ")"
		}
		fmt.Fprintf(stdout, "%s: %s%s\n", s.label, p, size)
		if env := os.Getenv(s.envVar); env != "" {
			fmt.Fprintf(stdout, "  %s is set, so ghmock uses %s instead\n", s.envVar, env)
		}
	}
	return errors.Join(errs...)
}

func formatSize(n int64) string {
	if n < 1<<20 {
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
}
