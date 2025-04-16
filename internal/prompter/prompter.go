package prompter

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/charmbracelet/huh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/surveyext"
	ghPrompter "github.com/cli/go-gh/v2/pkg/prompter"
)

//go:generate moq -rm -out prompter_mock.go . Prompter
type Prompter interface {
	// generic prompts from go-gh

	// Select prompts the user to select an option from a list of options.
	Select(prompt string, defaultValue string, options []string) (int, error)
	// MultiSelect prompts the user to select one or more options from a list of options.
	MultiSelect(prompt string, defaults []string, options []string) ([]int, error)
	// Input prompts the user to enter a string value.
	Input(prompt string, defaultValue string) (string, error)
	// Password prompts the user to enter a password.
	Password(prompt string) (string, error)
	// Confirm prompts the user to confirm an action.
	Confirm(prompt string, defaultValue bool) (bool, error)

	// gh specific prompts

	// AuthToken prompts the user to enter an authentication token.
	AuthToken() (string, error)
	// ConfirmDeletion prompts the user to confirm deletion of a resource by
	// typing the requiredValue.
	ConfirmDeletion(requiredValue string) error
	// InputHostname prompts the user to enter a hostname.
	InputHostname() (string, error)
	// MarkdownEditor prompts the user to edit a markdown document in an editor.
	// If blankAllowed is true, the user can skip the editor and an empty string
	// will be returned.
	MarkdownEditor(prompt string, defaultValue string, blankAllowed bool) (string, error)
}

func New(editorCmd string, stdin ghPrompter.FileReader, stdout ghPrompter.FileWriter, stderr ghPrompter.FileWriter) Prompter {
	accessiblePrompterValue, accessiblePrompterIsSet := os.LookupEnv("GH_ACCESSIBLE_PROMPTER")
	falseyValues := []string{"false", "0", "no", ""}

	if accessiblePrompterIsSet && !slices.Contains(falseyValues, accessiblePrompterValue) {
		return &accessiblePrompter{
			stdin:     stdin,
			stdout:    stdout,
			stderr:    stderr,
			editorCmd: editorCmd,
		}
	}

	return &surveyPrompter{
		prompter:  ghPrompter.New(stdin, stdout, stderr),
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		editorCmd: editorCmd,
	}
}

type accessiblePrompter struct {
	stdin     ghPrompter.FileReader
	stdout    ghPrompter.FileWriter
	stderr    ghPrompter.FileWriter
	editorCmd string
}

func (p *accessiblePrompter) newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeBase16()).
		WithAccessible(true).
		WithInput(p.stdin).
		WithOutput(p.stdout)
}

func (p *accessiblePrompter) Select(prompt, _ string, options []string) (int, error) {
	var result int
	formOptions := []huh.Option[int]{}
	for i, o := range options {
		formOptions = append(formOptions, huh.NewOption(o, i))
	}

	form := p.newForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(prompt).
				Value(&result).
				Options(formOptions...),
		),
	)

	err := form.Run()
	return result, err
}

func (p *accessiblePrompter) MultiSelect(prompt string, defaults []string, options []string) ([]int, error) {
	var result []int
	formOptions := make([]huh.Option[int], len(options))
	for i, o := range options {
		formOptions[i] = huh.NewOption(o, i)
	}

	form := p.newForm(
		huh.NewGroup(
			huh.NewMultiSelect[int]().
				Title(prompt).
				Value(&result).
				Limit(len(options)).
				Options(formOptions...),
		),
	)

	if err := form.Run(); err != nil {
		return nil, err
	}

	return result, nil
}

func (p *accessiblePrompter) Input(prompt, defaultValue string) (string, error) {
	result := defaultValue
	prompt = fmt.Sprintf("%s (%s)", prompt, defaultValue)
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title(prompt).
				Value(&result),
		),
	)

	err := form.Run()
	return result, err
}

func (p *accessiblePrompter) Password(prompt string) (string, error) {
	var result string
	// EchoMode(huh.EchoModePassword) doesn't have any effect in accessible mode.
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title(prompt).
				Value(&result),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}

	return result, nil
}

func (p *accessiblePrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	result := defaultValue
	form := p.newForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(prompt).
				Value(&result),
		),
	)
	if err := form.Run(); err != nil {
		return false, err
	}
	return result, nil
}

func (p *accessiblePrompter) AuthToken() (string, error) {
	var result string
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Paste your authentication token:").
				// Note: if this validation fails, the prompt loops.
				Validate(func(input string) error {
					if input == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}).
				Value(&result),
			// This doesn't have any effect in accessible mode.
			// EchoMode(huh.EchoModePassword),
		),
	)

	err := form.Run()
	return result, err
}

func (p *accessiblePrompter) ConfirmDeletion(requiredValue string) error {
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title(fmt.Sprintf("Type %q to confirm deletion", requiredValue)).
				Validate(func(input string) error {
					if input != requiredValue {
						return fmt.Errorf("You entered: %q", input)
					}
					return nil
				}),
		),
	)

	return form.Run()
}

func (p *accessiblePrompter) InputHostname() (string, error) {
	var result string
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Hostname:").
				Validate(ghinstance.HostnameValidator).
				Value(&result),
		),
	)

	err := form.Run()
	if err != nil {
		return "", err
	}
	return result, nil
}

func (p *accessiblePrompter) MarkdownEditor(prompt, defaultValue string, blankAllowed bool) (string, error) {
	var result string
	skipOption := "skip"
	launchOption := "launch"
	options := []huh.Option[string]{
		huh.NewOption(fmt.Sprintf("Launch %s", surveyext.EditorName(p.editorCmd)), launchOption),
	}
	if blankAllowed {
		options = append(options, huh.NewOption("Skip", skipOption))
	}

	form := p.newForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(prompt).
				Options(options...).
				Value(&result),
		),
	)

	if err := form.Run(); err != nil {
		return "", err
	}

	if result == skipOption {
		return "", nil
	}

	// launchOption was selected
	text, err := surveyext.Edit(p.editorCmd, "*.md", defaultValue, p.stdin, p.stdout, p.stderr)
	if err != nil {
		return "", err
	}

	return text, nil
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
