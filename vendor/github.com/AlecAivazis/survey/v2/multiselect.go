package survey

import (
	"errors"
	"fmt"

	"github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
)

/*
MultiSelect is a prompt that presents a list of various options to the user
for them to select using the arrow keys and enter. Response type is a slice of strings.

	days := []string{}
	prompt := &survey.MultiSelect{
		Message: "What days do you prefer:",
		Options: []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"},
	}
	survey.AskOne(prompt, &days)
*/
type MultiSelect struct {
	Renderer
	Message       string
	Options       []string
	Default       interface{}
	Help          string
	PageSize      int
	VimMode       bool
	FilterMessage string
	Filter        func(filter string, value string, index int) bool
	filter        string
	selectedIndex int
	checked       map[int]bool
	showingHelp   bool
}

// data available to the templates when processing
type MultiSelectTemplateData struct {
	MultiSelect
	Answer        string
	ShowAnswer    bool
	Checked       map[int]bool
	SelectedIndex int
	ShowHelp      bool
	PageEntries   []core.OptionAnswer
	Config        *PromptConfig
}

var MultiSelectQuestionTemplate = `
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}{{color "cyan"}} {{.Answer}}{{color "reset"}}{{"\n"}}
{{- else }}
	{{- "  "}}{{- color "cyan"}}[Use arrows to move, space to select, type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $option := .PageEntries}}
    {{- if eq $ix $.SelectedIndex }}{{color $.Config.Icons.SelectFocus.Format }}{{ $.Config.Icons.SelectFocus.Text }}{{color "reset"}}{{else}} {{end}}
    {{- if index $.Checked $option.Index }}{{color $.Config.Icons.MarkedOption.Format }} {{ $.Config.Icons.MarkedOption.Text }} {{else}}{{color $.Config.Icons.UnmarkedOption.Format }} {{ $.Config.Icons.UnmarkedOption.Text }} {{end}}
    {{- color "reset"}}
    {{- " "}}{{$option.Value}}{{"\n"}}
  {{- end}}
{{- end}}`

// OnChange is called on every keypress.
func (m *MultiSelect) OnChange(key rune, config *PromptConfig) {
	options := m.filterOptions(config)
	oldFilter := m.filter

	if key == terminal.KeyArrowUp || (m.VimMode && key == 'k') {
		// if we are at the top of the list
		if m.selectedIndex == 0 {
			// go to the bottom
			m.selectedIndex = len(options) - 1
		} else {
			// decrement the selected index
			m.selectedIndex--
		}
	} else if key == terminal.KeyArrowDown || (m.VimMode && key == 'j') {
		// if we are at the bottom of the list
		if m.selectedIndex == len(options)-1 {
			// start at the top
			m.selectedIndex = 0
		} else {
			// increment the selected index
			m.selectedIndex++
		}
		// if the user pressed down and there is room to move
	} else if key == terminal.KeySpace {
		// the option they have selected
		if m.selectedIndex < len(options) {
			selectedOpt := options[m.selectedIndex]

			// if we haven't seen this index before
			if old, ok := m.checked[selectedOpt.Index]; !ok {
				// set the value to true
				m.checked[selectedOpt.Index] = true
			} else {
				// otherwise just invert the current value
				m.checked[selectedOpt.Index] = !old
			}
			m.filter = ""
		}
		// only show the help message if we have one to show
	} else if string(key) == config.HelpInput && m.Help != "" {
		m.showingHelp = true
	} else if key == terminal.KeyEscape {
		m.VimMode = !m.VimMode
	} else if key == terminal.KeyDeleteWord || key == terminal.KeyDeleteLine {
		m.filter = ""
	} else if key == terminal.KeyDelete || key == terminal.KeyBackspace {
		if m.filter != "" {
			m.filter = m.filter[0 : len(m.filter)-1]
		}
	} else if key >= terminal.KeySpace {
		m.filter += string(key)
		m.VimMode = false
	}

	m.FilterMessage = ""
	if m.filter != "" {
		m.FilterMessage = " " + m.filter
	}
	if oldFilter != m.filter {
		// filter changed
		options = m.filterOptions(config)
		if len(options) > 0 && len(options) <= m.selectedIndex {
			m.selectedIndex = len(options) - 1
		}
	}
	// paginate the options
	// figure out the page size
	pageSize := m.PageSize
	// if we dont have a specific one
	if pageSize == 0 {
		// grab the global value
		pageSize = config.PageSize
	}

	// TODO if we have started filtering and were looking at the end of a list
	// and we have modified the filter then we should move the page back!
	opts, idx := paginate(pageSize, options, m.selectedIndex)

	// render the options
	m.Render(
		MultiSelectQuestionTemplate,
		MultiSelectTemplateData{
			MultiSelect:   *m,
			SelectedIndex: idx,
			Checked:       m.checked,
			ShowHelp:      m.showingHelp,
			PageEntries:   opts,
			Config:        config,
		},
	)
}

