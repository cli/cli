package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// restIndex finds the operation GitHub documents for a REST request.
type restIndex struct {
	routes []restRoute
}

type restRoute struct {
	template string
	segments []routeSegment
	item     *openapi3.PathItem
}

// routeSegment is one segment of a path template. A plain segment has only
// literal, a whole-segment parameter such as {owner} has param, and a segment
// that mixes text with parameters has pattern.
type routeSegment struct {
	literal string
	param   string
	pattern *regexp.Regexp
}

var paramPattern = regexp.MustCompile(`\{[^{}/]+\}`)

// slashParams are path parameters that can hold slashes, such as file paths and
// branch names. When one ends a template, it takes the rest of the path.
var slashParams = map[string]bool{"path": true, "ref": true, "branch": true, "basehead": true, "tag": true}

func loadRESTIndex(data []byte) (*restIndex, error) {
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, err
	}
	if doc.Paths == nil || doc.Paths.Len() == 0 {
		return nil, errors.New("the description has no paths")
	}
	idx := &restIndex{}
	for tmpl, item := range doc.Paths.Map() {
		idx.routes = append(idx.routes, restRoute{template: tmpl, segments: parseTemplate(tmpl), item: item})
	}
	sort.Slice(idx.routes, func(i, j int) bool { return idx.routes[i].template < idx.routes[j].template })
	return idx, nil
}

func parseTemplate(tmpl string) []routeSegment {
	parts := splitPath(tmpl)
	segs := make([]routeSegment, len(parts))
	for i, p := range parts {
		switch {
		case !strings.Contains(p, "{"):
			segs[i] = routeSegment{literal: p}
		case paramPattern.FindString(p) == p:
			segs[i] = routeSegment{param: p[1 : len(p)-1]}
		default:
			var expr strings.Builder
			expr.WriteString("^")
			last := 0
			for _, loc := range paramPattern.FindAllStringIndex(p, -1) {
				expr.WriteString(regexp.QuoteMeta(p[last:loc[0]]))
				expr.WriteString("[^/]+")
				last = loc[1]
			}
			expr.WriteString(regexp.QuoteMeta(p[last:]))
			expr.WriteString("$")
			segs[i] = routeSegment{pattern: regexp.MustCompile(expr.String())}
		}
	}
	return segs
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// match finds the documented path template for a request. Literal segments
// beat parameters, so /repos/{owner}/{repo}/issues beats
// /repos/{owner}/{repo}/{x} if both existed. Between templates that tie, such
// as /orgs/{org}/attestations/{attestation_id} and
// /orgs/{org}/attestations/{subject_digest}, the one that documents the
// method wins.
func (idx *restIndex) match(method, path string) *restRoute {
	parts := splitPath(path)
	var best []*restRoute
	bestScore := -1
	for i := range idx.routes {
		score, ok := idx.routes[i].score(parts)
		switch {
		case !ok || score < bestScore:
		case score > bestScore:
			best, bestScore = []*restRoute{&idx.routes[i]}, score
		default:
			best = append(best, &idx.routes[i])
		}
	}
	for _, r := range best {
		if r.item != nil && r.item.GetOperation(method) != nil {
			return r
		}
	}
	if len(best) == 0 {
		return nil
	}
	return best[0]
}

func (r *restRoute) score(parts []string) (int, bool) {
	segs := r.segments
	switch {
	case len(parts) < len(segs):
		return 0, false
	case len(parts) > len(segs):
		if len(segs) == 0 || !slashParams[segs[len(segs)-1].param] {
			return 0, false
		}
	}
	score := 0
	for i, s := range segs {
		part := parts[i]
		switch {
		case part == "":
			return 0, false
		case s.param != "":
		case s.pattern != nil:
			if !s.pattern.MatchString(part) {
				return 0, false
			}
			score++
		default:
			if s.literal != part {
				return 0, false
			}
			score += 2
		}
	}
	// An exact segment count beats a final parameter that takes the rest.
	score *= 2
	if len(parts) == len(segs) {
		score++
	}
	return score, true
}

// check compares a response with GitHub's description of it. It returns the
// problems it found, or a note saying why it couldn't check.
func (idx *restIndex) check(method, path string, status int, body []byte) ([]string, string) {
	route := idx.match(method, path)
	if route == nil {
		return nil, "not in GitHub's REST description"
	}
	op := route.item.GetOperation(method)
	if op == nil {
		return nil, fmt.Sprintf("GitHub documents no %s for %s", method, route.template)
	}
	ref := op.Responses.Status(status)
	if ref == nil || ref.Value == nil {
		return nil, fmt.Sprintf("GitHub doesn't document status %d for %s %s", status, method, route.template)
	}

	trimmed := bytes.TrimSpace(body)
	media := jsonMedia(ref.Value.Content)
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		if len(trimmed) > 0 {
			return nil, fmt.Sprintf("GitHub documents no JSON body for status %d", status)
		}
		return nil, ""
	}
	if len(trimmed) == 0 {
		return []string{fmt.Sprintf("body: missing, but GitHub documents a JSON body for status %d", status)}, ""
	}
	var v any
	if err := json.Unmarshal(trimmed, &v); err != nil {
		return []string{fmt.Sprintf("body: not valid JSON: %v", err)}, ""
	}
	err := media.Schema.Value.VisitJSON(v, openapi3.MultiErrors(), openapi3.VisitAsResponse())
	return restProblems(err), ""
}

// jsonMedia picks the JSON content of a documented response.
func jsonMedia(content openapi3.Content) *openapi3.MediaType {
	if m := content.Get("application/json"); m != nil {
		return m
	}
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(k, "json") {
			return content[k]
		}
	}
	return nil
}

// restProblems turns schema validation errors into short lines that use the
// same dotted paths as ghmock set --at. An allOf failure is replaced by the
// failures inside it. oneOf and anyOf failures aren't, because those list
// every alternative that didn't match.
func restProblems(err error) []string {
	var out []string
	var walk func(e error, prefix []string)
	walk = func(e error, prefix []string) {
		switch e := e.(type) {
		case nil:
		case openapi3.MultiError:
			for _, inner := range e {
				walk(inner, prefix)
			}
		case *openapi3.SchemaError:
			// GitHub's descriptions use oneOf where the alternatives overlap,
			// so real responses can match more than one. That's fine.
			if e.SchemaField == "oneOf" && strings.HasPrefix(e.Reason, "value matches more than one schema") {
				return
			}
			pointer := append(append([]string(nil), prefix...), e.JSONPointer()...)
			var inner openapi3.MultiError
			if e.SchemaField == "allOf" && errors.As(e.Origin, &inner) {
				walk(inner, pointer)
				return
			}
			path := strings.Join(pointer, ".")
			if path == "" {
				path = "body"
			}
			reason := e.Reason
			if strings.HasPrefix(reason, "property ") && strings.HasSuffix(reason, " is missing") {
				reason = "missing, but GitHub's description requires it"
			}
			out = append(out, path+": "+reason)
		default:
			line, _, _ := strings.Cut(e.Error(), "\n")
			out = append(out, "body: "+line)
		}
	}
	walk(err, nil)
	return dedupe(out)
}
