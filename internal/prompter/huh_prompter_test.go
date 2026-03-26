package prompter

import (
	"io"
	"testing"
	"time"

	"charm.land/huh/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Interaction helpers ---
// A set of helpers for simulating user input in huh form tests.
// Each helper (tab(), toggle(), typeKeys(), etc.) produces raw terminal
// bytes that are piped into form.Run() via io.Pipe, driving the real
// bubbletea event loop.

type interactionStep struct {
	bytes []byte
	delay time.Duration // pause before sending (lets the event loop settle)
}

type interaction struct {
	steps []interactionStep
}

func newInteraction(steps ...interactionStep) interaction {
	return interaction{steps: steps}
}

func (ix interaction) run(t *testing.T, w *io.PipeWriter) {
	t.Helper()
	for _, s := range ix.steps {
		time.Sleep(s.delay)
		_, err := w.Write(s.bytes)
		require.NoError(t, err)
	}
}

// Step helpers — each returns a single interactionStep.
//
// These send raw terminal escape sequences that bubbletea's input parser
// understands. Common ANSI escape codes:
//
//	\t        = Tab
//	\x1b[Z   = Shift+Tab (reverse tab)
//	\r       = Enter (carriage return)
//	\x1b[A   = Arrow Up
//	\x1b[B   = Arrow Down
//	\x1b[C   = Arrow Right
//	\x1b[D   = Arrow Left
//	\x01     = Ctrl+A (line start)
//	\x0b     = Ctrl+K (kill to end of line)

func tab() interactionStep {
	return interactionStep{bytes: []byte("\t")}
}

func shiftTab() interactionStep {
	return interactionStep{bytes: []byte("\x1b[Z")}
}

func enter() interactionStep {
	return interactionStep{bytes: []byte("\r")}
}

func toggle() interactionStep {
	return interactionStep{bytes: []byte("x")}
}

func down() interactionStep {
	return interactionStep{bytes: []byte("\x1b[B")}
}

func left() interactionStep {
	return interactionStep{bytes: []byte("\x1b[D")}
}

func right() interactionStep {
	return interactionStep{bytes: []byte("\x1b[C")}
}

func typeKeys(s string) interactionStep {
	return interactionStep{bytes: []byte(s)}
}

func pressY() interactionStep {
	return interactionStep{bytes: []byte("y")}
}

func pressN() interactionStep {
	return interactionStep{bytes: []byte("n")}
}

func clearLine() interactionStep {
	return interactionStep{bytes: []byte{0x01, 0x0b}}
}

// waitForOptions adds extra delay to let OptionsFunc load before continuing.
func waitForOptions() interactionStep {
	return interactionStep{bytes: nil, delay: 50 * time.Millisecond}
}

// --- Test harness ---

func newTestHuhPrompter() *huhPrompter {
	return &huhPrompter{}
}

// runForm runs a huh form with the given interaction, returning any error.
// The form runs in a goroutine using bubbletea's real event loop via io.Pipe.
func runForm(t *testing.T, f *huh.Form, ix interaction) {
	t.Helper()
	r, w := io.Pipe()
	f.WithInput(r).WithOutput(io.Discard).WithWidth(80)

	errCh := make(chan error, 1)
	go func() { errCh <- f.Run() }()

	ix.run(t, w)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("form.Run() did not complete in time")
	}
}

// --- Tests ---

