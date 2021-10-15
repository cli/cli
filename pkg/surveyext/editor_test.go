package surveyext

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	pseudotty "github.com/creack/pty"
	"github.com/stretchr/testify/assert"
)

func testLookPath(s string) ([]string, []string, error) {
	return []string{os.Args[0], "-test.run=TestHelperProcess", "--", s}, []string{"GH_WANT_HELPER_PROCESS=1"}, nil
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GH_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if err := func(args []string) error {
		switch args[0] {
		// "vim" appends a message to the file
		case "vim":
			f, err := os.OpenFile(args[1], os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = f.WriteString(" - added by vim")
			return err
		// "nano" truncates the contents of the file
		case "nano":
			f, err := os.OpenFile(args[1], os.O_TRUNC|os.O_WRONLY, 0)
			if err != nil {
				return err
			}
			return f.Close()
		default:
			return fmt.Errorf("unrecognized arguments: %#v\n", args)
		}
	}(os.Args[3:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func Test_GhEditor_Prompt_skip(t *testing.T) {
	pty := newTerminal(t)

	e := &GhEditor{
		BlankAllowed:  true,
		EditorCommand: "vim",
		Editor: &survey.Editor{
			Message:       "Body",
			FileName:      "*.md",
			Default:       "initial value",
			HideDefault:   true,
			AppendDefault: true,
		},
		lookPath: func(s string) ([]string, []string, error) {
			return nil, nil, errors.New("no editor allowed")
		},
	}
	e.WithStdio(pty.Stdio())

	// wait until the prompt is rendered and send the Enter key
	go func() {
		pty.WaitForOutput("Body")
		assert.Equal(t, "\x1b[0G\x1b[2K\x1b[0;1;92m? \x1b[0m\x1b[0;1;99mBody \x1b[0m\x1b[0;36m[(e) to launch vim, enter to skip] \x1b[0m", normalizeANSI(pty.Output()))
		pty.ResetOutput()
		assert.NoError(t, pty.SendKey('\n'))
	}()

	res, err := e.Prompt(defaultPromptConfig())
	assert.NoError(t, err)
	assert.Equal(t, "initial value", res)
	assert.Equal(t, "", normalizeANSI(pty.Output()))
}

func Test_GhEditor_Prompt_editorAppend(t *testing.T) {
	pty := newTerminal(t)

	e := &GhEditor{
		BlankAllowed:  true,
		EditorCommand: "vim",
		Editor: &survey.Editor{
			Message:       "Body",
			FileName:      "*.md",
			Default:       "initial value",
			HideDefault:   true,
			AppendDefault: true,
		},
		lookPath: testLookPath,
	}
	e.WithStdio(pty.Stdio())

	// wait until the prompt is rendered and send the 'e' key
	go func() {
		pty.WaitForOutput("Body")
		assert.Equal(t, "\x1b[0G\x1b[2K\x1b[0;1;92m? \x1b[0m\x1b[0;1;99mBody \x1b[0m\x1b[0;36m[(e) to launch vim, enter to skip] \x1b[0m", normalizeANSI(pty.Output()))
		pty.ResetOutput()
		assert.NoError(t, pty.SendKey('e'))
	}()

	res, err := e.Prompt(defaultPromptConfig())
	assert.NoError(t, err)
	assert.Equal(t, "initial value - added by vim", res)
	assert.Equal(t, "", normalizeANSI(pty.Output()))
}

func Test_GhEditor_Prompt_editorTruncate(t *testing.T) {
	pty := newTerminal(t)

	e := &GhEditor{
		BlankAllowed:  true,
		EditorCommand: "nano",
		Editor: &survey.Editor{
			Message:       "Body",
			FileName:      "*.md",
			Default:       "initial value",
			HideDefault:   true,
			AppendDefault: true,
		},
		lookPath: testLookPath,
	}
	e.WithStdio(pty.Stdio())

	// wait until the prompt is rendered and send the 'e' key
	go func() {
		pty.WaitForOutput("Body")
		assert.Equal(t, "\x1b[0G\x1b[2K\x1b[0;1;92m? \x1b[0m\x1b[0;1;99mBody \x1b[0m\x1b[0;36m[(e) to launch nano, enter to skip] \x1b[0m", normalizeANSI(pty.Output()))
		pty.ResetOutput()
		assert.NoError(t, pty.SendKey('e'))
	}()

	res, err := e.Prompt(defaultPromptConfig())
	assert.NoError(t, err)
	assert.Equal(t, "", res)
	assert.Equal(t, "", normalizeANSI(pty.Output()))
}

// survey doesn't expose this
func defaultPromptConfig() *survey.PromptConfig {
	return &survey.PromptConfig{
		PageSize:     7,
		HelpInput:    "?",
		SuggestInput: "tab",
		Icons: survey.IconSet{
			Error: survey.Icon{
				Text:   "X",
				Format: "red",
			},
			Help: survey.Icon{
				Text:   "?",
				Format: "cyan",
			},
			Question: survey.Icon{
				Text:   "?",
				Format: "green+hb",
			},
			MarkedOption: survey.Icon{
				Text:   "[x]",
				Format: "green",
			},
			UnmarkedOption: survey.Icon{
				Text:   "[ ]",
				Format: "default+hb",
			},
			SelectFocus: survey.Icon{
				Text:   ">",
				Format: "cyan+b",
			},
		},
		Filter: func(filter string, value string, index int) (include bool) {
			filter = strings.ToLower(filter)
			return strings.Contains(strings.ToLower(value), filter)
		},
		KeepFilter: false,
	}
}

type testTerminal struct {
	pty    *os.File
	tty    *os.File
	stdout *teeWriter
	stderr *teeWriter
}

func newTerminal(t *testing.T) *testTerminal {
	t.Helper()

	pty, tty, err := pseudotty.Open()
	if errors.Is(err, pseudotty.ErrUnsupported) {
		t.SkipNow()
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pty.Close()
		tty.Close()
	})

	if err := pseudotty.Setsize(tty, &pseudotty.Winsize{Cols: 72, Rows: 30}); err != nil {
		t.Fatal(err)
	}

	return &testTerminal{pty: pty, tty: tty}
}

func (t *testTerminal) SendKey(c rune) error {
	_, err := t.pty.WriteString(string(c))
	return err
}

func (t *testTerminal) WaitForOutput(s string) {
	for {
		time.Sleep(time.Millisecond)
		if strings.Contains(t.stdout.String(), s) {
			return
		}
	}
}

func (t *testTerminal) Output() string {
	return t.stdout.String()
}

func (t *testTerminal) ResetOutput() {
	t.stdout.Reset()
}

func (t *testTerminal) Stdio() terminal.Stdio {
	t.stdout = &teeWriter{File: t.tty}
	t.stderr = t.stdout
	return terminal.Stdio{
		In:  t.tty,
		Out: t.stdout,
		Err: t.stderr,
	}
}

// teeWriter is a writer that duplicates all writes to a file into a buffer
type teeWriter struct {
	*os.File
	buf bytes.Buffer
	mu  sync.Mutex
}

func (f *teeWriter) Write(p []byte) (n int, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, _ = f.buf.Write(p)
	return f.File.Write(p)
}

func (f *teeWriter) String() string {
	f.mu.Lock()
	s := f.buf.String()
	f.mu.Unlock()
	return s
}

func (f *teeWriter) Reset() {
	f.mu.Lock()
	f.buf.Reset()
	f.mu.Unlock()
}

// strips some ANSI escape sequences that we do not want tests to be concerned with
func normalizeANSI(t string) string {
	t = strings.ReplaceAll(t, "\x1b[?25h", "") // strip sequence that shows cursor
	t = strings.ReplaceAll(t, "\x1b[?25l", "") // strip sequence that hides cursor
	return t
}
