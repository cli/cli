package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
)

// checker compares queries and responses with GitHub's published schemas. It
// loads each schema the first time it's needed. A schema that can't be loaded
// turns its checks off and is reported once, instead of failing every request.
type checker struct {
	progress io.Writer

	gqlOnce   sync.Once
	gqlSchema *ast.Schema
	gqlErr    error

	restOnce sync.Once
	restIdx  *restIndex
	restErr  error
}

func newChecker(progress io.Writer) *checker {
	return &checker{progress: progress}
}

func (c *checker) graphQL() (*ast.Schema, error) {
	c.gqlOnce.Do(func() {
		path, err := graphQLSource.locate(c.progress)
		if err != nil {
			c.gqlErr = err
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			c.gqlErr = err
			return
		}
		schema, err := loadGraphQLSchema(data, path)
		if err != nil {
			c.gqlErr = fmt.Errorf("reading %s: %w", path, err)
			return
		}
		c.gqlSchema = schema
	})
	return c.gqlSchema, c.gqlErr
}

func (c *checker) rest() (*restIndex, error) {
	c.restOnce.Do(func() {
		path, err := restSource.locate(c.progress)
		if err != nil {
			c.restErr = err
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			c.restErr = err
			return
		}
		idx, err := loadRESTIndex(data)
		if err != nil {
			c.restErr = fmt.Errorf("reading %s: %w", path, err)
			return
		}
		c.restIdx = idx
	})
	return c.restIdx, c.restErr
}

// preload loads both schemas before gh starts, so a first-time download
// doesn't happen while gh waits for a response.
func (c *checker) preload() {
	_, _ = c.graphQL()
	_, _ = c.rest()
}

// unavailable describes each schema that couldn't be loaded.
func (c *checker) unavailable() []string {
	var notes []string
	if c.gqlErr != nil {
		notes = append(notes, fmt.Sprintf("GraphQL checks are off: %v", c.gqlErr))
	}
	if c.restErr != nil {
		notes = append(notes, fmt.Sprintf("REST checks are off: %v", c.restErr))
	}
	return notes
}

// checkResult is what checking one request and its response found. note says
// why a response wasn't checked, when it wasn't.
type checkResult struct {
	queryProblems    []string
	responseProblems []string
	note             string
}

// checkRequest checks a request gh made and the response ghmock gave. resp is
// nil when no response was stored. Off-schema responses aren't checked, but
// gh's query still is.
func (c *checker) checkRequest(e loggedRequest, resp *response) checkResult {
	return c.check(e, resp, true)
}

// Response-only saves reconstruct query text, not the original variable payload.
func (c *checker) checkResponse(e loggedRequest, resp *response) checkResult {
	return c.check(e, resp, false)
}

func (c *checker) check(e loggedRequest, resp *response, validateVariables bool) checkResult {
	var res checkResult
	if e.GraphQL != nil {
		schema, err := c.graphQL()
		if err != nil {
			return res
		}
		operation := e.GraphQL.Operation
		if !validateVariables && operation == "" && strings.HasPrefix(e.Name, "graphql ") {
			operation = strings.TrimPrefix(e.Name, "graphql ")
			if operation == "<anonymous>" {
				operation = ""
			}
		}
		op, problems := checkGraphQLQuery(schema, e.GraphQL.Query, operation)
		res.queryProblems = problems
		var variables map[string]any
		if validateVariables {
			variables = map[string]any{}
			var variablesErr error
			if len(e.GraphQL.Variables) > 0 {
				variablesErr = json.Unmarshal(e.GraphQL.Variables, &variables)
			}
			if variablesErr == nil && op != nil && len(problems) == 0 {
				var coerced map[string]any
				coerced, variablesErr = validator.VariableValues(schema, op, variables)
				if variablesErr == nil {
					variables = coerced
				}
			}
			if variablesErr != nil {
				res.queryProblems = append(res.queryProblems, "variables: "+variablesErr.Error())
			}
		}
		if resp == nil || resp.OffSchema {
			return res
		}
		switch {
		case op == nil:
			res.note = "gh's query couldn't be read"
		case resp.status() != 200:
			res.note = fmt.Sprintf("status %d has no GraphQL result to check", resp.status())
		default:
			res.responseProblems = checkGraphQLResponse(schema, op, resp.Body, variables)
		}
		return res
	}

	if resp == nil || resp.OffSchema {
		return res
	}
	if e.Host != apiHost {
		res.note = "not a REST API host"
		return res
	}
	idx, err := c.rest()
	if err != nil {
		return res
	}
	res.responseProblems, res.note = idx.check(e.Method, e.Path, resp.status(), resp.Body)
	return res
}

// formatGraphQLError turns a parser or validator error into a short line.
func formatGraphQLError(message string, line, column int) string {
	if line == 0 {
		return message
	}
	return fmt.Sprintf("line %d, column %d: %s", line, column, message)
}

// decodeBody decodes a JSON body, keeping numbers exact.
func decodeBody(body []byte) (any, error) {
	var v any
	if err := decodeJSON(bytes.TrimSpace(body), &v); err != nil {
		return nil, err
	}
	return v, nil
}

// describeJSON names the kind of a decoded JSON value for messages.
func describeJSON(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case string:
		return "a string"
	case json.Number, float64:
		return "a number"
	case bool:
		return "a boolean"
	case []any:
		return "a list"
	case map[string]any:
		return "an object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

// listNames joins names for messages, shortening long lists.
func listNames(names []string, limit int) string {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	if len(sorted) > limit {
		return strings.Join(sorted[:limit], ", ") + fmt.Sprintf(" and %d more", len(sorted)-limit)
	}
	return strings.Join(sorted, ", ")
}

// dedupe drops repeated problems, keeping the first of each.
func dedupe(problems []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range problems {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}
