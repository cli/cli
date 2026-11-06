package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	fs.BoolVar(&opts.full, "full", false, "include GraphQL query text and untruncated data")
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
		runs, err := st.readRuns()
		if err != nil {
			return err
		}
		if len(runs) == 0 {
			entries = lastRun(entries)
		} else {
			entries = requestsForRun(entries, runs[len(runs)-1].ID)
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
	}
}

func cmdReset(args []string, stdout io.Writer) error {
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
	fmt.Fprintf(stdout, "Deleted responses and request log in %s\n", st.dir)
	return nil
}
