package listcmd_test

import (
	"errors"
	"testing"

	"github.com/cli/cli/v2/internal/prompter"
	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/stretchr/testify/require"
)

// To be honest I would rather test the ConsoleUserPrompter using a real console.
// I don't like the prompter mock at all.
//
// However, that's a task for another evening when I'm not about to go on vacation.

func TestUserPrompter(t *testing.T) {
	pm := &prompter.PrompterMock{}
	pm.InputFunc = func(prompt, dfault string) (string, error) {
		switch prompt {
		case "Which user do you want to target?":
			return "promptedusername", nil
		default:
			return dfault, nil
		}
	}

	r := listcmd.ConsoleUserPrompter{
		Prompter: pm,
	}

	user, err := r.PromptForUser()
	require.NoError(t, err)
	require.Equal(t, listcmd.User("promptedusername"), user)
}

func TestUserPrompterErrorIsReturnedUnwrapped(t *testing.T) {
	pm := &prompter.PrompterMock{}
	pm.InputFunc = func(prompt, dfault string) (string, error) {
		return "", errors.New("expected test error")
	}

	r := listcmd.ConsoleUserPrompter{
		Prompter: pm,
	}

	_, err := r.PromptForUser()
	require.Error(t, err)
}