func TestHuhPrompterInput(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue string
		ix           interaction
		wantResult   string
	}{
		{
			name:       "basic input",
			ix:         newInteraction(typeKeys("hello"), enter()),
			wantResult: "hello",
		},
		{
			name:         "default value returned when no input",
			defaultValue: "default",
			ix:           newInteraction(enter()),
			wantResult:   "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildInputForm("Name:", tt.defaultValue)
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterSelect(t *testing.T) {
	tests := []struct {
		name         string
		options      []string
		defaultValue string
		ix           interaction
		wantIndex    int
	}{
		{
			name:      "selects first option by default",
			options:   []string{"a", "b", "c"},
			ix:        newInteraction(enter()),
			wantIndex: 0,
		},
		{
			name:         "respects default value",
			options:      []string{"a", "b", "c"},
			defaultValue: "b",
			ix:           newInteraction(enter()),
			wantIndex:    1,
		},
		{
			name:         "invalid default selects first",
			options:      []string{"a", "b", "c"},
			defaultValue: "z",
			ix:           newInteraction(enter()),
			wantIndex:    0,
		},
		{
			name:      "navigate down one",
			options:   []string{"a", "b", "c"},
			ix:        newInteraction(down(), enter()),
			wantIndex: 1,
		},
		{
			name:      "navigate down two",
			options:   []string{"a", "b", "c"},
			ix:        newInteraction(down(), down(), enter()),
			wantIndex: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildSelectForm("Pick:", tt.defaultValue, tt.options)
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantIndex, *result)
		})
	}
}

