package codespace

// This file defines functions common to the entire codespace command set.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

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
	GetCodespace(ctx context.Context, name string, includeConnection bool) (*api.Codespace, error)
	ListCodespaces(ctx context.Context, limit int) ([]*api.Codespace, error)
	DeleteCodespace(ctx context.Context, name string) error
	StartCodespace(ctx context.Context, name string) error
	CreateCodespace(ctx context.Context, params *api.CreateCodespaceParams) (*api.Codespace, error)
	GetRepository(ctx context.Context, nwo string) (*api.Repository, error)
	AuthorizedKeys(ctx context.Context, user string) ([]byte, error)
	GetCodespaceRegionLocation(ctx context.Context) (string, error)
	GetCodespacesMachines(ctx context.Context, repoID int, branch, location string) ([]*api.Machine, error)
	GetCodespaceRepositoryContents(ctx context.Context, codespace *api.Codespace, path string) ([]byte, error)
}

var errNoCodespaces = errors.New("you have no codespaces")

func chooseCodespace(ctx context.Context, apiClient apiClient) (*api.Codespace, error) {
	codespaces, err := apiClient.ListCodespaces(ctx, -1)
	if err != nil {
		return nil, fmt.Errorf("error getting codespaces: %w", err)
	}
	return chooseCodespaceFromList(ctx, codespaces)
}

// chooseCodespaceFromList returns the selected codespace from the list,
// or an error if there are no codespaces.
func chooseCodespaceFromList(ctx context.Context, codespaces []*api.Codespace) (*api.Codespace, error) {
	if len(codespaces) == 0 {
		return nil, errNoCodespaces
	}

	sort.Slice(codespaces, func(i, j int) bool {
		return codespaces[i].CreatedAt > codespaces[j].CreatedAt
	})

	type codespaceWithIndex struct {
		cs  codespace
		idx int
	}

	namesWithConflict := make(map[string]bool)
	codespacesByName := make(map[string]codespaceWithIndex)
	codespacesNames := make([]string, 0, len(codespaces))
	for _, apiCodespace := range codespaces {
		cs := codespace{apiCodespace}
		csName := cs.displayName(false, false)
		displayNameWithGitStatus := cs.displayName(false, true)

		_, hasExistingConflict := namesWithConflict[csName]
		if seenCodespace, ok := codespacesByName[csName]; ok || hasExistingConflict {
			// There is an existing codespace on the repo and branch.
			// We need to disambiguate by adding the codespace name
			// to the existing entry and the one we are processing now.
			if !hasExistingConflict {
				fullDisplayName := seenCodespace.cs.displayName(true, false)
				fullDisplayNameWithGitStatus := seenCodespace.cs.displayName(true, true)

				codespacesByName[fullDisplayName] = codespaceWithIndex{seenCodespace.cs, seenCodespace.idx}
				codespacesNames[seenCodespace.idx] = fullDisplayNameWithGitStatus
				delete(codespacesByName, csName) // delete the existing map entry with old name

				// All other codespaces with the same name should update
				// to their specific name, this tracks conflicting names going forward
				namesWithConflict[csName] = true
			}

			// update this codespace names to include the name to disambiguate
			csName = cs.displayName(true, false)
			displayNameWithGitStatus = cs.displayName(true, true)
		}

		codespacesByName[csName] = codespaceWithIndex{cs, len(codespacesNames)}
		codespacesNames = append(codespacesNames, displayNameWithGitStatus)
	}

	csSurvey := []*survey.Question{
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
	if err := ask(csSurvey, &answers); err != nil {
		return nil, fmt.Errorf("error getting answers: %w", err)
	}

	// Codespaces are indexed without the git status included as compared
	// to how it is displayed in the prompt, so the git status symbol needs
	// cleaning up in case it is included.
	selectedCodespace := strings.Replace(answers.Codespace, gitStatusDirty, "", -1)
	return codespacesByName[selectedCodespace].cs.Codespace, nil
}

// getOrChooseCodespace prompts the user to choose a codespace if the codespaceName is empty.
// It then fetches the codespace record with full connection details.
func getOrChooseCodespace(ctx context.Context, apiClient apiClient, codespaceName string) (codespace *api.Codespace, err error) {
	if codespaceName == "" {
		codespace, err = chooseCodespace(ctx, apiClient)
		if err != nil {
			if err == errNoCodespaces {
				return nil, err
			}
			return nil, fmt.Errorf("choosing codespace: %w", err)
		}
	} else {
		codespace, err = apiClient.GetCodespace(ctx, codespaceName, true)
		if err != nil {
			return nil, fmt.Errorf("getting full codespace details: %w", err)
		}
	}

	return codespace, nil
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

type codespace struct {
	*api.Codespace
}

// displayName returns the repository nwo and branch.
// If includeName is true, the name of the codespace is included.
// If includeGitStatus is true, the branch will include a star if
// the codespace has unsaved changes.
func (c codespace) displayName(includeName, includeGitStatus bool) string {
	branch := c.Branch
	if includeGitStatus {
		branch = c.branchWithGitStatus()
	}

	if includeName {
		return fmt.Sprintf(
			"%s: %s [%s]", c.RepositoryNWO, branch, c.Name,
		)
	}
	return c.RepositoryNWO + ": " + branch
}

// gitStatusDirty represents an unsaved changes status.
const gitStatusDirty = "*"

// branchWithGitStatus returns the branch with a star
// if the branch is currently being worked on.
func (c codespace) branchWithGitStatus() string {
	if c.hasUnsavedChanges() {
		return c.Branch + gitStatusDirty
	}

	return c.Branch
}

// hasUnsavedChanges returns whether the environment has
// unsaved changes.
func (c codespace) hasUnsavedChanges() bool {
	return c.Environment.GitStatus.HasUncommitedChanges || c.Environment.GitStatus.HasUnpushedChanges
}
