package listcmd

import (
	"github.com/cli/cli/v2/internal/prompter"
)

type ConsoleUserPrompter struct {
	Prompter prompter.Prompter
}

func (p ConsoleUserPrompter) PromptForUser() (User, error) {
	u, err := p.Prompter.Input("Which user do you want to target?", "")
	if err != nil {
		return User(""), err
	}

	return User(u), nil
}