func (m *MultiSelect) filterOptions(config *PromptConfig) []core.OptionAnswer {
	// the filtered list
	answers := []core.OptionAnswer{}

	// if there is no filter applied
	if m.filter == "" {
		// return all of the options
		return core.OptionAnswerList(m.Options)
	}

	// the filter to apply
	filter := m.Filter
	if filter == nil {
		filter = config.Filter
	}

	// apply the filter to each option
	for i, opt := range m.Options {
		// i the filter says to include the option
		if filter(m.filter, opt, i) {
			answers = append(answers, core.OptionAnswer{
				Index: i,
				Value: opt,
			})
		}
	}

	// we're done here
	return answers
}

func (m *MultiSelect) Prompt(config *PromptConfig) (interface{}, error) {
	// compute the default state
	m.checked = make(map[int]bool)
	// if there is a default
	if m.Default != nil {
		// if the default is string values
		if defaultValues, ok := m.Default.([]string); ok {
			for _, dflt := range defaultValues {
				for i, opt := range m.Options {
					// if the option corresponds to the default
					if opt == dflt {
						// we found our initial value
						m.checked[i] = true
						// stop looking
						break
					}
				}
			}
			// if the default value is index values
		} else if defaultIndices, ok := m.Default.([]int); ok {
			// go over every index we need to enable by default
			for _, idx := range defaultIndices {
				// and enable it
				m.checked[idx] = true
			}
		}
	}

	// if there are no options to render
	if len(m.Options) == 0 {
		// we failed
		return "", errors.New("please provide options to select from")
	}

	// figure out the page size
	pageSize := m.PageSize
	// if we dont have a specific one
	if pageSize == 0 {
		// grab the global value
		pageSize = config.PageSize
	}
	// paginate the options
	// build up a list of option answers
	opts, idx := paginate(pageSize, core.OptionAnswerList(m.Options), m.selectedIndex)

	cursor := m.NewCursor()
	cursor.Hide()       // hide the cursor
	defer cursor.Show() // show the cursor when we're done

	// ask the question
	err := m.Render(
		MultiSelectQuestionTemplate,
		MultiSelectTemplateData{
			MultiSelect:   *m,
			SelectedIndex: idx,
			Checked:       m.checked,
			PageEntries:   opts,
			Config:        config,
		},
	)
	if err != nil {
		return "", err
	}

	rr := m.NewRuneReader()
	rr.SetTermMode()
	defer rr.RestoreTermMode()

	// start waiting for input
	for {
		r, _, _ := rr.ReadRune()
		if r == '\r' || r == '\n' {
			break
		}
		if r == terminal.KeyInterrupt {
			return "", terminal.InterruptErr
		}
		if r == terminal.KeyEndTransmission {
			break
		}
		m.OnChange(r, config)
	}
	m.filter = ""
	m.FilterMessage = ""

	answers := []core.OptionAnswer{}
	for i, option := range m.Options {
		if val, ok := m.checked[i]; ok && val {
			answers = append(answers, core.OptionAnswer{Value: option, Index: i})
		}
	}

	return answers, nil
}

// Cleanup removes the options section, and renders the ask like a normal question.
func (m *MultiSelect) Cleanup(config *PromptConfig, val interface{}) error {
	// the answer to show
	answer := ""
	for _, ans := range val.([]core.OptionAnswer) {
		answer = fmt.Sprintf("%s, %s", answer, ans.Value)
	}

	// if we answered anything
	if len(answer) > 2 {
		// remove the precending commas
		answer = answer[2:]
	}

	// execute the output summary template with the answer
	return m.Render(
		MultiSelectQuestionTemplate,
		MultiSelectTemplateData{
			MultiSelect:   *m,
			SelectedIndex: m.selectedIndex,
			Checked:       m.checked,
			Answer:        answer,
			ShowAnswer:    true,
			Config:        config,
		},
	)
}
