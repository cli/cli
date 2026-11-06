package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// logOptions are the parsed flags of ghmock log.
type logOptions struct {
	all    bool
	full   bool
	asJSON bool
}

func parseLogArgs(args []string) (logOptions, error) {
	var opts logOptions
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.all, "all", false, "show requests from every run")
	fs.BoolVar(&opts.full, "full", false, "include GraphQL query text, untruncated data and the response gh got")
	fs.BoolVar(&opts.asJSON, "json", false, "print one JSON object per request")
	if err := fs.Parse(args); err != nil {
		return logOptions{}, usageError{err}
	}
	if fs.NArg() > 0 {
		return logOptions{}, newUsageError("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return opts, nil
}

func cmdLog(args []string, stdout io.Writer) error {
	opts, err := parseLogArgs(args)
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
	if !opts.all {
		entries, err = st.latestRequests(entries)
		if err != nil {
			return err
		}
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "No requests logged.")
		return nil
	}
	if opts.asJSON {
		enc := json.NewEncoder(stdout)
		for _, e := range entries {
			if err := enc.Encode(e); err != nil {
				return err
			}
		}
		return nil
	}
	printLog(stdout, entries, opts.full)
	return nil
}

func (s *state) latestRequests(entries []loggedRequest) ([]loggedRequest, error) {
	runs, err := s.readRuns()
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return lastRun(entries), nil
	}
	return requestsForRun(entries, runs[len(runs)-1].ID), nil
}

// lastRun keeps only the entries from the most recent run.
func lastRun(entries []loggedRequest) []loggedRequest {
	if len(entries) == 0 {
		return nil
	}
	return requestsForRun(entries, entries[len(entries)-1].Run)
}

func requestsForRun(entries []loggedRequest, runID string) []loggedRequest {
	var kept []loggedRequest
	for _, e := range entries {
		if e.Run == runID {
			kept = append(kept, e)
		}
	}
	return kept
}

func printLog(w io.Writer, entries []loggedRequest, full bool) {
	var run, binary string
	for _, e := range entries {
		if e.Run != run || e.Binary != binary {
			run, binary = e.Run, e.Binary
			fmt.Fprintf(w, "=== run %s, gh %s\n", run, binary)
		}
		fmt.Fprintln(w, formatRequest(e))
		if !full {
			continue
		}
		if e.GraphQL != nil {
			fmt.Fprintf(w, "   %s %s\n", e.GraphQL.Kind, e.GraphQL.operationLabel())
			if len(e.GraphQL.Variables) > 0 {
				fmt.Fprintf(w, "   variables: %s\n", compactJSON(e.GraphQL.Variables))
			}
			fmt.Fprintln(w, "   query:")
			for _, line := range splitLines(e.GraphQL.Query) {
				fmt.Fprintf(w, "     %s\n", line)
			}
		}
		if e.Body != "" {
			fmt.Fprintf(w, "   body: %s\n", compactJSON([]byte(e.Body)))
		}
		if e.RawBody != nil {
			fmt.Fprintf(w, "   raw request body: %s (bytes in --json)\n", rawBodySummary(*e.RawBody))
		}
		if len(e.ResponseHeaders) > 0 {
			names := slices.Sorted(maps.Keys(e.ResponseHeaders))
			for i, name := range names {
				names[i] = name + ": " + e.ResponseHeaders[name]
			}
			fmt.Fprintf(w, "   response headers: %s\n", strings.Join(names, ", "))
		}
		if len(e.ResponseBody) > 0 {
			fmt.Fprintf(w, "   response: %s\n", compactJSON(e.ResponseBody))
		}
		if e.ResponseRawBody != nil {
			fmt.Fprintf(w, "   raw response body: %s (bytes in --json)\n", rawBodySummary(*e.ResponseRawBody))
		}
		for _, group := range problemGroups(e) {
			fmt.Fprintf(w, "   %s:\n", group.label)
			for _, p := range group.problems {
				fmt.Fprintf(w, "     %s\n", p)
			}
		}
	}
}

func rawBodySummary(body []byte) string {
	return fmt.Sprintf("%d bytes, sha256 %x", len(body), sha256.Sum256(body))
}

func cmdReset(args []string, stdout io.Writer) error {
	if askedForHelp(args) {
		return flag.ErrHelp
	}
	if len(args) > 0 {
		return newUsageError("reset takes no arguments")
	}
	st, err := openState()
	if err != nil {
		return err
	}
	if err := st.reset(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Deleted responses, request log and saved runs in %s\n", st.dir)
	return nil
}
