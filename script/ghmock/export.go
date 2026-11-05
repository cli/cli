package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

func parseExportArgs(args []string) (string, error) {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return "", usageError{err}
	}
	switch fs.NArg() {
	case 0:
		return "", newUsageError("missing folder to export to")
	case 1:
		return fs.Arg(0), nil
	default:
		return "", newUsageError("unexpected arguments: %s", strings.Join(fs.Args()[1:], " "))
	}
}

// cmdExport writes everything needed to check a result to a folder: each
// run's command, output and report, every request with the response gh got,
// the stored responses, and the schema files the checks used.
func cmdExport(args []string, stdout io.Writer) error {
	dir, err := parseExportArgs(args)
	if err != nil {
		return err
	}
	st, err := openState()
	if err != nil {
		return err
	}
	runs, err := st.readRuns()
	if err != nil {
		return err
	}
	if len(runs) == 0 {
		return errors.New("nothing to export yet; run gh with ghmock run first")
	}
	entries, err := st.readLog()
	if err != nil {
		return err
	}
	responses, err := st.loadResponses()
	if err != nil {
		return err
	}
	if err := prepareExportDir(dir); err != nil {
		return err
	}

	files := []struct {
		name  string
		write func(io.Writer) error
	}{
		{"README.md", func(w io.Writer) error {
			_, err := io.WriteString(w, exportReadme())
			return err
		}},
		{"report.txt", func(w io.Writer) error { return writeExportReport(w, runs) }},
		{"runs.jsonl", func(w io.Writer) error { return writeJSONL(w, runs) }},
		{"requests.jsonl", func(w io.Writer) error { return writeJSONL(w, entries) }},
		{"responses.json", func(w io.Writer) error {
			data, err := json.MarshalIndent(responses, "", "  ")
			if err != nil {
				return err
			}
			_, err = w.Write(append(data, '\n'))
			return err
		}},
	}
	for _, f := range files {
		if err := writeFile(filepath.Join(dir, f.name), f.write); err != nil {
			return err
		}
	}

	total, offSchema := 0, 0
	for _, list := range responses {
		for _, r := range list {
			total++
			if r.OffSchema {
				offSchema++
			}
		}
	}
	summary := fmt.Sprintf("Exported %s, %s and %s", plural(len(runs), "run"), plural(len(entries), "request"), plural(total, "response"))
	if offSchema > 0 {
		summary += fmt.Sprintf(", %d off-schema,", offSchema)
	}
	fmt.Fprintf(stdout, "%s to %s\n", summary, dir)
	return nil
}

// prepareExportDir creates dir, or checks that it's empty, so an export never
// mixes with other files.
func prepareExportDir(dir string) error {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(dir, 0o755)
	case err != nil:
		return err
	case len(entries) > 0:
		return fmt.Errorf("%s isn't empty; export to a new folder", dir)
	}
	return nil
}

func writeFile(path string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := write(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeExportReport writes what each run printed, after its full command and
// the schema files it checked against.
func writeExportReport(w io.Writer, runs []runRecord) error {
	for i, run := range runs {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "### Run %s at %s\n", run.ID, run.Time.UTC().Format(time.RFC3339))
		fmt.Fprintf(w, "$ %s\n", run.Command)
		for _, f := range run.Schemas {
			if f.Error != "" {
				fmt.Fprintf(w, "%s: not loaded, so not checked: %s\n", f.Name, f.Error)
			} else {
				fmt.Fprintf(w, "%s: sha256 %s, %s\n", f.Name, f.SHA256, f.Path)
			}
		}
		fmt.Fprintln(w)
		if _, err := io.WriteString(w, run.Report); err != nil {
			return err
		}
	}
	return nil
}

func exportReadme() string {
	var b strings.Builder
	b.WriteString("# ghmock export\n\n" +
		"- `report.txt`: what each `ghmock run` printed, after its full command and the schema files it checked against.\n" +
		"- `requests.jsonl`: every request gh made, one JSON object per line, with the response gh got after trimming.\n" +
		"- `responses.json`: the stored responses when this was exported. Responses marked `\"off_schema\": true` were saved without passing the schema check.\n" +
		"- `runs.jsonl`: each run's command, gh's output and exit code, and the SHA-256 of each schema file.\n\n" +
		"To read the requests with ghmock:\n\n" +
		"```sh\nGHMOCK_DIR=<this folder> ghmock log --all --full\n```\n")
	if rev := buildRevision(); rev != "" {
		fmt.Fprintf(&b, "\nMade by ghmock from cli/cli %s.\n", rev)
	}
	return b.String()
}

// buildRevision is the commit ghmock was built from, if Go recorded it.
func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	var revision string
	modified := false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if revision != "" && modified {
		revision += ", with local changes"
	}
	return revision
}
