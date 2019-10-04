package survey

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2/core"
	"github.com/AlecAivazis/survey/v2/terminal"
)

// DefaultAskOptions is the default options on ask, using the OS stdio.
func defaultAskOptions() *AskOptions {
	return &AskOptions{
		Stdio: terminal.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		},
		PromptConfig: PromptConfig{
			PageSize:  7,
			HelpInput: "?",
			Icons: IconSet{
				Error: Icon{
					Text:   "X",
					Format: "red",
				},
				Help: Icon{
					Text:   "?",
					Format: "cyan",
				},
				Question: Icon{
					Text:   "?",
					Format: "green+hb",
				},
				MarkedOption: Icon{
					Text:   "[x]",
					Format: "green",
				},
				UnmarkedOption: Icon{
					Text:   "[ ]",
					Format: "default+hb",
				},
				SelectFocus: Icon{
					Text:   ">",
					Format: "cyan+b",
				},
			},
			Filter: func(filter string, value string, index int) (include bool) {
				filter = strings.ToLower(filter)

				// include this option if it matches
				return strings.Contains(strings.ToLower(value), filter)
			},
		},
	}
}
func defaultPromptConfig() *PromptConfig {
	return &defaultAskOptions().PromptConfig
}

func defaultIcons() *IconSet {
	return &defaultPromptConfig().Icons
}

// OptionAnswer is an ergonomic alias for core.OptionAnswer
type OptionAnswer = core.OptionAnswer

// Icon holds the text and format to show for a particular icon
type Icon struct {
	Text   string
	Format string
}

// IconSet holds the icons to use for various prompts
type IconSet struct {
	HelpInput      Icon
	Error          Icon
	Help           Icon
	Question       Icon
	MarkedOption   Icon
	UnmarkedOption Icon
	SelectFocus    Icon
}

// Validator is a function passed to a Question after a user has provided a response.
// If the function returns an error, then the user will be prompted again for another
// response.
type Validator func(ans interface{}) error

// Transformer is a function passed to a Question after a user has provided a response.
// The function can be used to implement a custom logic that will result to return
// a different representation of the given answer.
//
// Look `TransformString`, `ToLower` `Title` and `ComposeTransformers` for more.
type Transformer func(ans interface{}) (newAns interface{})

// Question is the core data structure for a survey questionnaire.
type Question struct {
	Name      string
	Prompt    Prompt
	Validate  Validator
	Transform Transformer
}

// PromptConfig holds the global configuration for a prompt
type PromptConfig struct {
	PageSize  int
	Icons     IconSet
	HelpInput string
	Filter    func(filter string, option string, index int) bool
}

// Prompt is the primary interface for the objects that can take user input
// and return a response.
type Prompt interface {
	Prompt(config *PromptConfig) (interface{}, error)
	Cleanup(*PromptConfig, interface{}) error
	Error(*PromptConfig, error) error
}

// PromptAgainer Interface for Prompts that support prompting again after invalid input
type PromptAgainer interface {
	PromptAgain(config *PromptConfig, invalid interface{}, err error) (interface{}, error)
}

// AskOpt allows setting optional ask options.
type AskOpt func(options *AskOptions) error

// AskOptions provides additional options on ask.
type AskOptions struct {
	Stdio        terminal.Stdio
	Validators   []Validator
	PromptConfig PromptConfig
}

// WithStdio specifies the standard input, output and error files survey
// interacts with. By default, these are os.Stdin, os.Stdout, and os.Stderr.
func WithStdio(in terminal.FileReader, out terminal.FileWriter, err io.Writer) AskOpt {
	return func(options *AskOptions) error {
		options.Stdio.In = in
		options.Stdio.Out = out
		options.Stdio.Err = err
		return nil
	}
}

// WithFilter specifies the default filter to use when asking questions.
func WithFilter(filter func(filter string, value string, index int) (include bool)) AskOpt {
	return func(options *AskOptions) error {
		// save the filter internally
		options.PromptConfig.Filter = filter

		return nil
	}
}

// WithValidator specifies a validator to use while prompting the user
func WithValidator(v Validator) AskOpt {
	return func(options *AskOptions) error {
		// add the provided validator to the list
		options.Validators = append(options.Validators, v)

		// nothing went wrong
		return nil
	}
}

type wantsStdio interface {
	WithStdio(terminal.Stdio)
}

// WithPageSize sets the default page size used by prompts
func WithPageSize(pageSize int) AskOpt {
	return func(options *AskOptions) error {
		// set the page size
		options.PromptConfig.PageSize = pageSize

		// nothing went wrong
		return nil
	}
}

// WithHelpInput changes the character that prompts look for to give the user helpful information.
func WithHelpInput(r rune) AskOpt {
	return func(options *AskOptions) error {
		// set the input character
		options.PromptConfig.HelpInput = string(r)

		// nothing went wrong
		return nil
	}
}

