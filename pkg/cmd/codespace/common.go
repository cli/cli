package codespace

// This file defines functions common to the entire codespace command set.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type App struct {
	apiClient apiClient
	logger    *output.Logger
}

func NewApp(logger *output.Logger, apiClient apiClient) *App {
	return &App{
		apiClient: apiClient,
		logger:    logger,
	}
}

//go:generate moq -fmt goimports -rm -skip-ensure -out mock_api.go . apiClient
type apiClient interface {
	GetUser(ctx context.Context) (*api.User, error)
	GetCodespaceToken(ctx context.Context, user, name string) (string, error)
	GetCodespace(ctx context.Context, token, user, name string) (*api.Codespace, error)
	ListCodespaces(ctx context.Context) ([]*api.Codespace, error)
	DeleteCodespace(ctx context.Context, user, name string) error
	StartCodespace(ctx context.Context, token string, codespace *api.Codespace) error
	CreateCodespace(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error)
	GetRepository(ctx context.Context, nwo string) (*api.Repository, error)
	AuthorizedKeys(ctx context.Context, user string) ([]byte, error)
	GetCodespaceRegionLocation(ctx context.Context) (string, error)
	GetCodespacesSKUs(ctx context.Context, user *api.User, repository *api.Repository, branch, location string) ([]*api.SKU, error)
	GetCodespaceRepositoryContents(ctx context.Context, codespace *api.Codespace, path string) ([]byte, error)
}

var errNoCodespaces = errors.New("you have no codespaces")

func chooseCodespace(ctx context.Context, apiClient apiClient) (*api.Codespace, error) {
	codespaces, err := apiClient.ListCodespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}
	return chooseCodespaceFromList(ctx, codespaces)
}

func chooseCodespaceFromList(ctx context.Context, codespaces []*api.Codespace) (*api.Codespace, error) {
	if len(codespaces) == 0 {
		return nil, errNoCodespaces
	}

	sort.Slice(codespaces, func(i, j int) bool {
		return codespaces[i].CreatedAt > codespaces[j].CreatedAt
	})

	codespacesByName := make(map[string]*api.Codespace)
	codespacesNames := make([]string, 0, len(codespaces))
	for _, codespace := range codespaces {
		codespacesByName[codespace.Name] = codespace
		codespacesNames = append(codespacesNames, codespace.Name)
	}

	sshSurvey := []*survey.Question{
		{
			Name: "codespace",
			Prompt: &survey.Select{
				Message: "Choose codespace:",
				Options: codespacesNames,
				Default: codespacesNames[0],
			},
			Validate: survey.Required,
		},
	}

	var answers struct {
		Codespace string
	}
	if err := ask(sshSurvey, &answers); err != nil {
		return nil, fmt.Errorf("error getting answers: %w", err)
	}

	codespace := codespacesByName[answers.Codespace]
	return codespace, nil
}

// getOrChooseCodespace prompts the user to choose a codespace if the codespaceName is empty.
// It then fetches the codespace token and the codespace record.
func getOrChooseCodespace(ctx context.Context, apiClient apiClient, user *api.User, codespaceName string) (codespace *api.Codespace, token string, err error) {
	if codespaceName == "" {
		codespace, err = chooseCodespace(ctx, apiClient)
		if err != nil {
			if err == errNoCodespaces {
				return nil, "", err
			}
			return nil, "", fmt.Errorf("choosing codespace: %w", err)
		}
		codespaceName = codespace.Name

		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token: %w", err)
		}
	} else {
		token, err = apiClient.GetCodespaceToken(ctx, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting codespace token for given codespace: %w", err)
		}

		codespace, err = apiClient.GetCodespace(ctx, token, user.Login, codespaceName)
		if err != nil {
			return nil, "", fmt.Errorf("getting full codespace details: %w", err)
		}
	}

	return codespace, token, nil
}

func safeClose(closer io.Closer, err *error) {
	if closeErr := closer.Close(); *err == nil {
		*err = closeErr
	}
}

// hasTTY indicates whether the process connected to a terminal.
// It is not portable to assume stdin/stdout are fds 0 and 1.
var hasTTY = term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))

// ask asks survey questions on the terminal, using standard options.
// It fails unless hasTTY, but ideally callers should avoid calling it in that case.
func ask(qs []*survey.Question, response interface{}) error {
	if !hasTTY {
		return fmt.Errorf("no terminal")
	}
	err := survey.Ask(qs, response, survey.WithShowCursor(true))
	// The survey package temporarily clears the terminal's ISIG mode bit
	// (see tcsetattr(3)) so the QUIT button (Ctrl-C) is reported as
	// ASCII \x03 (ETX) instead of delivering SIGINT to the application.
	// So we have to serve ourselves the SIGINT.
	//
	// https://github.com/AlecAivazis/survey/#why-isnt-ctrl-c-working
	if err == terminal.InterruptErr {
		self, _ := os.FindProcess(os.Getpid())
		_ = self.Signal(os.Interrupt) // assumes POSIX

		// Suspend the goroutine, to avoid a race between
		// return from main and async delivery of INT signal.
		select {}
	}
	return err
}

// checkAuthorizedKeys reports an error if the user has not registered any SSH keys;
// see https://github.com/cli/cli/v2/issues/166#issuecomment-921769703.
// The check is not required for security but it improves the error message.
func checkAuthorizedKeys(ctx context.Context, client apiClient, user string) error {
	keys, err := client.AuthorizedKeys(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to read GitHub-authorized SSH keys for %s: %w", user, err)
	}
	if len(keys) == 0 {
		return fmt.Errorf("user %s has no GitHub-authorized SSH keys", user)
	}
	return nil // success
}

var ErrTooManyArgs = errors.New("the command accepts no arguments")

func noArgsConstraint(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return ErrTooManyArgs
	}
	return nil
}
