package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// response is one canned reply to a request. A zero Status means 200.
type response struct {
	Status  int               `json:"status,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
}

func (r response) status() int {
	if r.Status == 0 {
		return 200
	}
	return r.Status
}

// responseSet maps a request name to its responses, used in order within a run.
type responseSet map[string][]response

// loggedRequest is one request that gh made during a run.
type loggedRequest struct {
	Run       string          `json:"run"`
	Binary    string          `json:"binary"`
	Seq       int             `json:"seq"`
	Method    string          `json:"method"`
	Host      string          `json:"host"`
	Path      string          `json:"path,omitempty"`
	RawQuery  string          `json:"raw_query,omitempty"`
	Name      string          `json:"name,omitempty"`
	GraphQL   *graphQLRequest `json:"graphql,omitempty"`
	Body      string          `json:"body,omitempty"`
	Status    int             `json:"status"`
	Matched   bool            `json:"matched"`
	Blocked   bool            `json:"blocked,omitempty"`
	Response  int             `json:"response,omitempty"`
	Responses int             `json:"responses,omitempty"`
}

// isWrite reports whether the request asked GitHub to change something.
func (r loggedRequest) isWrite() bool {
	if r.GraphQL != nil {
		return r.GraphQL.Kind == "mutation"
	}
	switch r.Method {
	case "GET", "HEAD", "OPTIONS", "CONNECT":
		return false
	}
	return true
}

// state is the folder that holds responses and the request log between commands.
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
	if len(entries) == 0 {
		return nil
	}
	f, err := os.OpenFile(s.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

func (s *state) readLog() ([]loggedRequest, error) {
	f, err := os.Open(s.logPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []loggedRequest
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var e loggedRequest
		if err := json.Unmarshal(line, &e); err != nil {
			return nil, fmt.Errorf("reading %s: %w", s.logPath(), err)
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func (s *state) reset() error {
	for _, p := range []string{s.responsesPath(), s.logPath()} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}
