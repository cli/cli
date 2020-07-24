package prompt

import "github.com/AlecAivazis/survey/v2"

var Confirm = func(prompt string, result *bool) error {
	p := &survey.Confirm{
		Message: prompt,
		Default: true,
	}
	return survey.AskOne(p, result)
}
