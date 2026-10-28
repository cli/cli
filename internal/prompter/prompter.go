package prompter

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/surveyext"
)

//go:generate moq -rm -out prompter_mock.go . Prompter
type Prompter interface {
	Select(prompt string, defaultValue string, options []string) (int, error)
	MultiSelect(prompt string, defaults []string, options []string) ([]int, error)
	MultiSelectWithSearch(prompt, searchPrompt string, defaults []string, persistentOptions []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error)
	Input(prompt string, defaultValue string) (string, error)
	Password(prompt string) (string, error)
	Confirm(prompt string, defaultValue bool) (bool, error)
	AuthToken() (string, error)
	ConfirmDeletion(requiredValue string) error
	InputHostname() (string, error)
	MarkdownEditor(prompt string, defaultValue string, blankAllowed bool) (string, error)
}

func New(editorCmd string, io *iostreams.IOStreams) Prompter {
	return &surveyPrompter{
		stdin:     io.In,
		stdout:    io.Out,
		stderr:    io.ErrOut,
		editorCmd: editorCmd,
	}
}

type MultiSelectSearchResult struct {
	Keys        []string
	Labels      []string
	MoreResults int
	Err         error
}

func multiSelectWithSearch(p Prompter, prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	selectedOptions := defaultValues

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
		optionKeys := make([]string, 0, 1+len(selectedOptions)+len(searchResultKeys)+len(persistentValues))
		optionLabels := make([]string, 0, len(optionKeys))

		optionKeys = append(optionKeys, "")
		if moreResults > 0 {
			optionLabels = append(optionLabels, fmt.Sprintf("Search (%d more)", moreResults))
		} else {
			optionLabels = append(optionLabels, "Search")
		}

		for _, k := range selectedOptions {
			l := optionKeyLabels[k]
			if l == "" {
				l = k
			}
			optionKeys = append(optionKeys, k)
			optionLabels = append(optionLabels, l)
		}

		for _, k := range searchResultKeys {
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
			if idx == 0 {
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

type surveyPrompter struct {
	stdin     fileReader
	stdout    fileWriter
	stderr    io.Writer
	editorCmd string
}

type fileReader interface {
	io.Reader
	Fd() uintptr
}

type fileWriter interface {
	io.Writer
	Fd() uintptr
}

func (p *surveyPrompter) Select(prompt, defaultValue string, options []string) (int, error) {
	var result int
	q := &survey.Select{
		Message:  prompt,
		Options:  options,
		PageSize: 20,
		Filter:   latinMatchingFilter,
	}
	if slices.Contains(options, defaultValue) && defaultValue != "" {
		q.Default = defaultValue
	}
	if err := p.ask(q, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (p *surveyPrompter) MultiSelect(prompt string, defaultValues, options []string) ([]int, error) {
	var result []int
	validatedDefaults := []string{}
	for _, defaultValue := range defaultValues {
		if slices.Contains(options, defaultValue) {
			validatedDefaults = append(validatedDefaults, defaultValue)
		}
	}
	q := &survey.MultiSelect{
		Message:  prompt,
		Options:  options,
		PageSize: 20,
		Filter:   latinMatchingFilter,
		Default:  validatedDefaults,
	}
	if err := p.ask(q, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (p *surveyPrompter) MultiSelectWithSearch(prompt, searchPrompt string, defaultValues, persistentValues []string, searchFunc func(string) MultiSelectSearchResult) ([]string, error) {
	return multiSelectWithSearch(p, prompt, searchPrompt, defaultValues, persistentValues, searchFunc)
}

func (p *surveyPrompter) Input(prompt, defaultValue string) (string, error) {
	var result string
	err := p.ask(&survey.Input{Message: prompt, Default: defaultValue}, &result)
	return result, err
}

func (p *surveyPrompter) Password(prompt string) (string, error) {
	var result string
	err := p.ask(&survey.Password{Message: prompt}, &result)
	return result, err
}

func (p *surveyPrompter) Confirm(prompt string, defaultValue bool) (bool, error) {
	var result bool
	err := p.ask(&survey.Confirm{Message: prompt, Default: defaultValue}, &result)
	return result, err
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
		survey.WithValidator(func(val interface{}) error {
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

func latinMatchingFilter(filter, value string, _ int) bool {
	filter = strings.ToLower(filter)
	value = strings.ToLower(value)
	return strings.Contains(value, filter) || strings.Contains(text.RemoveDiacritics(value), filter)
}
