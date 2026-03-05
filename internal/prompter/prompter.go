package prompter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/charmbracelet/huh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/pkg/iostreams"
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
	// MultiSelectWithSearch is MultiSelect with an added search option to the list,
	// prompting the user for text input to filter the options via the searchFunc.
	// Items selected in the search are persisted in the list after subsequent searches.
	// Items passed in persistentOptions are always shown in the list, even when not selected.
	// Unlike MultiSelect, MultiselectWithSearch returns the selected option strings,
	// not their indices, since the list of options is dynamic.
	// The searchFunc has the signature: func(query string) MultiSelectSearchResult.
	// In the returned MultiSelectSearchResult, Keys are the values eventually returned by MultiSelectWithSearch and Labels are what is shown to the user in the prompt.
	MultiSelectWithSearch(prompt, searchPrompt string, defaults []string, persistentOptions []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error)
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

func New(editorCmd string, io *iostreams.IOStreams) Prompter {
	if io.AccessiblePrompterEnabled() {
		return &accessiblePrompter{
			stdin:     io.In,
			stdout:    io.Out,
			stderr:    io.ErrOut,
			editorCmd: editorCmd,
		}
	}

	return &surveyPrompter{
		prompter:  ghPrompter.New(io.In, io.Out, io.ErrOut),
		stdin:     io.In,
		stdout:    io.Out,
		stderr:    io.ErrOut,
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

// addDefaultsToPrompt adds default values to the prompt string.
func (p *accessiblePrompter) addDefaultsToPrompt(prompt string, defaultValues []string) string {
	// Removing empty defaults from the slice.
	defaultValues = slices.DeleteFunc(defaultValues, func(s string) bool {
		return s == ""
	})

	// Pluralizing the prompt if there are multiple default values.
	if len(defaultValues) == 1 {
		prompt = fmt.Sprintf("%s (default: %s)", prompt, defaultValues[0])
	} else if len(defaultValues) > 1 {
		prompt = fmt.Sprintf("%s (defaults: %s)", prompt, strings.Join(defaultValues, ", "))
	}

	// Zero-length defaultValues means return prompt unchanged.
	return prompt
}

func (p *accessiblePrompter) Select(prompt, defaultValue string, options []string) (int, error) {
	var result int

	// Remove invalid default values from the defaults slice.
	if !slices.Contains(options, defaultValue) {
		defaultValue = ""
	}

	prompt = p.addDefaultsToPrompt(prompt, []string{defaultValue})
	formOptions := []huh.Option[int]{}
	for i, o := range options {
		// If this option is the default value, assign its index
		// to the result variable. huh will treat it as a default selection.
		if defaultValue == o {
			result = i
		}
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

	// Remove invalid default values from the defaults slice.
	defaults = slices.DeleteFunc(defaults, func(s string) bool {
		return !slices.Contains(options, s)
	})

	prompt = p.addDefaultsToPrompt(prompt, defaults)
	formOptions := make([]huh.Option[int], len(options))
	for i, o := range options {
		// If this option is in the defaults slice,
		// let's add its index to the result slice and huh
		// will treat it as a default selection.
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

	if err := form.Run(); err != nil {
		return nil, err
	}

	return result, nil
}

func (p *accessiblePrompter) Input(prompt, defaultValue string) (string, error) {
	result := defaultValue
	prompt = p.addDefaultsToPrompt(prompt, []string{defaultValue})
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
	// EchoModePassword is not used as password masking is unsupported in huh.
	// EchoModeNone and EchoModePassword have the same effect of hiding user input.
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModeNone).
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

	if defaultValue {
		prompt = p.addDefaultsToPrompt(prompt, []string{"yes"})
	} else {
		prompt = p.addDefaultsToPrompt(prompt, []string{"no"})
	}

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
	// EchoModeNone and EchoModePassword both result in disabling echo mode
	// as password masking is outside of VT100 spec.
	form := p.newForm(
		huh.NewGroup(
			huh.NewInput().
				EchoMode(huh.EchoModeNone).
				Title("Paste your authentication token:").
				// Note: if this validation fails, the prompt loops.
				Validate(func(input string) error {
					if input == "" {
						return fmt.Errorf("token is required")
					}
					return nil
				}).
				Value(&result),
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

func (p *accessiblePrompter) MultiSelectWithSearch(prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	return multiSelectWithSearch(p, prompt, searchPrompt, defaultValues, persistentValues, searchFunc)
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

func (p *surveyPrompter) MultiSelectWithSearch(prompt string, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	return multiSelectWithSearch(p, prompt, searchPrompt, defaultValues, persistentValues, searchFunc)
}

type MultiSelectSearchResult struct {
	Keys        []string
	Labels      []string
	MoreResults int
	Err         error
}

func multiSelectWithSearch(p Prompter, prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	selectedOptions := defaultValues

	// The optionKeyLabels map is used to uniquely identify optionKeyLabels
	// and provide optional display labels.
	optionKeyLabels := make(map[string]string)
	for _, k := range selectedOptions {
		optionKeyLabels[k] = k
	}

	searchResult := searchFunc("")
	if searchResult.Err != nil {
		return nil, fmt.Errorf("failed to search: %w", searchResult.Err)
	}
	searchResultKeys := searchResult.Keys
	searchResultLabels := searchResult.Labels
	moreResults := searchResult.MoreResults

	for i, k := range searchResultKeys {
		optionKeyLabels[k] = searchResultLabels[i]
	}

	for {
		// Build dynamic option list ->  search sentinel, selections, search results, persistent options.
		optionKeys := make([]string, 0, 1+len(selectedOptions)+len(searchResultKeys)+len(persistentValues))
		optionLabels := make([]string, 0, len(optionKeys))

		// 1. Search sentinel.
		optionKeys = append(optionKeys, "")
		if moreResults > 0 {
			optionLabels = append(optionLabels, fmt.Sprintf("Search (%d more)", moreResults))
		} else {
			optionLabels = append(optionLabels, "Search")
		}

		// 2. Selections
		for _, k := range selectedOptions {
			l := optionKeyLabels[k]

			if l == "" {
				l = k
			}

			optionKeys = append(optionKeys, k)
			optionLabels = append(optionLabels, l)
		}

		// 3. Search results
		for _, k := range searchResultKeys {
			// It's already selected or persistent, if we add here we'll have duplicates.
			if slices.Contains(selectedOptions, k) || slices.Contains(persistentValues, k) {
				continue
			}

			l := optionKeyLabels[k]
			if l == "" {
				l = k
			}
			optionKeys = append(optionKeys, k)
			optionLabels = append(optionLabels, l)
		}

		// 4. Persistent options
		for _, k := range persistentValues {
			if slices.Contains(selectedOptions, k) {
				continue
			}

			l := optionKeyLabels[k]
			if l == "" {
				l = k
			}

			optionKeys = append(optionKeys, k)
			optionLabels = append(optionLabels, l)
		}

		selectedOptionLabels := make([]string, len(selectedOptions))
		for i, k := range selectedOptions {
			l := optionKeyLabels[k]
			if l == "" {
				l = k
			}
			selectedOptionLabels[i] = l
		}

		selectedIdxs, err := p.MultiSelect(prompt, selectedOptionLabels, optionLabels)
		if err != nil {
			return nil, err
		}

		pickedSearch := false
		var newSelectedOptions []string
		for _, idx := range selectedIdxs {
			if idx == 0 { // Search sentinel selected
				pickedSearch = true
				continue
			}

			if idx < 0 || idx >= len(optionKeys) {
				continue
			}

			key := optionKeys[idx]
			if key == "" {
				continue
			}

			newSelectedOptions = append(newSelectedOptions, key)
		}

		selectedOptions = newSelectedOptions
		for _, k := range selectedOptions {
			if _, ok := optionKeyLabels[k]; !ok {
				optionKeyLabels[k] = k
			}
		}

		if pickedSearch {
			query, err := p.Input(searchPrompt, "")
			if err != nil {
				return nil, err
			}

			searchResult := searchFunc(query)
			if searchResult.Err != nil {
				return nil, searchResult.Err
			}
			searchResultKeys = searchResult.Keys
			searchResultLabels = searchResult.Labels
			moreResults = searchResult.MoreResults

			for i, k := range searchResultKeys {
				optionKeyLabels[k] = searchResultLabels[i]
			}

			continue
		}

		return selectedOptions, nil
	}
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
