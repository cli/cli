package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

const (
	maxShownBody     = 300
	maxShownProblems = 10
)

// printReport writes what each binary did. With more than one binary, it then
// lists how each one differs from the first. notes say which schema checks
// couldn't run.
func printReport(w io.Writer, opts runOptions, results []runResult, notes []string) {
	quoted := make([]string, len(opts.args))
	for i, arg := range opts.args {
		quoted[i] = shellQuote(arg)
	}
	command := "$ gh " + strings.Join(quoted, " ")
	if len(results) == 1 {
		fmt.Fprintf(w, "%s - %s\n", results[0].version, results[0].path)
		printResult(w, command, results[0])
		printNotes(w, notes)
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
	printNotes(w, notes)
}

func printNotes(w io.Writer, notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Fprintln(w, "--- schema checks")
	for _, n := range notes {
		fmt.Fprintln(w, n)
	}
}

func printResult(w io.Writer, command string, res runResult) {
	if res.config != nil {
		fmt.Fprintf(w, "Retained configuration: %s\n", res.config.Directory)
	}
	if res.git != nil {
		fmt.Fprintf(w, "Git: %s\nRetained Git state: %s\n", res.git.Version, res.git.Directory)
	}
	fmt.Fprintln(w, command)
	switch {
	case res.timedOut:
		fmt.Fprintln(w, "exit code: none, timed out")
	default:
		fmt.Fprintf(w, "exit code: %d\n", res.exitCode)
	}
	if res.terminal {
		if res.terminalOutput != "" {
			printOutput(w, "terminal output", res.terminalOutput)
		}
		for i, s := range res.steps {
			printOutput(w, fmt.Sprintf("screen before --keys %d, %s", i+1, s.Keys), s.Screen)
		}
		printOutput(w, "final screen", res.stdout)
		if res.unusedKeys > 0 {
			fmt.Fprintf(w, "gh exited before ghmock typed the last %s.\n", plural(res.unusedKeys, "--keys entry"))
		}
	} else {
		printOutput(w, "stdout", res.stdout)
		printOutput(w, "stderr", res.stderr)
	}

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
	if blocked := blockedNames(res.requests); len(blocked) > 0 {
		fmt.Fprintln(w, "--- blocked, because with --localhost ghmock only serves github.localhost")
		for _, name := range blocked {
			fmt.Fprintln(w, name)
		}
		fmt.Fprintln(w, "If gh picked that host because of --hostname or a URL, leave out --hostname and use github.localhost in URLs.")
	}
	if failed := tlsFailures(res.requests); len(failed) > 0 {
		fmt.Fprintln(w, "--- gh rejected ghmock's certificate")
		for _, f := range failed {
			fmt.Fprintln(w, f)
		}
		fmt.Fprintln(w, "gh must trust ghmock's certificate authority through SSL_CERT_FILE. Some builds ignore it, such as macOS builds made with older Go. Run those with --localhost.")
	}
	printProblems(w, res.requests)
}

// tlsFailures lists, once each and in order, the hosts whose TLS handshake
// with ghmock failed, with the reason.
func tlsFailures(requests []loggedRequest) []string {
	seen := map[string]bool{}
	var failures []string
	for _, e := range requests {
		line := e.Host + ": " + e.TLSError
		if e.TLSError == "" || seen[line] {
			continue
		}
		seen[line] = true
		failures = append(failures, line)
	}
	return failures
}

// blockedNames lists, once each and in order, requests ghmock refused because
// they were for another host.
func blockedNames(requests []loggedRequest) []string {
	seen := map[string]bool{}
	var names []string
	for _, e := range requests {
		name := displayName(e)
		if !e.Blocked || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	return names
}

// printProblems lists what the schema checks found, request by request.
func printProblems(w io.Writer, requests []loggedRequest) {
	header := false
	for _, e := range requests {
		for _, group := range problemGroups(e) {
			if !header {
				fmt.Fprintln(w, "--- schema problems")
				header = true
			}
			fmt.Fprintf(w, "%d. %s: %s\n", e.Seq, displayName(e), group.label)
			fmt.Fprint(w, indentProblems(group.problems, "   ", maxShownProblems))
		}
	}
}

// problemGroup is one kind of schema problem found for a request.
type problemGroup struct {
	label    string
	problems []string
}

func problemGroups(e loggedRequest) []problemGroup {
	var groups []problemGroup
	if len(e.SchemaProblems) > 0 {
		groups = append(groups, problemGroup{"the response doesn't match GitHub's schema", e.SchemaProblems})
	}
	if len(e.QueryProblems) > 0 {
		groups = append(groups, problemGroup{"gh's query doesn't match GitHub's schema, so GitHub would answer with an error", e.QueryProblems})
	}
	return groups
}

// indentProblems lists problems one per line. With a limit above zero, it
// shows at most that many and points to the log for the rest.
func indentProblems(problems []string, indent string, limit int) string {
	var b strings.Builder
	for i, p := range problems {
		if limit > 0 && i == limit {
			fmt.Fprintf(&b, "%s...and %d more; ghmock log --json shows them all\n", indent, len(problems)-i)
			break
		}
		fmt.Fprintf(&b, "%s%s\n", indent, p)
	}
	return b.String()
}

func printOutput(w io.Writer, label, text string) {
	fmt.Fprintf(w, "--- %s\n", label)
	if text == "" {
		fmt.Fprintln(w, "(empty)")
		return
	}
	text = showEscapes(text)
	fmt.Fprint(w, text)
	if !strings.HasSuffix(text, "\n") {
		fmt.Fprintln(w)
	}
}

// showEscapes writes the escape character as \e, so color codes such as
// \e[33m stay visible in reports and a missing reset can be spotted.
func showEscapes(text string) string {
	return strings.ReplaceAll(text, "\x1b", `\e`)
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
	if e.RawBody != nil {
		b.WriteString(" raw body " + rawBodySummary(*e.RawBody))
	}
	switch {
	case e.Git != nil:
		if e.Method == "SSH" {
			fmt.Fprintf(&b, " -> %s, exit code %d", e.Git.Service, e.Git.ExitCode)
		} else {
			fmt.Fprintf(&b, " -> %d, %s", e.Status, e.Git.Service)
		}
		if e.Git.Error != "" {
			b.WriteString(" [" + e.Git.Error + "]")
		}
	case e.Blocked:
		b.WriteString(" -> blocked")
	case e.TLSError != "":
		b.WriteString(" -> TLS handshake failed")
	case e.Matched:
		fmt.Fprintf(&b, " -> %d, response %d of %d", e.Status, e.Response, e.Responses)
	default:
		fmt.Fprintf(&b, " -> %d, no response stored", e.Status)
	}
	if e.OffSchema {
		b.WriteString(" [off-schema]")
	}
	if n := len(e.SchemaProblems); n > 0 {
		fmt.Fprintf(&b, " [%s]", plural(n, "schema problem"))
	}
	if n := len(e.QueryProblems); n > 0 {
		fmt.Fprintf(&b, " [query: %s]", plural(n, "schema problem"))
	}
	if e.SchemaNote != "" {
		fmt.Fprintf(&b, " [not checked: %s]", e.SchemaNote)
	}
	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// missingNames lists, once each and in order, requests that had no response.
func missingNames(requests []loggedRequest) []string {
	seen := map[string]bool{}
	var names []string
	for _, e := range requests {
		if e.Matched || e.Git != nil || e.Blocked || e.TLSError != "" || seen[e.Name] {
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

	if exitLabel(base) != exitLabel(other) {
		same = false
		fmt.Fprintf(w, "  exit code: %s -> %s\n", exitLabel(base), exitLabel(other))
	}
	stdoutLabel := "stdout"
	if base.terminal {
		stdoutLabel = "final screen"
	}
	for _, out := range []struct{ label, a, b string }{
		{stdoutLabel, base.stdout, other.stdout},
		{"stderr", base.stderr, other.stderr},
		{"terminal output", base.terminalOutput, other.terminalOutput},
	} {
		if out.a == out.b {
			continue
		}
		same = false
		fmt.Fprintf(w, "  %s:\n", out.label)
		for _, line := range lineDiff(showEscapes(out.a), showEscapes(out.b)) {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}

	if !slices.Equal(base.steps, other.steps) {
		same = false
		fmt.Fprintf(w, "  prompt snapshots differ: %d in [1], %d in [%d]\n", len(base.steps), len(other.steps), n)
		for i := 0; i < min(len(base.steps), len(other.steps)); i++ {
			if base.steps[i] != other.steps[i] {
				fmt.Fprintf(w, "    prompt %d: %s -> %s\n", i+1, showEscapes(base.steps[i].Screen), showEscapes(other.steps[i].Screen))
			}
		}
	}
	if base.unusedKeys != other.unusedKeys {
		same = false
		fmt.Fprintf(w, "  unused keys: %d -> %d\n", base.unusedKeys, other.unusedKeys)
	}

	baseCounts, otherCounts := countNames(base.requests), countNames(other.requests)
	for _, name := range unionNames(base.requests, other.requests) {
		if baseCounts[name] != otherCounts[name] {
			same = false
			fmt.Fprintf(w, "  %s: %d in [1], %d in [%d]\n", name, baseCounts[name], otherCounts[name], n)
		}
	}

	for _, name := range unionNames(base.requests, other.requests) {
		a, b := askedFields(base.requests, name), askedFields(other.requests, name)
		if a == nil || b == nil {
			continue
		}
		onlyBase, onlyOther := onlyIn(a, b), onlyIn(b, a)
		if len(onlyBase) == 0 && len(onlyOther) == 0 {
			continue
		}
		same = false
		fmt.Fprintf(w, "  %s asked for different fields:\n", name)
		if len(onlyBase) > 0 {
			fmt.Fprintf(w, "    only [1]: %s\n", joinLimited(onlyBase, maxShownProblems))
		}
		if len(onlyOther) > 0 {
			fmt.Fprintf(w, "    only [%d]: %s\n", n, joinLimited(onlyOther, maxShownProblems))
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

// askedFields lists the fields that GraphQL requests with this name asked
// for, or nil if there were none whose query could be read.
func askedFields(requests []loggedRequest, name string) map[string]bool {
	var fields map[string]bool
	for _, e := range requests {
		if displayName(e) != name || e.GraphQL == nil {
			continue
		}
		paths := queryFields(e.GraphQL.Query, e.GraphQL.Operation)
		if paths == nil {
			continue
		}
		if fields == nil {
			fields = map[string]bool{}
		}
		for path := range paths {
			fields[path] = true
		}
	}
	return fields
}

// joinLimited joins items, showing at most limit of them.
func joinLimited(items []string, limit int) string {
	if len(items) > limit {
		return strings.Join(items[:limit], ", ") + fmt.Sprintf(" and %d more", len(items)-limit)
	}
	return strings.Join(items, ", ")
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
	rawBody                          string
	raw                              bool
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
		} else if e.RawBody != nil {
			p.raw = true
			p.rawBody = string(*e.RawBody)
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
		} else if p.raw {
			parts = append(parts, "raw body "+rawBodySummary([]byte(p.rawBody)))
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
