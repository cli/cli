package command

// This file extends survey.Editor to give it more flexible behavior. For more context, read
// https://github.com/github/gh-cli/issues/70

import (
	"bytes"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	shellquote "github.com/kballard/go-shellquote"
)

var (
	bom    = []byte{0xef, 0xbb, 0xbf}
	editor = "nano"
)

func init() {
	if runtime.GOOS == "windows" {
		editor = "notepad"
	}
	if v := os.Getenv("VISUAL"); v != "" {
		editor = v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		editor = e
	}
}

type ghEditor struct {
	*survey.Editor
}

// this is not being called in the embedding case and isn't consulted in the alias case because of
// the incomplete overriding.
func (e *ghEditor) prompt(initialValue string, config *survey.PromptConfig) (interface{}, error) {
	// render the template
	err := e.Render(
		survey.EditorQuestionTemplate,
		survey.EditorTemplateData{
			Editor: *e.Editor,
			Config: config,
		},
	)
	if err != nil {
		return "", err
	}

	// start reading runes from the standard in
	rr := e.NewRuneReader()
	rr.SetTermMode()
	defer rr.RestoreTermMode()

	cursor := e.NewCursor()
	cursor.Hide()
	defer cursor.Show()

	for {
		r, _, err := rr.ReadRune()
		if err != nil {
			return "", err
		}
		if r == '\r' || r == '\n' {
			break
		}
		if r == terminal.KeyInterrupt {
			return "", terminal.InterruptErr
		}
		if r == terminal.KeyEndTransmission {
			break
		}
		if string(r) == config.HelpInput && e.Help != "" {
			err = e.Render(
				survey.EditorQuestionTemplate,
				survey.EditorTemplateData{
					Editor:   *e.Editor,
					ShowHelp: true,
					Config:   config,
				},
			)
			if err != nil {
				return "", err
			}
		}
		continue
	}

	// prepare the temp file
	pattern := e.FileName
	if pattern == "" {
		pattern = "survey*.txt"
	}
	f, err := ioutil.TempFile("", pattern)
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())

	// write utf8 BOM header
	// The reason why we do this is because notepad.exe on Windows determines the
	// encoding of an "empty" text file by the locale, for example, GBK in China,
	// while golang string only handles utf8 well. However, a text file with utf8
	// BOM header is not considered "empty" on Windows, and the encoding will then
	// be determined utf8 by notepad.exe, instead of GBK or other encodings.
	if _, err := f.Write(bom); err != nil {
		return "", err
	}

	// write initial value
	if _, err := f.WriteString(initialValue); err != nil {
		return "", err
	}

	// close the fd to prevent the editor unable to save file
	if err := f.Close(); err != nil {
		return "", err
	}

	stdio := e.Stdio()

	args, err := shellquote.Split(editor)
	if err != nil {
		return "", err
	}
	args = append(args, f.Name())

	// open the editor
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = stdio.In
	cmd.Stdout = stdio.Out
	cmd.Stderr = stdio.Err
	cursor.Show()
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// raw is a BOM-unstripped UTF8 byte slice
	raw, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return "", err
	}

	// strip BOM header
	text := string(bytes.TrimPrefix(raw, bom))

	// check length, return default value on empty
	if len(text) == 0 && !e.AppendDefault {
		return e.Default, nil
	}

	return text, nil
}

func (e *ghEditor) Prompt(config *survey.PromptConfig) (interface{}, error) {
	initialValue := ""
	if e.Default != "" && e.AppendDefault {
		initialValue = e.Default
	}
	return e.prompt(initialValue, config)
}
