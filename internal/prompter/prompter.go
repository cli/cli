package prompter

import (
	"fmt"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/surveyext"
	ghPrompter "github.com/cli/go-gh/v2/pkg/prompter"
)

//go:generate moq -rm -out prompter_mock.go . Prompter
type Prompter interface {
	// generic prompts from go-gh
	Select(string, string, []string) (int, error)
	MultiSelect(prompt string, defaults []string, options []string) ([]int, error)
	Input(string, string) (string, error)
	Password(string) (string, error)
	Confirm(string, bool) (bool, error)

	// gh specific prompts
	AuthToken() (string, error)
	ConfirmDeletion(string) error
	InputHostname() (string, error)
	MarkdownEditor(string, string, bool) (string, error)
}

func New(editorCmd string, stdin ghPrompter.FileReader, stdout ghPrompter.FileWriter, stderr ghPrompter.FileWriter) Prompter {
	return &surveyPrompter{
		prompter:  ghPrompter.New(stdin, stdout, stderr),
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		editorCmd: editorCmd,
	}
}

type surveyPrompter struct {
	prompter  *ghPrompter.Prompter
	stdin     ghPrompter.FileReader
	stdout    ghPrompter.FileWriter
	stderr    ghPrompter.FileWriter
	editorCmd string
}

func (p *surveyPrompter) Select(prompt, defaultValue string, options []string) (int, error) {
	return p.prompter.Select(prompt, defaultValue, options)
}

func (p *surveyPrompter) MultiSelect(prompt string, defaultValues, options []string) ([]int, error) {
	return p.prompter.MultiSelect(prompt, defaultValues, options)
}

func (p *surveyPrompter) Input(prompt, defaultValue string) (string, error) {
	return p.prompter.Input(prompt, defaultValue)
}

func (p *surveyPrompter) Password(prompt string) (string, error) {
	return p.prompter.Password(prompt)
}

func (p *surveyPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	return p.prompter.Confirm(prompt, defaultValue)
}

func (p *surveyPrompter) AuthToken() (string, error) {
	var result string
	err := p.ask(&survey.Password{
		Message: "Paste your authentication token:",
	}, &result, survey.WithValidator(survey.Required))
	return result, err
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

func (p *surveyPrompter) InputHostname() (string, error) {
	var result string
	err := p.ask(
		&survey.Input{
			Message: "Hostname:",
		}, &result, survey.WithValidator(func(v interface{}) error {
			return ghinstance.HostnameValidator(v.(string))
		}))
	return result, err
}

func (p *surveyPrompter) MarkdownEditor(prompt, defaultValue string, blankAllowed bool) (string, error) {
	var result string
	err := p.ask(&surveyext.GhEditor{
		BlankAllowed:  blankAllowed,
		EditorCommand: p.editorCmd,
		Editor: &survey.Editor{
			Message:       prompt,
			Default:       defaultValue,
			FileName:      "*.md",
			HideDefault:   true,
			AppendDefault: true,
		},
	}, &result)
	return result, err
}

func (p *surveyPrompter) ask(q survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	opts = append(opts, survey.WithStdio(p.stdin, p.stdout, p.stderr))
	err := survey.AskOne(q, response, opts...)
	if err == nil {
		return nil
	}
	return fmt.Errorf("could not prompt: %w", err)
}
