package survey

import (
	"errors"

	"github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
)

/*
Select is a prompt that presents a list of various options to the user
for them to select using the arrow keys and enter. Response type is a string.

	color := ""
	prompt := &survey.Select{
		Message: "Choose a color:",
		Options: []string{"red", "blue", "green"},
	}
	survey.AskOne(prompt, &color)
*/
type Select struct {
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
	useDefault    bool
	showingHelp   bool
}

// SelectTemplateData is the data available to the templates when processing
type SelectTemplateData struct {
	Select
	PageEntries   []core.OptionAnswer
	SelectedIndex int
	Answer        string
	ShowAnswer    bool
	ShowHelp      bool
	Config        *PromptConfig
}

var SelectQuestionTemplate = `
{{- if .ShowHelp }}{{- color .Config.Icons.Help.Format }}{{ .Config.Icons.Help.Text }} {{ .Help }}{{color "reset"}}{{"\n"}}{{end}}
{{- color .Config.Icons.Question.Format }}{{ .Config.Icons.Question.Text }} {{color "reset"}}
{{- color "default+hb"}}{{ .Message }}{{ .FilterMessage }}{{color "reset"}}
{{- if .ShowAnswer}}{{color "cyan"}} {{.Answer}}{{color "reset"}}{{"\n"}}
{{- else}}
  {{- "  "}}{{- color "cyan"}}[Use arrows to move, type to filter{{- if and .Help (not .ShowHelp)}}, {{ .Config.HelpInput }} for more help{{end}}]{{color "reset"}}
  {{- "\n"}}
  {{- range $ix, $choice := .PageEntries}}
    {{- if eq $ix $.SelectedIndex }}{{color $.Config.Icons.SelectFocus.Format }}{{ $.Config.Icons.SelectFocus.Text }} {{else}}{{color "default"}}  {{end}}
    {{- $choice.Value}}
    {{- color "reset"}}{{"\n"}}
  {{- end}}
{{- end}}`

// OnChange is called on every keypress.
func (s *Select) OnChange(key rune, config *PromptConfig) bool {
	options := s.filterOptions(config)
	oldFilter := s.filter

	// if the user pressed the enter key and the index is a valid option
	if key == terminal.KeyEnter || key == '\n' {
		// if the selected index is a valid option
		if len(options) > 0 && s.selectedIndex < len(options) {

			// we're done (stop prompting the user)
			return true
		}

		// we're not done (keep prompting)
		return false

		// if the user pressed the up arrow or 'k' to emulate vim
	} else if key == terminal.KeyArrowUp || (s.VimMode && key == 'k') && len(options) > 0 {
		s.useDefault = false

		// if we are at the top of the list
		if s.selectedIndex == 0 {
			// start from the button
			s.selectedIndex = len(options) - 1
		} else {
			// otherwise we are not at the top of the list so decrement the selected index
			s.selectedIndex--
		}

		// if the user pressed down or 'j' to emulate vim
	} else if key == terminal.KeyArrowDown || (s.VimMode && key == 'j') && len(options) > 0 {
		s.useDefault = false
		// if we are at the bottom of the list
		if s.selectedIndex == len(options)-1 {
			// start from the top
			s.selectedIndex = 0
		} else {
			// increment the selected index
			s.selectedIndex++
		}
		// only show the help message if we have one
	} else if string(key) == config.HelpInput && s.Help != "" {
		s.showingHelp = true
		// if the user wants to toggle vim mode on/off
	} else if key == terminal.KeyEscape {
		s.VimMode = !s.VimMode
		// if the user hits any of the keys that clear the filter
	} else if key == terminal.KeyDeleteWord || key == terminal.KeyDeleteLine {
		s.filter = ""
		// if the user is deleting a character in the filter
	} else if key == terminal.KeyDelete || key == terminal.KeyBackspace {
		// if there is content in the filter to delete
		if s.filter != "" {
			// subtract a line from the current filter
			s.filter = s.filter[0 : len(s.filter)-1]
			// we removed the last value in the filter
		}
	} else if key >= terminal.KeySpace {
		s.filter += string(key)
		// make sure vim mode is disabled
		s.VimMode = false
		// make sure that we use the current value in the filtered list
		s.useDefault = false
	}

	s.FilterMessage = ""
	if s.filter != "" {
		s.FilterMessage = " " + s.filter
	}
	if oldFilter != s.filter {
		// filter changed
		options = s.filterOptions(config)
		if len(options) > 0 && len(options) <= s.selectedIndex {
			s.selectedIndex = len(options) - 1
		}
	}

	// figure out the options and index to render
	// figure out the page size
	pageSize := s.PageSize
	// if we dont have a specific one
	if pageSize == 0 {
		// grab the global value
		pageSize = config.PageSize
	}

	// TODO if we have started filtering and were looking at the end of a list
	// and we have modified the filter then we should move the page back!
	opts, idx := paginate(pageSize, options, s.selectedIndex)

	// render the options
	s.Render(
		SelectQuestionTemplate,
		SelectTemplateData{
			Select:        *s,
			SelectedIndex: idx,
			ShowHelp:      s.showingHelp,
			PageEntries:   opts,
			Config:        config,
		},
	)

	// keep prompting
	return false
}

