package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// response is one canned reply to a request. A zero Status means 200.
// OffSchema marks a response saved with --off-schema, which is sent as stored,
// without trimming, whether or not it passes the schema check.
type response struct {
	Status    int               `json:"status,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      json.RawMessage   `json:"body,omitempty"`
	OffSchema bool              `json:"off_schema,omitempty"`
}

func (r response) status() int {
	if r.Status == 0 {
		return 200
	}
	return r.Status
}

// responseSet maps a request name to its responses, used in order within a run.
type responseSet map[string][]response

// loggedRequest is one request that gh made during a run. QueryProblems are
// ways gh's GraphQL query doesn't match GitHub's schema. SchemaProblems are
// ways the stored response doesn't match it, and SchemaNote says why a
// response wasn't checked. ResponseHeaders and ResponseBody are what gh got,
// after trimming, because the stored response can change after the run.
type loggedRequest struct {
	Run      string          `json:"run"`
	Binary   string          `json:"binary"`
	Seq      int             `json:"seq"`
	Method   string          `json:"method"`
	Host     string          `json:"host"`
	Path     string          `json:"path,omitempty"`
	RawQuery string          `json:"raw_query,omitempty"`
	Name     string          `json:"name,omitempty"`
	GraphQL  *graphQLRequest `json:"graphql,omitempty"`
	Body     string          `json:"body,omitempty"`
	Status   int             `json:"status"`
	Matched  bool            `json:"matched"`
	Blocked  bool            `json:"blocked,omitempty"`
	// TLSError is why gh's TLS handshake with ghmock failed, such as gh not
	// trusting ghmock's certificate authority.
	TLSError       string   `json:"tls_error,omitempty"`
	Response       int      `json:"response,omitempty"`
	Responses      int      `json:"responses,omitempty"`
	OffSchema      bool     `json:"off_schema,omitempty"`
	QueryProblems  []string `json:"query_problems,omitempty"`
	SchemaProblems []string `json:"schema_problems,omitempty"`
	SchemaNote     string   `json:"schema_note,omitempty"`

	ResponseHeaders map[string]string `json:"response_headers,omitempty"`
	ResponseBody    json.RawMessage   `json:"response_body,omitempty"`
}

// runRecord is one ghmock run, kept for ghmock export. Report is what ghmock
// run printed.
type runRecord struct {
	ID      string       `json:"id"`
	Time    time.Time    `json:"time"`
	Command string       `json:"command"`
	Schemas []schemaFile `json:"schemas,omitempty"`
	Results []runOutcome `json:"results"`
	Report  string       `json:"report"`
}

// runOutcome is what one gh binary did in a run.
type runOutcome struct {
	Binary   string `json:"binary"`
	Path     string `json:"path"`
	Version  string `json:"version"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out,omitempty"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	// Terminal runs keep the screen before each --keys entry in Steps, and
	// the final screen in Stdout.
	Terminal       bool           `json:"terminal,omitempty"`
	TerminalOutput string         `json:"terminal_output,omitempty"`
	Steps          []terminalStep `json:"steps,omitempty"`
	Waiting        bool           `json:"waiting_for_input,omitempty"`
	UnusedKeys     int            `json:"unused_keys,omitempty"`
}

// state is the folder that holds responses, the request log and the runs
// between commands.
type state struct {
	dir string
}

func openState() (*state, error) {
	dir := os.Getenv("GHMOCK_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), "ghmock")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &state{dir: dir}, nil
}

func (s *state) responsesPath() string {
	return filepath.Join(s.dir, "responses.json")
}

func (s *state) logPath() string {
	return filepath.Join(s.dir, "requests.jsonl")
}

func (s *state) runsPath() string {
	return filepath.Join(s.dir, "runs.jsonl")
}

func (s *state) loadResponses() (responseSet, error) {
	data, err := os.ReadFile(s.responsesPath())
	if errors.Is(err, fs.ErrNotExist) {
		return responseSet{}, nil
	}
	if err != nil {
		return nil, err
	}
	responses := responseSet{}
	if len(bytes.TrimSpace(data)) == 0 {
		return responses, nil
	}
	if err := json.Unmarshal(data, &responses); err != nil {
		return nil, fmt.Errorf("reading %s: %w", s.responsesPath(), err)
	}
	return responses, nil
}

// saveResponses writes through a temporary file so a running server never
// reads a half-written file.
func (s *state) saveResponses(responses responseSet) error {
	data, err := json.MarshalIndent(responses, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "responses-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.responsesPath())
}

func (s *state) appendLog(entries []loggedRequest) error {
	return appendJSONL(s.logPath(), entries)
}

func (s *state) readLog() ([]loggedRequest, error) {
	return readJSONL[loggedRequest](s.logPath())
}

func (s *state) appendRun(rec runRecord) error {
	return appendJSONL(s.runsPath(), []runRecord{rec})
}

func (s *state) readRuns() ([]runRecord, error) {
	return readJSONL[runRecord](s.runsPath())
}

// appendJSONL adds items to a file, one JSON object per line.
func appendJSONL[T any](path string, items []T) error {
	if len(items) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := writeJSONL(f, items); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeJSONL[T any](w io.Writer, items []T) error {
	enc := json.NewEncoder(w)
	for _, item := range items {
		if err := enc.Encode(item); err != nil {
			return err
		}
	}
	return nil
}

// readJSONL reads a file of JSON objects, one per line. A missing file has
// none.
func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var items []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		items = append(items, item)
	}
	return items, scanner.Err()
}

func (s *state) reset() error {
	for _, p := range []string{s.responsesPath(), s.logPath(), s.runsPath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
