package surveyext

// This file extends survey.Editor to give it more flexible behavior. For more context, read
// https://github.com/cli/cli/issues/70
// To see what we extended, search through for EXTENDED comments.

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	shellquote "github.com/kballard/go-shellquote"
)

var (
	bom           = []byte{0xef, 0xbb, 0xbf}
	defaultEditor = "nano" // EXTENDED to switch from vim as a default editor
)

func init() {
	if g := os.Getenv("GIT_EDITOR"); g != "" {
		defaultEditor = g
	} else if v := os.Getenv("VISUAL"); v != "" {
		defaultEditor = v
	} else if e := os.Getenv("EDITOR"); e != "" {
		defaultEditor = e
	} else if runtime.GOOS == "windows" {
		defaultEditor = "notepad"
	}
}

// EXTENDED to enable different prompting behavior
type GhEditor struct {
	*survey.Editor
	EditorCommand string
	BlankAllowed  bool

	lookPath func(string) ([]string, []string, error)
}

// EXTENDED to change prompt text
var EditorQuestionTemplate = `
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }} {{color "reset"}}
{{- if .ShowAnswer}}
  {{- color "cyan"}}{{.Answer}}{{color "reset"}}{{"\n"}}
{{- else }}
  {{- if and .Help (not .ShowHelp)}}{{color "cyan"}}[{{ .Config.HelpInput }} for help]{{color "reset"}} {{end}}
  {{- if and .Default (not .HideDefault)}}{{color "white"}}({{.Default}}) {{color "reset"}}{{end}}
	{{- color "cyan"}}[(e) to launch {{ .EditorCommand }}{{- if .BlankAllowed }}, enter to skip{{ end }}] {{color "reset"}}
{{- end}}`

// EXTENDED to pass editor name (to use in prompt)
type EditorTemplateData struct {
	survey.Editor
	EditorCommand string
	BlankAllowed  bool
	Answer        string
	ShowAnswer    bool
	ShowHelp      bool
	Config        *survey.PromptConfig
}

// EXTENDED to augment prompt text and keypress handling
func (e *GhEditor) prompt(initialValue string, config *survey.PromptConfig) (interface{}, error) {
	err := e.Render(
		EditorQuestionTemplate,
		// EXTENDED to support printing editor in prompt and BlankAllowed
		EditorTemplateData{
			Editor:        *e.Editor,
			BlankAllowed:  e.BlankAllowed,
			EditorCommand: EditorName(e.EditorCommand),
			Config:        config,
		},
	)
	if err != nil {
		return "", err
	}

	// start reading runes from the standard in
	rr := e.NewRuneReader()
	_ = rr.SetTermMode()
	defer func() { _ = rr.RestoreTermMode() }()

	cursor := e.NewCursor()
	_ = cursor.Hide()
	defer func() {
		_ = cursor.Show()
	}()

	for {
		// EXTENDED to handle the e to edit / enter to skip behavior + BlankAllowed
		r, _, err := rr.ReadRune()
		if err != nil {
			return "", err
		}
		if r == 'e' {
			break
		}
		if r == '\r' || r == '\n' {
			if e.BlankAllowed {
				return initialValue, nil
			} else {
				continue
			}
		}
		if r == terminal.KeyInterrupt {
			return "", terminal.InterruptErr
		}
		if r == terminal.KeyEndTransmission {
			break
		}
		if string(r) == config.HelpInput && e.Help != "" {
			err = e.Render(
				EditorQuestionTemplate,
				EditorTemplateData{
					// EXTENDED to support printing editor in prompt, BlankAllowed
					Editor:        *e.Editor,
					BlankAllowed:  e.BlankAllowed,
					EditorCommand: EditorName(e.EditorCommand),
					ShowHelp:      true,
					Config:        config,
				},
			)
			if err != nil {
				return "", err
			}
		}
		continue
	}

	stdio := e.Stdio()
	lookPath := e.lookPath
	if lookPath == nil {
		lookPath = defaultLookPath
	}
	text, err := edit(e.EditorCommand, e.FileName, initialValue, stdio.In, stdio.Out, stdio.Err, cursor, lookPath)
	if err != nil {
		return "", err
	}

	// check length, return default value on empty
	if len(text) == 0 && !e.AppendDefault {
		return e.Default, nil
	}

	return text, nil
}

// EXTENDED This is straight copypasta from survey to get our overridden prompt called.;
func (e *GhEditor) Prompt(config *survey.PromptConfig) (interface{}, error) {
	initialValue := ""
	if e.Default != "" && e.AppendDefault {
		initialValue = e.Default
	}
	return e.prompt(initialValue, config)
}

func EditorName(editorCommand string) string {
	if editorCommand == "" {
		editorCommand = defaultEditor
	}
	if args, err := shellquote.Split(editorCommand); err == nil {
		editorCommand = args[0]
	}
	return filepath.Base(editorCommand)
}
