// Package evidence verifies and renders a captured invocation without rerunning it.
package evidence

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cli/cli/v2/cli-exercise/internal/engine"
	"github.com/cli/cli/v2/cli-exercise/internal/recording"
)

type terminalConfig struct {
	Columns       int      `json:"columns"`
	Rows          int      `json:"rows"`
	FontPath      string   `json:"-"`
	FontFallbacks []string `json:"-"`
	FontSize      float64  `json:"fontSize"`
	FPS           int      `json:"fps"`
	Background    string   `json:"background"`
	Foreground    string   `json:"foreground"`
}

type contract struct {
	CaseID       string                  `json:"caseId"`
	Mode         string                  `json:"mode"`
	Goal         string                  `json:"goal"`
	Workspace    string                  `json:"workspace"`
	Terminal     terminalConfig          `json:"terminal"`
	Steps        []json.RawMessage       `json:"steps"`
	Expectations []expectation           `json:"expectations"`
	Output       recording.OutputOptions `json:"output"`
	Command      struct {
		Executable string   `json:"executable"`
		Args       []string `json:"args"`
	} `json:"command"`
	Chapter *chapterPresentation `json:"-"`
}

type result struct {
	ContractSHA256  string  `json:"contractSha256"`
	StartedAt       string  `json:"startedAt,omitempty"`
	FinishedAt      string  `json:"finishedAt,omitempty"`
	DurationSeconds float64 `json:"durationSeconds"`
	ExitCode        *int    `json:"exitCode"`
	CaptureStatus   string  `json:"captureStatus"`
	StepsCompleted  *int    `json:"stepsCompleted"`
	Interrupted     bool    `json:"interrupted"`
}

type state struct {
	Time            *float64               `json:"t"`
	Revision        int                    `json:"revision"`
	AlternateScreen bool                   `json:"alternateScreen"`
	Data            recording.TerminalData `json:"data"`
}

type event struct {
	Time    *float64 `json:"t"`
	Type    string   `json:"type"`
	Chapter *string  `json:"chapter"`
	Text    *string  `json:"text"`
}

type expectation struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Value       json.RawMessage `json:"value,omitempty"`
	Description string          `json:"description,omitempty"`
}

type check struct {
	expectation
	engine.AssertionResult
	Evidence []string `json:"evidence,omitempty"`
}

type verification struct {
	ContractSHA256 string               `json:"contractSha256"`
	Results        []verificationResult `json:"results"`
}

type verificationResult struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Reason   string   `json:"reason"`
	Evidence []string `json:"evidence"`
}

func containedFile(workspace, relative string) (string, error) {
	if filepath.IsAbs(relative) || relative == "" {
		return "", fmt.Errorf("verification evidence paths must be workspace-relative")
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, relative))
	if err != nil {
		return "", fmt.Errorf("verification evidence does not exist: %w", err)
	}
	if !inside(root, candidate) {
		return "", fmt.Errorf("verification evidence escapes the workspace")
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("verification evidence must identify an existing regular file")
	}
	return candidate, nil
}

func evaluate(contract contract, result result, states []state, manual *verification, workspace, hash string) (string, []check, error) {
	incomplete := result.CaptureStatus != "complete" || result.Interrupted
	if contract.Mode == "exact" {
		incomplete = incomplete || result.StepsCompleted == nil || *result.StepsCompleted != len(contract.Steps)
	}
	allowed := make(map[string]bool)
	for _, expectation := range contract.Expectations {
		if expectation.Type == "manual" {
			allowed[expectation.ID] = true
		}
	}
	overrides := make(map[string]check)
	if manual != nil {
		if manual.ContractSHA256 != hash {
			return "", nil, fmt.Errorf("manual verification belongs to a different contract")
		}
		for _, item := range manual.Results {
			if !allowed[item.ID] {
				return "", nil, fmt.Errorf("unknown manual expectation %q", item.ID)
			}
			if _, exists := overrides[item.ID]; exists {
				return "", nil, fmt.Errorf("duplicate manual expectation %q", item.ID)
			}
			if !slices.Contains([]string{"passed", "failed", "blocked"}, item.Status) ||
				strings.TrimSpace(item.Reason) == "" || len(item.Evidence) == 0 {
				return "", nil, fmt.Errorf("manual expectation %q needs a valid status, reason, and evidence", item.ID)
			}
			for _, path := range item.Evidence {
				if _, err := containedFile(workspace, path); err != nil {
					return "", nil, err
				}
			}
			overrides[item.ID] = check{Status: item.Status, Reason: item.Reason, Evidence: item.Evidence}
		}
	}
	text := ""
	if len(states) != 0 {
		var err error
		text, err = states[len(states)-1].Data.VisibleText()
		if err != nil {
			return "", nil, err
		}
	}
	checks := make([]check, 0, len(contract.Expectations))
	actual := engine.AssertionContext{ExitCode: result.ExitCode, Conclusive: !incomplete}
	if len(states) != 0 {
		actual.Text = &text
	}
	results := make([]engine.AssertionResult, 0, len(contract.Expectations))
	for _, expected := range contract.Expectations {
		var value any
		if len(expected.Value) != 0 {
			if err := json.Unmarshal(expected.Value, &value); err != nil {
				return "", nil, fmt.Errorf("expectation %q has an invalid value: %w", expected.ID, err)
			}
		}
		item := check{expectation: expected, AssertionResult: engine.EvaluateAssertion(expected.Type, value, actual)}
		if expected.Type == "manual" && overrides[expected.ID].Status != "" {
			item = overrides[expected.ID]
			item.expectation = expected
		}
		checks = append(checks, item)
		results = append(results, item.AssertionResult)
	}
	return engine.AssertionVerdict(results, !incomplete), checks, nil
}

