package prompter

import (
	"errors"
	"fmt"
	"slices"

	"charm.land/huh/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/surveyext"
	ghPrompter "github.com/cli/go-gh/v2/pkg/prompter"
)

type huhPrompter struct {
	stdin     ghPrompter.FileReader
	stdout    ghPrompter.FileWriter
	stderr    ghPrompter.FileWriter
	editorCmd string
}

func (p *huhPrompter) newForm(groups ...*huh.Group) *huh.Form {
	return huh.NewForm(groups...).
		WithTheme(huh.ThemeFunc(huh.ThemeBase16)).
		WithInput(p.stdin).
		WithOutput(p.stdout)
}

func (p *huhPrompter) runForm(form *huh.Form) error {
	err := form.Run()
	if errors.Is(err, huh.ErrUserAborted) {
		// TODO(huh-prompter-improvements)
		// It's unfortunate that we take a dependency on survey/terminal here, but our clean cancellation logic
		// in cmd.go expects it. Better would be to have a prompter.Cancelled sentinel error, but then we need to
		// go and change non-experimental code to do so, and I don't think we should take that on right now.
		return terminal.InterruptErr
	}
	return err
}

func (p *huhPrompter) buildSelectForm(prompt, defaultValue string, options []string) (*huh.Form, *int) {
	var result int

	if !slices.Contains(options, defaultValue) {
		defaultValue = ""
	}

	formOptions := make([]huh.Option[int], len(options))
	for i, o := range options {
		if defaultValue == o {
			result = i
		}
		formOptions[i] = huh.NewOption(o, i)
	}

	form := p.newForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title(prompt).
				Value(&result).
				Options(formOptions...),
		),
	)
	return form, &result
}

func (p *huhPrompter) Select(prompt, defaultValue string, options []string) (int, error) {
	form, result := p.buildSelectForm(prompt, defaultValue, options)
	err := p.runForm(form)
	return *result, err
}

func (p *huhPrompter) buildMultiSelectForm(prompt string, defaults []string, options []string) (*huh.Form, *[]int) {
	var result []int

	defaults = slices.DeleteFunc(defaults, func(s string) bool {
		return !slices.Contains(options, s)
	})

	formOptions := make([]huh.Option[int], len(options))
	for i, o := range options {
		if slices.Contains(defaults, o) {
			result = append(result, i)
		}
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
	return form, &result
}

func (p *huhPrompter) MultiSelect(prompt string, defaults []string, options []string) ([]int, error) {
	form, result := p.buildMultiSelectForm(prompt, defaults, options)
	err := p.runForm(form)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (p *huhPrompter) buildMultiSelectWithSearchForm(prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) (*huh.Form, *multiSelectSearchField) {
	field := newMultiSelectSearchField(prompt, searchPrompt, defaultValues, persistentValues, searchFunc)
	form := p.newForm(huh.NewGroup(field))
	return form, field
}

func (p *huhPrompter) MultiSelectWithSearch(prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	form, field := p.buildMultiSelectWithSearchForm(prompt, searchPrompt, defaultValues, persistentValues, searchFunc)
	err := p.runForm(form)
	if err != nil {
		return nil, err
	}
	return field.selectedKeys(), nil
}

func (p *huhPrompter) buildInputForm(prompt, defaultValue string) (*huh.Form, *string) {
	result := defaultValue
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title(prompt).
				Value(&result),
		),
	)
	return form, &result
}

func (p *huhPrompter) Input(prompt, defaultValue string) (string, error) {
	form, result := p.buildInputForm(prompt, defaultValue)
	err := p.runForm(form)
	return *result, err
}

func (p *huhPrompter) buildPasswordForm(prompt string) (*huh.Form, *string) {
	var result string
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title(prompt).
				Value(&result),
		),
	)
	return form, &result
}

func (p *huhPrompter) Password(prompt string) (string, error) {
	form, result := p.buildPasswordForm(prompt)
	err := p.runForm(form)
	if err != nil {
		return "", err
	}
	return *result, nil
}

func (p *huhPrompter) buildConfirmForm(prompt string, defaultValue bool) (*huh.Form, *bool) {
	result := defaultValue
	form := p.newForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(prompt).
				Value(&result),
		),
	)
	return form, &result
}

func (p *huhPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	form, result := p.buildConfirmForm(prompt, defaultValue)
	err := p.runForm(form)
	if err != nil {
		return false, err
	}
	return *result, nil
}

func (p *huhPrompter) buildAuthTokenForm() (*huh.Form, *string) {
	var result string
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModePassword).
				Title("Paste your authentication token:").
				Validate(func(input string) error {
					if input == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}).
				Value(&result),
		),
	)
	return form, &result
}

func (p *huhPrompter) AuthToken() (string, error) {
	form, result := p.buildAuthTokenForm()
	err := p.runForm(form)
	return *result, err
}

func (p *huhPrompter) buildConfirmDeletionForm(requiredValue string) *huh.Form {
	return p.newForm(
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
}

func (p *huhPrompter) ConfirmDeletion(requiredValue string) error {
	return p.runForm(p.buildConfirmDeletionForm(requiredValue))
}

func (p *huhPrompter) buildInputHostnameForm() (*huh.Form, *string) {
	var result string
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Hostname:").
				Validate(ghinstance.HostnameValidator).
				Value(&result),
		),
	)
	return form, &result
}

func (p *huhPrompter) InputHostname() (string, error) {
	form, result := p.buildInputHostnameForm()
	err := p.runForm(form)
	if err != nil {
		return "", err
	}
	return *result, nil
}

func (p *huhPrompter) buildMarkdownEditorForm(prompt string, blankAllowed bool) (*huh.Form, *string) {
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
	return form, &result
}

func (p *huhPrompter) MarkdownEditor(prompt, defaultValue string, blankAllowed bool) (string, error) {
	form, result := p.buildMarkdownEditorForm(prompt, blankAllowed)
	err := p.runForm(form)
	if err != nil {
		return "", err
	}

	if *result == "skip" {
		return "", nil
	}

	text, err := surveyext.Edit(p.editorCmd, "*.md", defaultValue, p.stdin, p.stdout, p.stderr)
	if err != nil {
		return "", err
	}

	return text, nil
}
