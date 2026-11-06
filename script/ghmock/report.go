package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

const maxShownBody = 300

// printReport writes what each binary did. With more than one binary, it then
// lists how each one differs from the first.
func printReport(w io.Writer, opts runOptions, results []runResult) {
	command := "$ gh " + strings.Join(opts.args, " ")
	if len(results) == 1 {
		fmt.Fprintf(w, "%s - %s\n", results[0].version, results[0].path)
		printResult(w, command, results[0])
		return
	}

	for i, res := range results {
		suffix := ""
		if i == 0 {
			suffix = " (baseline)"
		}
		fmt.Fprintf(w, "=== [%d] %s - %s%s\n", i+1, res.version, res.path, suffix)
		printResult(w, command, res)
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "=== differences from [1]")
	for i := 1; i < len(results); i++ {
		printDifferences(w, results[0], results[i], i+1)
	}
}

func printResult(w io.Writer, command string, res runResult) {
	fmt.Fprintln(w, command)
	if res.timedOut {
		fmt.Fprintln(w, "exit code: none, timed out")
	} else {
		fmt.Fprintf(w, "exit code: %d\n", res.exitCode)
	}
	printOutput(w, "stdout", res.stdout)
	printOutput(w, "stderr", res.stderr)

	fmt.Fprintf(w, "--- requests (%d)\n", len(res.requests))
	for _, e := range res.requests {
		fmt.Fprintln(w, formatRequest(e))
	}
	if missing := missingNames(res.requests); len(missing) > 0 {
		fmt.Fprintln(w, "--- no response for")
		for _, name := range missing {
			fmt.Fprintln(w, name)
		}
	}
}

func printOutput(w io.Writer, label, text string) {
	fmt.Fprintf(w, "--- %s\n", label)
	if text == "" {
		fmt.Fprintln(w, "(empty)")
		return
	}
	fmt.Fprint(w, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
}

// displayName names a request in reports, including blocked ones.
func displayName(e loggedRequest) string {
	if e.Name != "" {
		return e.Name
	}
	return e.Method + " " + e.Host + e.Path
}

func formatRequest(e loggedRequest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d. %s", e.Seq, displayName(e))
	if e.RawQuery != "" {
		b.WriteString("?" + e.RawQuery)
	}
	if e.GraphQL != nil && len(e.GraphQL.Variables) > 0 {
		b.WriteString(" " + truncate(compactJSON(e.GraphQL.Variables), maxShownBody))
	}
	if e.Body != "" {
		b.WriteString(" body " + truncate(compactJSON([]byte(e.Body)), maxShownBody))
	}
	switch {
	case e.Blocked:
		b.WriteString(" -> blocked")
	case e.Matched:
		fmt.Fprintf(&b, " -> %d, response %d of %d", e.Status, e.Response, e.Responses)
	default:
		fmt.Fprintf(&b, " -> %d, no response stored", e.Status)
	}
	return b.String()
}

// missingNames lists, once each and in order, requests that had no response.
func missingNames(requests []loggedRequest) []string {
	seen := map[string]bool{}
	var names []string
	for _, e := range requests {
		if e.Matched || e.Blocked || seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		names = append(names, e.Name)
	}
	return names
}

func printDifferences(w io.Writer, base, other runResult, n int) {
	fmt.Fprintf(w, "[%d] %s:\n", n, other.version)
	same := true

	if base.exitCode != other.exitCode || base.timedOut != other.timedOut {
		same = false
		fmt.Fprintf(w, "  exit code: %s -> %s\n", exitLabel(base), exitLabel(other))
	}
	for _, out := range []struct{ label, a, b string }{
		{"stdout", base.stdout, other.stdout},
		{"stderr", base.stderr, other.stderr},
	} {
		if out.a == out.b {
			continue
		}
		same = false
		fmt.Fprintf(w, "  %s:\n", out.label)
		for _, line := range lineDiff(out.a, out.b) {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}

	baseCounts, otherCounts := countNames(base.requests), countNames(other.requests)
	for _, name := range unionNames(base.requests, other.requests) {
		if baseCounts[name] != otherCounts[name] {
			same = false
			fmt.Fprintf(w, "  %s: %d in [1], %d in [%d]\n", name, baseCounts[name], otherCounts[name], n)
		}
	}

	for _, name := range unionNames(base.requests, other.requests) {
		a, b := requestPayloads(base.requests, name), requestPayloads(other.requests, name)
		if len(a) == 0 || len(b) == 0 || slices.Equal(a, b) {
			continue
		}
		same = false
		fmt.Fprintf(w, "  %s sent different data:\n", name)
		fmt.Fprintf(w, "    [1] %s\n", truncate(formatPayloads(a), maxShownBody))
		fmt.Fprintf(w, "    [%d] %s\n", n, truncate(formatPayloads(b), maxShownBody))
	}

	if same {
		fmt.Fprintln(w, "  same exit code, output and requests")
	}
}

func exitLabel(res runResult) string {
	if res.timedOut {
		return "timed out"
	}
	return fmt.Sprint(res.exitCode)
}

func countNames(requests []loggedRequest) map[string]int {
	counts := map[string]int{}
	for _, e := range requests {
		counts[displayName(e)]++
	}
	return counts
}

// unionNames lists request names from both runs, in the order first seen.
func unionNames(a, b []loggedRequest) []string {
	seen := map[string]bool{}
	var names []string
	for _, list := range [][]loggedRequest{a, b} {
		for _, e := range list {
			name := displayName(e)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names
}

type requestPayload struct {
	rawQuery, body, query, variables string
	graphQL                          bool
}

// requestPayloads preserves field boundaries when comparing request data.
func requestPayloads(requests []loggedRequest, name string) []requestPayload {
	var payloads []requestPayload
	for _, e := range requests {
		if displayName(e) != name {
			continue
		}
		p := requestPayload{rawQuery: e.RawQuery}
		if e.GraphQL != nil {
			p.graphQL = true
			p.query = e.GraphQL.Query
			p.variables = compactJSON(e.GraphQL.Variables)
		} else {
			p.body = compactJSON([]byte(e.Body))
		}
		payloads = append(payloads, p)
	}
	return payloads
}

func formatPayloads(payloads []requestPayload) string {
	var formatted []string
	for _, p := range payloads {
		var parts []string
		if p.rawQuery != "" {
			parts = append(parts, "?"+p.rawQuery)
		}
		if p.graphQL {
			if p.query != "" {
				parts = append(parts, fmt.Sprintf("query %q", p.query))
			}
			if p.variables != "" {
				parts = append(parts, p.variables)
			}
		} else if p.body != "" {
			if p.rawQuery != "" || !json.Valid([]byte(p.body)) {
				parts = append(parts, fmt.Sprintf("body %q", p.body))
			} else {
				parts = append(parts, p.body)
			}
		}
		formatted = append(formatted, strings.Join(parts, " "))
	}
	return strings.Join(formatted, " | ")
}

// lineDiff lists the lines removed from a ("- ") and added in b ("+ ").
func lineDiff(a, b string) []string {
	al, bl := splitLines(a), splitLines(b)
	n, m := len(al), len(bl)
	if n*m > 4_000_000 {
		return []string{"(too long to compare line by line)"}
	}
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case al[i] == bl[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, "- "+al[i])
			i++
		default:
			out = append(out, "+ "+bl[j])
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, "- "+al[i])
	}
	for ; j < m; j++ {
		out = append(out, "+ "+bl[j])
	}
	return out
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func compactJSON(data []byte) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return string(data)
	}
	return buf.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