type frame struct {
	State   int
	Note    *event
	Time    float64
	Repeats int
}

func framePlan(states []state, events []event, duration float64, fps int, timing string) ([]frame, error) {
	if len(states) == 0 {
		return nil, fmt.Errorf("no recorded terminal states are available")
	}
	previous := -1.0
	for _, state := range states {
		if state.Time == nil || !finite(*state.Time) || *state.Time < 0 || *state.Time < previous {
			return nil, fmt.Errorf("terminal timestamps must be finite, nonnegative, and monotonic")
		}
		previous = *state.Time
	}
	if !finite(duration) || duration <= 0 || fps < 1 || fps > 60 || previous > duration+1/float64(fps) {
		return nil, fmt.Errorf("recorded duration or frame rate does not cover the captured states")
	}
	duration = max(duration, previous)
	if duration*float64(fps) > 1_000_000 {
		return nil, fmt.Errorf("recording timeline exceeds the one-million-frame rendering limit")
	}
	notes := make([]event, 0)
	for _, event := range events {
		if event.Type != "note" {
			continue
		}
		if event.Time == nil || !finite(*event.Time) || *event.Time < 0 ||
			*event.Time > duration+1/float64(fps) || event.Text == nil {
			return nil, fmt.Errorf("recorded annotation has invalid timing or text")
		}
		notes = append(notes, event)
	}
	slices.SortStableFunc(notes, func(a, b event) int {
		if *a.Time < *b.Time {
			return -1
		}
		if *a.Time > *b.Time {
			return 1
		}
		return 0
	})
	type point struct {
		time    float64
		repeats int
	}
	points := make([]point, 0)
	switch timing {
	case "realtime":
		for index := range max(1, int(math.Ceil(duration*float64(fps)))) {
			points = append(points, point{float64(index) / float64(fps), 1})
		}
	case "condensed":
		boundaries := []float64{0, duration}
		for _, state := range states {
			boundaries = append(boundaries, *state.Time)
		}
		for _, note := range notes {
			if *note.Time < duration {
				boundaries = append(boundaries, *note.Time)
			}
		}
		slices.Sort(boundaries)
		boundaries = slices.Compact(boundaries)
		for index := 0; index+1 < len(boundaries); index++ {
			length := boundaries[index+1] - boundaries[index]
			if length > 0 {
				points = append(points, point{boundaries[index], max(1, int(math.RoundToEven(min(length, 2)*float64(fps))))})
			}
		}
	default:
		return nil, fmt.Errorf("unsupported timing mode %q", timing)
	}
	if len(points) == 0 || points[len(points)-1].time < previous {
		points = append(points, point{previous, 1})
	}
	plan := make([]frame, 0, len(points))
	stateIndex, noteIndex := 0, -1
	for _, point := range points {
		for stateIndex+1 < len(states) && *states[stateIndex+1].Time <= point.time {
			stateIndex++
		}
		for noteIndex+1 < len(notes) && *notes[noteIndex+1].Time <= point.time {
			noteIndex++
		}
		entry := frame{State: stateIndex, Time: point.time, Repeats: point.repeats}
		if noteIndex >= 0 {
			entry.Note = &notes[noteIndex]
		}
		plan = append(plan, entry)
	}
	return plan, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
