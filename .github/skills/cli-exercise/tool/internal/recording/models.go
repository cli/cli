// Package recording defines the data shared by capture, prerequisite checks, and rendering.
package recording

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode"
)

// DefaultOutputTiming shortens presentation pauses without changing captured timestamps.
const DefaultOutputTiming = "condensed"

// OutputOptions describes presentation choices, separate from execution evidence.
type OutputOptions struct {
	Formats             []string `json:"formats"`
	Timing              string   `json:"timing,omitempty"`
	Captions            *bool    `json:"captions"`
	TimingAuthorization string   `json:"timingAuthorization,omitempty"`
}

// UnmarshalJSON defaults only an omitted timing value, not a blank or null value.
func (options *OutputOptions) UnmarshalJSON(data []byte) error {
	type fields OutputOptions
	var value struct {
		fields
		Timing json.RawMessage `json:"timing"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	timing := DefaultOutputTiming
	if len(value.Timing) != 0 {
		if err := json.Unmarshal(value.Timing, &timing); err != nil ||
			strings.TrimSpace(string(value.Timing)) == "null" || !slices.Contains([]string{"realtime", "condensed"}, timing) {
			return fmt.Errorf("output timing must be realtime or condensed")
		}
	}
	value.fields.Timing = timing
	*options = OutputOptions(value.fields)
	return nil
}

// Span is a styled span of actual terminal cells.
type Span struct {
	Text  string `json:"text"`
	Width *int   `json:"width"`
	Flags int    `json:"flags"`
	FG    string `json:"fg"`
	BG    string `json:"bg"`
}

// UnmarshalJSON distinguishes missing or null text from an intentionally empty span.
func (span *Span) UnmarshalJSON(data []byte) error {
	type fields Span
	var value struct {
		fields
		Text *string `json:"text"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	if value.Text == nil {
		return fmt.Errorf("terminal span must contain text")
	}
	value.fields.Text = *value.Text
	*span = Span(value.fields)
	return nil
}

// Line contains the ordered spans in one terminal row.
type Line struct {
	Spans []Span `json:"spans"`
}

// TerminalData decodes the known Tuistory fields. Capture retains the original JSON alongside it.
type TerminalData struct {
	Columns       int    `json:"cols"`
	Rows          int    `json:"rows"`
	Cursor        []int  `json:"cursor"`
	CursorVisible *bool  `json:"cursorVisible"`
	CursorStyle   string `json:"cursorStyle"`
	Offset        *int   `json:"offset,omitempty"`
	TotalLines    *int   `json:"totalLines,omitempty"`
	Lines         []Line `json:"lines"`
}

// VisibleText returns only the visible viewport, excluding scrollback.
func (data TerminalData) VisibleText() (string, error) {
	if data.Rows < 1 || data.Rows > 1000 || data.Columns < 1 || data.Columns > 1000 || data.Lines == nil {
		return "", fmt.Errorf("terminal data has invalid dimensions or lines")
	}
	lines := make([]string, 0, min(data.Rows, len(data.Lines)))
	for _, line := range data.Lines[max(0, len(data.Lines)-data.Rows):] {
		if line.Spans == nil {
			return "", fmt.Errorf("terminal line has invalid spans")
		}
		var joined strings.Builder
		for _, span := range line.Spans {
			joined.WriteString(span.Text)
		}
		lines = append(lines, strings.TrimRightFunc(joined.String(), unicode.IsSpace))
	}
	return strings.TrimRightFunc(strings.Join(lines, "\n"), unicode.IsSpace), nil
}

// Executable identifies a selected helper tool.
type Executable struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256,omitempty"`
}

// Tuistory identifies an explicitly selected library installation.
type Tuistory struct {
	ModuleRoot    string `json:"moduleRoot"`
	Version       string `json:"version,omitempty"`
	PackageSHA256 string `json:"packageSha256,omitempty"`
}

// Font preserves a legacy preflight font selection for receipt compatibility.
type Font struct {
	Path   string `json:"path"`
	Family string `json:"family"`
}

// Tools distinguishes an unavailable tool from a selected one.
type Tools struct {
	Node     *Executable `json:"node,omitempty"`
	FFmpeg   *Executable `json:"ffmpeg,omitempty"`
	FFprobe  *Executable `json:"ffprobe,omitempty"`
	Tuistory *Tuistory   `json:"tuistory,omitempty"`
	Font     *Font       `json:"font,omitempty"`
}

// Executable returns one of the known executable selections.
func (tools Tools) Executable(name string) *Executable {
	switch name {
	case "node":
		return tools.Node
	case "ffmpeg":
		return tools.FFmpeg
	case "ffprobe":
		return tools.FFprobe
	default:
		return nil
	}
}

// CapabilityChecks records observed readiness rather than presumed compatibility.
type CapabilityChecks struct {
	Formats   map[string]bool `json:"formats"`
	NativePTY bool            `json:"nativePty"`
}

// Receipt is the common prerequisite identity and capability record.
type Receipt struct {
	SchemaVersion int              `json:"schemaVersion"`
	Status        string           `json:"status"`
	Manifest      string           `json:"manifestSha256"`
	Tools         Tools            `json:"tools"`
	Checks        CapabilityChecks `json:"checks"`
}
