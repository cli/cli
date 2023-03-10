package prompter

import (
	"fmt"
	"io"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/surveyext"
)

//go:generate moq -rm -out prompter_mock.go . Prompter
type Prompter interface {
	Select(string, string, []string) (int, error)
	MultiSelect(string, string, []string) ([]string, error)
	Input(string, string) (string, error)
	InputHostname() (string, error)
	Password(string) (string, error)
	AuthToken() (string, error)
	Confirm(string, bool) (bool, error)
	ConfirmDeletion(string) error
	MarkdownEditor(string, string, bool) (string, error)
}

type fileWriter interface {
	io.Writer
	Fd() uintptr
}

type fileReader interface {
	io.Reader
	Fd() uintptr
}

func New(editorCmd string, stdin fileReader, stdout fileWriter, stderr io.Writer) Prompter {
	return &surveyPrompter{
		editorCmd: editorCmd,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
	}
}

type surveyPrompter struct {
	editorCmd string
	stdin     fileReader
	stdout    fileWriter
	stderr    io.Writer
}

func (p *surveyPrompter) Select(message, defaultValue string, options []string) (result int, err error) {
	q := &survey.Select{
		Message:  message,
		Options:  options,
		PageSize: 20,
	}

	if defaultValue != "" {
		// in some situations, defaultValue ends up not being a valid option; do
		// not set default in that case as it will make survey panic
		for _, o := range options {
			if o == defaultValue {
				q.Default = defaultValue
				break
			}
		}
	}

	err = p.ask(q, &result)

	return
}

func (p *surveyPrompter) MultiSelect(message, defaultValue string, options []string) (result []string, err error) {
	q := &survey.MultiSelect{
		Message:  message,
		Options:  options,
		PageSize: 20,
	}

	if defaultValue != "" {
		q.Default = defaultValue
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

func (p *surveyPrompter) ConfirmDeletion(requiredValue string) error {
	var result string
	return p.ask(
		&survey.Input{
			Message: fmt.Sprintf("Type %s to confirm deletion:", requiredValue),
		},
		&result,
		survey.WithValidator(
			func(val interface{}) error {
				if str := val.(string); !strings.EqualFold(str, requiredValue) {
					return fmt.Errorf("You entered %s", str)
				}
				return nil
			}))
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