func (s *Select) filterOptions(config *PromptConfig) []core.OptionAnswer {
	// the filtered list
	answers := []core.OptionAnswer{}

	// if there is no filter applied
	if s.filter == "" {
		return core.OptionAnswerList(s.Options)
	}

	// the filter to apply
	filter := s.Filter
	if filter == nil {
		filter = config.Filter
	}

	//
	for i, opt := range s.Options {
		// i the filter says to include the option
		if filter(s.filter, opt, i) {
			answers = append(answers, core.OptionAnswer{
				Index: i,
				Value: opt,
			})
		}
	}

	// return the list of answers
	return answers
}

func (s *Select) Prompt(config *PromptConfig) (interface{}, error) {
	// if there are no options to render
	if len(s.Options) == 0 {
		// we failed
		return "", errors.New("please provide options to select from")
	}

	// start off with the first option selected
	sel := 0
	// if there is a default
	if s.Default != "" {
		// find the choice
		for i, opt := range s.Options {
			// if the option corresponds to the default
			if opt == s.Default {
				// we found our initial value
				sel = i
				// stop looking
				break
			}
		}
	}
	// save the selected index
	s.selectedIndex = sel

	// figure out the page size
	pageSize := s.PageSize
	// if we dont have a specific one
	if pageSize == 0 {
		// grab the global value
		pageSize = config.PageSize
	}

	// figure out the options and index to render
	opts, idx := paginate(pageSize, core.OptionAnswerList(s.Options), sel)

	// ask the question
	err := s.Render(
		SelectQuestionTemplate,
		SelectTemplateData{
			Select:        *s,
			PageEntries:   opts,
			SelectedIndex: idx,
			Config:        config,
		},
	)
	if err != nil {
		return "", err
	}

	// by default, use the default value
	s.useDefault = true

	rr := s.NewRuneReader()
	rr.SetTermMode()
	defer rr.RestoreTermMode()

	cursor := s.NewCursor()
	cursor.Hide()       // hide the cursor
	defer cursor.Show() // show the cursor when we're done

	// start waiting for input
	for {
		r, _, err := rr.ReadRune()
		if err != nil {
			return "", err
		}
		if r == terminal.KeyInterrupt {
			return "", terminal.InterruptErr
		}
		if r == terminal.KeyEndTransmission {
			break
		}
		if s.OnChange(r, config) {
			break
		}
	}
	options := s.filterOptions(config)
	s.filter = ""
	s.FilterMessage = ""

	// the index to report
	var val string
	// if we are supposed to use the default value
	if s.useDefault || s.selectedIndex >= len(options) {
		// if there is a default value
		if s.Default != nil {
			// if the default is a string
			if defaultString, ok := s.Default.(string); ok {
				// use the default value
				val = defaultString
				// the default value could also be an interpret which is interpretted as the index
			} else if defaultIndex, ok := s.Default.(int); ok {
				val = s.Options[defaultIndex]
			} else {
				return val, errors.New("default value of select must be an int or string")
			}
		} else if len(options) > 0 {
			// there is no default value so use the first
			val = options[0].Value
		}
		// otherwise the selected index points to the value
	} else if s.selectedIndex < len(options) {
		// the
		val = options[s.selectedIndex].Value
	}

	// now that we have the value lets go hunt down the right index to return
	idx = -1
	for i, optionValue := range s.Options {
		if optionValue == val {
			idx = i
		}
	}

	return core.OptionAnswer{Value: val, Index: idx}, err
}

func (s *Select) Cleanup(config *PromptConfig, val interface{}) error {
	return s.Render(
		SelectQuestionTemplate,
		SelectTemplateData{
			Select:     *s,
			Answer:     val.(core.OptionAnswer).Value,
			ShowAnswer: true,
			Config:     config,
		},
	)
}