func TestHuhPrompterMultiSelect(t *testing.T) {
	tests := []struct {
		name       string
		options    []string
		defaults   []string
		ix         interaction
		wantResult []int
	}{
		{
			name:       "no defaults and no toggles returns empty",
			options:    []string{"a", "b", "c"},
			ix:         newInteraction(enter()),
			wantResult: []int{},
		},
		{
			name:       "defaults are pre-selected",
			options:    []string{"a", "b", "c"},
			defaults:   []string{"a", "c"},
			ix:         newInteraction(enter()),
			wantResult: []int{0, 2},
		},
		{
			name:       "toggle first option",
			options:    []string{"a", "b", "c"},
			ix:         newInteraction(toggle(), enter()),
			wantResult: []int{0},
		},
		{
			name:    "toggle multiple options",
			options: []string{"a", "b", "c"},
			ix: newInteraction(
				toggle(), // toggle a
				down(),   // move to b
				down(),   // move to c
				toggle(), // toggle c
				enter(),
			),
			wantResult: []int{0, 2},
		},
		{
			name:       "invalid defaults are excluded",
			options:    []string{"a", "b"},
			defaults:   []string{"z"},
			ix:         newInteraction(enter()),
			wantResult: []int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildMultiSelectForm("Pick:", tt.defaults, tt.options)
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterConfirm(t *testing.T) {
	tests := []struct {
		name         string
		defaultValue bool
		ix           interaction
		wantResult   bool
	}{
		{
			name:       "default false submitted as-is",
			ix:         newInteraction(enter()),
			wantResult: false,
		},
		{
			name:         "default true submitted as-is",
			defaultValue: true,
			ix:           newInteraction(enter()),
			wantResult:   true,
		},
		{
			name:       "toggle from false to true with left arrow",
			ix:         newInteraction(left(), enter()),
			wantResult: true,
		},
		{
			name:         "toggle from true to false with right arrow",
			defaultValue: true,
			ix:           newInteraction(right(), enter()),
			wantResult:   false,
		},
		{
			name:       "accept with y key",
			ix:         newInteraction(pressY(), enter()),
			wantResult: true,
		},
		{
			name:         "reject with n key",
			defaultValue: true,
			ix:           newInteraction(pressN(), enter()),
			wantResult:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildConfirmForm("Sure?", tt.defaultValue)
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterPassword(t *testing.T) {
	tests := []struct {
		name       string
		ix         interaction
		wantResult string
	}{
		{
			name:       "basic password",
			ix:         newInteraction(typeKeys("s3cret"), enter()),
			wantResult: "s3cret",
		},
		{
			name:       "empty password",
			ix:         newInteraction(enter()),
			wantResult: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildPasswordForm("Password:")
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterMarkdownEditor(t *testing.T) {
	tests := []struct {
		name         string
		blankAllowed bool
		ix           interaction
		wantResult   string
	}{
		{
			name:         "selects launch by default",
			blankAllowed: true,
			ix:           newInteraction(enter()),
			wantResult:   "launch",
		},
		{
			name:         "navigate to skip",
			blankAllowed: true,
			ix:           newInteraction(down(), enter()),
			wantResult:   "skip",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildMarkdownEditorForm("Body:", tt.blankAllowed)
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterMultiSelectWithSearch(t *testing.T) {
	staticSearchFunc := func(query string) MultiSelectSearchResult {
		if query == "" {
			return MultiSelectSearchResult{
				Keys:   []string{"result-a", "result-b"},
				Labels: []string{"Result A", "Result B"},
			}
		}
		return MultiSelectSearchResult{
			Keys:   []string{"search-1", "search-2"},
			Labels: []string{"Search 1", "Search 2"},
		}
	}

	tests := []struct {
		name       string
		defaults   []string
		persistent []string
		ix         interaction
		wantResult []string
	}{
		{
			name:       "defaults are pre-selected and returned on immediate submit",
			defaults:   []string{"result-a"},
			ix:         newInteraction(tab(), enter()),
			wantResult: []string{"result-a"},
		},
		{
			name:       "toggle an option from search results",
			ix:         newInteraction(tab(), waitForOptions(), toggle(), enter()),
			wantResult: []string{"result-a"},
		},
		{
			name: "toggle multiple options",
			ix: newInteraction(
				tab(), waitForOptions(),
				toggle(), // toggle result-a
				down(),   // move to result-b
				toggle(), // toggle result-b
				enter(),
			),
			wantResult: []string{"result-a", "result-b"},
		},
		{
			name:       "no selection returns empty",
			ix:         newInteraction(tab(), enter()),
			wantResult: []string{},
		},
		{
			name:       "persistent options are shown and selectable",
			persistent: []string{"persistent-1"},
			ix: newInteraction(
				tab(), waitForOptions(),
				down(),   // skip result-a
				down(),   // skip result-b
				toggle(), // toggle persistent-1
				enter(),
			),
			wantResult: []string{"persistent-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildMultiSelectWithSearchForm(
				"Select", "Search", tt.defaults, tt.persistent, staticSearchFunc,
			)
			runForm(t, f, tt.ix)
			assert.Equal(t, tt.wantResult, result.selectedKeys())
		})
	}
}

func TestHuhPrompterMultiSelectWithSearchPersistence(t *testing.T) {
	staticSearchFunc := func(query string) MultiSelectSearchResult {
		if query == "" {
			return MultiSelectSearchResult{
				Keys:   []string{"result-a", "result-b"},
				Labels: []string{"Result A", "Result B"},
			}
		}
		return MultiSelectSearchResult{
			Keys:   []string{"search-1", "search-2"},
			Labels: []string{"Search 1", "Search 2"},
		}
	}

	t.Run("selections persist after changing search query", func(t *testing.T) {
		p := newTestHuhPrompter()
		f, result := p.buildMultiSelectWithSearchForm(
			"Select", "Search", nil, nil, staticSearchFunc,
		)
		runForm(t, f, newInteraction(
			tab(), waitForOptions(),
			toggle(),        // toggle result-a
			shiftTab(),      // back to search input
			typeKeys("foo"), // change query
			tab(), waitForOptions(),
			enter(), // submit — result-a should persist
		))
		assert.Equal(t, []string{"result-a"}, result.selectedKeys())
	})
	t.Run("empty search results shows no-results placeholder", func(t *testing.T) {
		emptySearchFunc := func(query string) MultiSelectSearchResult {
			return MultiSelectSearchResult{}
		}
		p := newTestHuhPrompter()
		f, result := p.buildMultiSelectWithSearchForm(
			"Select", "Search", nil, nil, emptySearchFunc,
		)
		// With no results, the "No results" message is shown.
		// Toggle does nothing, submitting returns empty.
		runForm(t, f, newInteraction(tab(), waitForOptions(), toggle(), enter()))
		assert.Equal(t, []string{}, result.selectedKeys())
	})
}

func TestHuhPrompterAuthToken(t *testing.T) {
	tests := []struct {
		name       string
		ix         interaction
		wantResult string
	}{
		{
			name:       "accepts token input",
			ix:         newInteraction(typeKeys("ghp_abc123"), enter()),
			wantResult: "ghp_abc123",
		},
		{
			name:       "rejects blank then accepts valid input",
			ix:         newInteraction(enter(), typeKeys("ghp_valid"), enter()),
			wantResult: "ghp_valid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildAuthTokenForm()
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterConfirmDeletion(t *testing.T) {
	tests := []struct {
		name          string
		requiredValue string
		ix            interaction
	}{
		{
			name:          "accepts matching input",
			requiredValue: "my-repo",
			ix:            newInteraction(typeKeys("my-repo"), enter()),
		},
		{
			name:          "rejects wrong input then accepts correct input",
			requiredValue: "my-repo",
			ix:            newInteraction(typeKeys("wrong"), enter(), clearLine(), typeKeys("my-repo"), enter()),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f := p.buildConfirmDeletionForm(tt.requiredValue)
			runForm(t, f, tt.ix)
		})
	}
}

func TestHuhPrompterInputHostname(t *testing.T) {
	tests := []struct {
		name       string
		ix         interaction
		wantResult string
	}{
		{
			name:       "accepts valid hostname",
			ix:         newInteraction(typeKeys("github.example.com"), enter()),
			wantResult: "github.example.com",
		},
		{
			name:       "rejects blank then accepts valid hostname",
			ix:         newInteraction(enter(), typeKeys("github.example.com"), enter()),
			wantResult: "github.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestHuhPrompter()
			f, result := p.buildInputHostnameForm()
			runForm(t, f, tt.ix)
			require.Equal(t, tt.wantResult, *result)
		})
	}
}

func TestHuhPrompterMultiSelectWithSearchBackspace(t *testing.T) {
	// Simulate real API latency and non-overlapping results.
	staticSearchFunc := func(query string) MultiSelectSearchResult {
		time.Sleep(100 * time.Millisecond) // simulate API latency
		if query == "" {
			return MultiSelectSearchResult{
				Keys:   []string{"alice", "bob"},
				Labels: []string{"Alice", "Bob"},
			}
		}
		return MultiSelectSearchResult{
			Keys:   []string{"frank", "fiona"},
			Labels: []string{"Frank", "Fiona"},
		}
	}

	t.Run("selections persist after backspacing search query", func(t *testing.T) {
		p := newTestHuhPrompter()
		f, result := p.buildMultiSelectWithSearchForm(
			"Select", "Search", nil, nil, staticSearchFunc,
		)
		longWait := interactionStep{delay: 300 * time.Millisecond}
		runForm(t, f, newInteraction(
			tab(), longWait,
			toggle(),         // toggle alice
			shiftTab(),       // back to search input
			typeKeys("f"),    // type "f"
			longWait,         // wait for API + OptionsFunc
			typeKeys("\x7f"), // backspace to ""
			longWait,         // wait for cache/API
			tab(), longWait,
			enter(),
		))
		assert.Equal(t, []string{"alice"}, result.selectedKeys())
	})
}

func TestRunFormTranslatesErrUserAborted(t *testing.T) {
	p := newTestHuhPrompter()
	form, _ := p.buildSelectForm("Pick one:", "", []string{"a", "b", "c"})

	r, w := io.Pipe()
	form.WithInput(r).WithOutput(io.Discard).WithWidth(80)

	errCh := make(chan error, 1)
	go func() { errCh <- p.runForm(form) }()

	// Send Ctrl+C to trigger huh.ErrUserAborted
	_, err := w.Write([]byte{0x03})
	require.NoError(t, err)

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, terminal.InterruptErr, "expected huh.ErrUserAborted to be translated to terminal.InterruptErr")
	case <-time.After(5 * time.Second):
		t.Fatal("runForm did not complete in time")
	}
}
