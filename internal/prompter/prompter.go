package prompter

import (
	"fmt"
	"io"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/surveyext"
)

//go:generate moq -rm -out prompter_mock.go . Prompter
type Prompter interface {
	Select(string, string, []string) (int, error)
	MultiSelect(string, string, []string) (int, error)
	Input(string, string) (string, error)
	InputHostname() (string, error)
	Password(string) (string, error)
	AuthToken() (string, error)
	Confirm(string, bool) (bool, error)
	MarkdownEditor(string, string, bool) (string, error)
}

func New(editorCmd string, stdin io.Reader, stdout, stderr io.Writer) Prompter {
	return &surveyPrompter{
		editorCmd: editorCmd,
		stdin:     stdin.(terminal.FileReader),
		stdout:    stdout.(terminal.FileWriter),
		stderr:    stderr,
	}
}

type surveyPrompter struct {
	editorCmd string
	stdin     terminal.FileReader
	stdout    terminal.FileWriter
	stderr    io.Writer
}

func (p *surveyPrompter) Select(message, defaultValue string, options []string) (result int, err error) {
	q := &survey.Select{
		Message:  message,
		Default:  defaultValue,
		Options:  options,
		PageSize: 20,
	}

	err = p.ask(q, &result)

	return
}

func (p *surveyPrompter) MultiSelect(message, defaultValue string, options []string) (result int, err error) {
	q := &survey.MultiSelect{
		Message:  message,
		Default:  defaultValue,
		Options:  options,
		PageSize: 20,
	}

	err = p.ask(q, &result)

	return
}

func (p *surveyPrompter) ask(q survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	opts = append(opts, survey.WithStdio(p.stdin, p.stdout, p.stderr))
	err := survey.AskOne(q, response, opts...)
	if err == nil {
		return nil
	}
	return fmt.Errorf("could not prompt: %w", err)
}

func (p *surveyPrompter) Input(prompt, defaultValue string) (result string, err error) {
	err = p.ask(&survey.Input{
		Message: prompt,
		Default: defaultValue,
	}, &result)

	return
}

func (p *surveyPrompter) InputHostname() (result string, err error) {
	err = p.ask(
		&survey.Input{
			Message: "GHE hostname:",
		}, &result, survey.WithValidator(func(v interface{}) error {
			return ghinstance.HostnameValidator(v.(string))
		}))

	return
}

func (p *surveyPrompter) Password(prompt string) (result string, err error) {
	err = p.ask(&survey.Password{
		Message: prompt,
	}, &result)

	return
}

func (p *surveyPrompter) Confirm(prompt string, defaultValue bool) (result bool, err error) {
	err = p.ask(&survey.Confirm{
		Message: prompt,
		Default: defaultValue,
	}, &result)

	return
}
func (p *surveyPrompter) MarkdownEditor(message, defaultValue string, blankAllowed bool) (result string, err error) {

	err = p.ask(&surveyext.GhEditor{
		BlankAllowed:  blankAllowed,
		EditorCommand: p.editorCmd,
		Editor: &survey.Editor{
			Message:       message,
			Default:       defaultValue,
			FileName:      "*.md",
			HideDefault:   true,
			AppendDefault: true,
		},
	}, &result)

	return
}

func (p *surveyPrompter) AuthToken() (result string, err error) {
	err = p.ask(&survey.Password{
		Message: "Paste your authentication token:",
	}, &result, survey.WithValidator(survey.Required))

	return
}
