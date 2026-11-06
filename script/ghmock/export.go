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

type exportOptions struct {
	path    string
	archive bool
}

func parseExportArgs(args []string) (exportOptions, error) {
	var opts exportOptions
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&opts.archive, "archive", false, "write a portable tar.gz archive")
	if err := fs.Parse(args); err != nil {
		return exportOptions{}, usageError{err}
	}
	switch fs.NArg() {
	case 0:
		opts.path = os.Getenv("GHMOCK_EXPORT_PATH")
		if opts.path == "" {
			return exportOptions{}, newUsageError("missing folder to export to")
		}
	case 1:
		opts.path = fs.Arg(0)
	default:
		return exportOptions{}, newUsageError("unexpected arguments: %s", strings.Join(fs.Args()[1:], " "))
	}
	return opts, nil
}

// cmdExport writes everything needed to check a result to a folder or archive: each
// run's command, output and report, every request with the response gh got,
// the stored responses, and the schema files the checks used.
func cmdExport(args []string, stdout io.Writer) error {
	opts, err := parseExportArgs(args)
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
	write := func(dir string) error {
		return writeExportDir(st, dir, runs, entries, responses)
	}
	if opts.archive {
		err = writeExportArchive(opts.path, write)
	} else {
		err = write(opts.path)
	}
	if err != nil {
		return err
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
	fmt.Fprintf(stdout, "%s to %s\n", summary, opts.path)
	return nil
}

func writeExportDir(st *state, dir string, runs []runRecord, entries []loggedRequest, responses responseSet) error {
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
	if _, err := os.Stat(filepath.Join(st.dir, "git")); err == nil {
		if err := copyGitTree(filepath.Join(st.dir, "git"), filepath.Join(dir, "git")); err != nil {
			return fmt.Errorf("exporting Git fixtures and workspaces: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Join(st.dir, "config")); err == nil {
		if err := copyConfigTree(filepath.Join(st.dir, "config"), filepath.Join(dir, "config")); err != nil {
			return fmt.Errorf("exporting configuration fixtures: %w", err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}

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
		"- `runs.jsonl`: each run's command, gh's output and exit code, and the SHA-256 of each schema file.\n" +
		"- `git/`, when present: the prepared fixture, each run's starting state and independent per-build workspaces. Git worktree links are relocated for this export. Each workspace's `commands.jsonl` records Git setup or inspection commands, output and requests.\n\n" +
		"- `config/`, when present: the supplied configuration before each run and each build's resulting configuration. These files include fixture credentials; supply synthetic credentials only. Paths in `config_baseline` and each result's `config.directory` are relative to this export.\n\n" +
		"Archive exports preserve hidden Git metadata and use relative worktree links. Extract into an empty folder before inspection. Linked worktrees in portable archives need Git 2.48 or newer. Directory exports are prepared for their original destination.\n\n" +
		"To read the requests with ghmock:\n\n" +
		"```sh\nGHMOCK_DIR=<this folder> ghmock log --all --full\n```\n\n" +
		"To inspect retained Git state, select a build by its one-based position in the run:\n\n" +
		"```sh\nGHMOCK_DIR=<this folder> ghmock git --run latest --build 1 --in <checkout> -- status --porcelain=v1\n" +
		"GHMOCK_DIR=<this folder> ghmock git --run latest --before --in <checkout> -- status --porcelain=v1\n```\n" +
		"\nThese are real repositories containing untrusted fixture data. Inspect their configuration and hooks before running Git commands; Git can execute programs as well as modify the repositories. No GitHub API mutation is automatically mirrored into a Git repository.\n")
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
