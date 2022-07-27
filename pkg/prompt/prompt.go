package prompt

import "github.com/AlecAivazis/survey/v2"

// Deprecated: use PrompterMock
func StubConfirm(result bool) func() {
	orig := Confirm
	Confirm = func(_ string, r *bool) error {
		*r = result
		return nil
	}
	return func() {
		Confirm = orig
	}
}

// Deprecated: use Prompter
var Confirm = func(prompt string, result *bool) error {
	p := &survey.Confirm{
		Message: prompt,
		Default: true,
	}
	return SurveyAskOne(p, result)
}

// Deprecated: use Prompter
var SurveyAskOne = func(p survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(p, response, opts...)
}

// Deprecated: use Prompter
var SurveyAsk = func(qs []*survey.Question, response interface{}, opts ...survey.AskOpt) error {
	return survey.Ask(qs, response, opts...)
}