// WithIcons sets the icons that will be used when prompting the user
func WithIcons(setIcons func(*IconSet)) AskOpt {
	return func(options *AskOptions) error {
		// update the default icons with whatever the user says
		setIcons(&options.PromptConfig.Icons)

		// nothing went wrong
		return nil
	}
}

/*
AskOne performs the prompt for a single prompt and asks for validation if required.
Response types should be something that can be casted from the response type designated
in the documentation. For example:

	name := ""
	prompt := &survey.Input{
		Message: "name",
	}

	survey.AskOne(prompt, &name)

*/
func AskOne(p Prompt, response interface{}, opts ...AskOpt) error {
	err := Ask([]*Question{{Prompt: p}}, response, opts...)
	if err != nil {
		return err
	}

	return nil
}

/*
Ask performs the prompt loop, asking for validation when appropriate. The response
type can be one of two options. If a struct is passed, the answer will be written to
the field whose name matches the Name field on the corresponding question. Field types
should be something that can be casted from the response type designated in the
documentation. Note, a survey tag can also be used to identify a Otherwise, a
map[string]interface{} can be passed, responses will be written to the key with the
matching name. For example:

	qs := []*survey.Question{
		{
			Name:     "name",
			Prompt:   &survey.Input{Message: "What is your name?"},
			Validate: survey.Required,
			Transform: survey.Title,
		},
	}

	answers := struct{ Name string }{}


	err := survey.Ask(qs, &answers)
*/
func Ask(qs []*Question, response interface{}, opts ...AskOpt) error {
	// build up the configuration options
	options := defaultAskOptions()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(options); err != nil {
			return err
		}
	}

	// if we weren't passed a place to record the answers
	if response == nil {
		// we can't go any further
		return errors.New("cannot call Ask() with a nil reference to record the answers")
	}

	// go over every question
	for _, q := range qs {
		// If Prompt implements controllable stdio, pass in specified stdio.
		if p, ok := q.Prompt.(wantsStdio); ok {
			p.WithStdio(options.Stdio)
		}

		// grab the user input and save it
		ans, err := q.Prompt.Prompt(&options.PromptConfig)
		// if there was a problem
		if err != nil {
			return err
		}

		// build up a list of validators that we have to apply to this question
		validators := []Validator{}

		// make sure to include the question specific one
		if q.Validate != nil {
			validators = append(validators, q.Validate)
		}
		// add any "global" validators
		for _, validator := range options.Validators {
			validators = append(validators, validator)
		}

		// apply every validator to thte response
		for _, validator := range validators {
			// wait for a valid response
			for invalid := validator(ans); invalid != nil; invalid = validator(ans) {
				err := q.Prompt.Error(&options.PromptConfig, invalid)
				// if there was a problem
				if err != nil {
					return err
				}

				// ask for more input
				if promptAgainer, ok := q.Prompt.(PromptAgainer); ok {
					ans, err = promptAgainer.PromptAgain(&options.PromptConfig, ans, invalid)
				} else {
					ans, err = q.Prompt.Prompt(&options.PromptConfig)
				}
				// if there was a problem
				if err != nil {
					return err
				}
			}
		}

		if q.Transform != nil {
			// check if we have a transformer available, if so
			// then try to acquire the new representation of the
			// answer, if the resulting answer is not nil.
			if newAns := q.Transform(ans); newAns != nil {
				ans = newAns
			}
		}

		// tell the prompt to cleanup with the validated value
		q.Prompt.Cleanup(&options.PromptConfig, ans)

		// if something went wrong
		if err != nil {
			// stop listening
			return err
		}

		// add it to the map
		err = core.WriteAnswer(response, q.Name, ans)
		// if something went wrong
		if err != nil {
			return err
		}

	}

	// return the response
	return nil
}

// paginate returns a single page of choices given the page size, the total list of
// possible choices, and the current selected index in the total list.
func paginate(pageSize int, choices []core.OptionAnswer, sel int) ([]core.OptionAnswer, int) {
	var start, end, cursor int

	if len(choices) < pageSize {
		// if we dont have enough options to fill a page
		start = 0
		end = len(choices)
		cursor = sel

	} else if sel < pageSize/2 {
		// if we are in the first half page
		start = 0
		end = pageSize
		cursor = sel

	} else if len(choices)-sel-1 < pageSize/2 {
		// if we are in the last half page
		start = len(choices) - pageSize
		end = len(choices)
		cursor = sel - start

	} else {
		// somewhere in the middle
		above := pageSize / 2
		below := pageSize - above

		cursor = pageSize / 2
		start = sel - above
		end = sel + below
	}

	// return the subset we care about and the index
	return choices[start:end], cursor
}
