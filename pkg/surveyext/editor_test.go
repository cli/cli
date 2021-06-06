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
	"github.com/stretchr/testify/require"
)

func Test_GhEditor_Prompt(t *testing.T) {
	e := &GhEditor{
		BlankAllowed:  true,
		EditorCommand: "false",
		Editor: &survey.Editor{
			Message:       "Body",
			FileName:      "*.md",
			Default:       "initial value",
			HideDefault:   true,
			AppendDefault: true,
		},
	}

	pty, tty, err := pseudotty.Open()
	if errors.Is(err, pseudotty.ErrUnsupported) {
		return
	}
	require.NoError(t, err)
	defer pty.Close()
	defer tty.Close()

	err = pseudotty.Setsize(tty, &pseudotty.Winsize{Cols: 72, Rows: 30})
	require.NoError(t, err)

	out := teeWriter{File: tty}
	e.WithStdio(terminal.Stdio{
		In:  tty,
		Out: &out,
		Err: tty,
	})

	var res string
	errc := make(chan error)

	go func() {
		r, err := e.Prompt(defaultPromptConfig())
		if r != nil {
			res = r.(string)
		}
		errc <- err
	}()

	time.Sleep(5 * time.Millisecond)
	assert.Equal(t, "\x1b[0G\x1b[2K\x1b[0;1;92m? \x1b[0m\x1b[0;1;99mBody \x1b[0m\x1b[0;36m[(e) to launch false, enter to skip] \x1b[0m\x1b[?25l", out.String())
	fmt.Fprint(pty, "\n") // send Enter key

	err = <-errc
	assert.NoError(t, err)
	assert.Equal(t, "initial value", res)
	assert.Equal(t, "\x1b[?25h", out.String())
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
	f.buf.Reset()
	f.mu.Unlock()
	return s
}
