package ghcs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/pkg/cmd/codespace/output"
	"github.com/fatih/camelcase"
	"github.com/spf13/cobra"
)

type createOptions struct {
	repo       string
	branch     string
	machine    string
	showStatus bool
}

func newCreateCmd(app *App) *cobra.Command {
	opts := createOptions{}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a codespace",
		Args:  noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Create(cmd.Context(), opts)
		},
	}

	createCmd.Flags().StringVarP(&opts.repo, "repo", "r", "", "repository name with owner: user/repo")
	createCmd.Flags().StringVarP(&opts.branch, "branch", "b", "", "repository branch")
	createCmd.Flags().StringVarP(&opts.machine, "machine", "m", "", "hardware specifications for the VM")
	createCmd.Flags().BoolVarP(&opts.showStatus, "status", "s", false, "show status of post-create command and dotfiles")

	return createCmd
}

// Create creates a new Codespace
func (a *App) Create(ctx context.Context, opts createOptions) error {
	locationCh := getLocation(ctx, a.apiClient)
	userCh := getUser(ctx, a.apiClient)

	repo, err := getRepoName(opts.repo)
	if err != nil {
		return fmt.Errorf("error getting repository name: %w", err)
	}
	branch, err := getBranchName(opts.branch)
	if err != nil {
		return fmt.Errorf("error getting branch name: %w", err)
	}

	repository, err := a.apiClient.GetRepository(ctx, repo)
	if err != nil {
		return fmt.Errorf("error getting repository: %w", err)
	}

	locationResult := <-locationCh
	if locationResult.Err != nil {
		return fmt.Errorf("error getting codespace region location: %w", locationResult.Err)
	}

	userResult := <-userCh
	if userResult.Err != nil {
		return fmt.Errorf("error getting codespace user: %w", userResult.Err)
	}

	machine, err := getMachineName(ctx, opts.machine, userResult.User, repository, branch, locationResult.Location, a.apiClient)
	if err != nil {
		return fmt.Errorf("error getting machine type: %w", err)
	}
	if machine == "" {
		return errors.New("there are no available machine types for this repository")
	}

	a.logger.Print("Creating your codespace...")
	codespace, err := a.apiClient.CreateCodespace(ctx, &api.CreateCodespaceParams{
		User:         userResult.User.Login,
		RepositoryID: repository.ID,
		Branch:       branch,
		Machine:      machine,
		Location:     locationResult.Location,
	})
	a.logger.Print("\n")
	if err != nil {
		return fmt.Errorf("error creating codespace: %w", err)
	}

	if opts.showStatus {
		if err := showStatus(ctx, a.logger, a.apiClient, userResult.User, codespace); err != nil {
			return fmt.Errorf("show status: %w", err)
		}
	}

	a.logger.Printf("Codespace created: ")

	fmt.Fprintln(os.Stdout, codespace.Name)

	return nil
}

// showStatus polls the codespace for a list of post create states and their status. It will keep polling
// until all states have finished. Once all states have finished, we poll once more to check if any new
// states have been introduced and stop polling otherwise.
func showStatus(ctx context.Context, log *output.Logger, apiClient apiClient, user *api.User, codespace *api.Codespace) error {
	var lastState codespaces.PostCreateState
	var breakNextState bool

	finishedStates := make(map[string]bool)
	ctx, stopPolling := context.WithCancel(ctx)
	defer stopPolling()

	poller := func(states []codespaces.PostCreateState) {
		var inProgress bool
		for _, state := range states {
			if _, found := finishedStates[state.Name]; found {
				continue // skip this state as we've processed it already
			}

			if state.Name != lastState.Name {
				log.Print(state.Name)

				if state.Status == codespaces.PostCreateStateRunning {
					inProgress = true
					lastState = state
					log.Print("...")
					break
				}

				finishedStates[state.Name] = true
				log.Println("..." + state.Status)
			} else {
				if state.Status == codespaces.PostCreateStateRunning {
					inProgress = true
					log.Print(".")
					break
				}

				finishedStates[state.Name] = true
				log.Println(state.Status)
				lastState = codespaces.PostCreateState{} // reset the value
			}
		}

		if !inProgress {
			if breakNextState {
				stopPolling()
				return
			}
			breakNextState = true
		}
	}

	err := codespaces.PollPostCreateStates(ctx, log, apiClient, user, codespace, poller)
	if err != nil {
		if errors.Is(err, context.Canceled) && breakNextState {
			return nil // we cancelled the context to stop polling, we can ignore the error
		}

		return fmt.Errorf("failed to poll state changes from codespace: %w", err)
	}

	return nil
}

type getUserResult struct {
	User *api.User
	Err  error
}

// getUser fetches the user record associated with the GITHUB_TOKEN
func getUser(ctx context.Context, apiClient apiClient) <-chan getUserResult {
	ch := make(chan getUserResult, 1)
	go func() {
		user, err := apiClient.GetUser(ctx)
		ch <- getUserResult{user, err}
	}()
	return ch
}

type locationResult struct {
	Location string
	Err      error
}

// getLocation fetches the closest Codespace datacenter region/location to the user.
func getLocation(ctx context.Context, apiClient apiClient) <-chan locationResult {
	ch := make(chan locationResult, 1)
	go func() {
		location, err := apiClient.GetCodespaceRegionLocation(ctx)
		ch <- locationResult{location, err}
	}()
	return ch
}

// getRepoName prompts the user for the name of the repository, or returns the repository if non-empty.
func getRepoName(repo string) (string, error) {
	if repo != "" {
		return repo, nil
	}

	repoSurvey := []*survey.Question{
		{
			Name:     "repository",
			Prompt:   &survey.Input{Message: "Repository:"},
			Validate: survey.Required,
		},
	}
	err := ask(repoSurvey, &repo)
	return repo, err
}

// getBranchName prompts the user for the name of the branch, or returns the branch if non-empty.
func getBranchName(branch string) (string, error) {
	if branch != "" {
		return branch, nil
	}

	branchSurvey := []*survey.Question{
		{
			Name:     "branch",
			Prompt:   &survey.Input{Message: "Branch:"},
			Validate: survey.Required,
		},
	}
	err := ask(branchSurvey, &branch)
	return branch, err
}

// getMachineName prompts the user to select the machine type, or validates the machine if non-empty.
func getMachineName(ctx context.Context, machine string, user *api.User, repo *api.Repository, branch, location string, apiClient apiClient) (string, error) {
	skus, err := apiClient.GetCodespacesSKUs(ctx, user, repo, branch, location)
	if err != nil {
		return "", fmt.Errorf("error requesting machine instance types: %w", err)
	}

	// if user supplied a machine type, it must be valid
	// if no machine type was supplied, we don't error if there are no machine types for the current repo
	if machine != "" {
		for _, sku := range skus {
			if machine == sku.Name {
				return machine, nil
			}
		}

		availableSKUs := make([]string, len(skus))
		for i := 0; i < len(skus); i++ {
			availableSKUs[i] = skus[i].Name
		}

		return "", fmt.Errorf("there is no such machine for the repository: %s\nAvailable machines: %v", machine, availableSKUs)
	} else if len(skus) == 0 {
		return "", nil
	}

	if len(skus) == 1 {
		return skus[0].Name, nil // VS Code does not prompt for SKU if there is only one, this makes us consistent with that behavior
	}

	skuNames := make([]string, 0, len(skus))
	skuByName := make(map[string]*api.SKU)
	for _, sku := range skus {
		nameParts := camelcase.Split(sku.Name)
		machineName := strings.Title(strings.ToLower(nameParts[0]))
		skuName := fmt.Sprintf("%s - %s", machineName, sku.DisplayName)
		skuNames = append(skuNames, skuName)
		skuByName[skuName] = sku
	}

	skuSurvey := []*survey.Question{
		{
			Name: "sku",
			Prompt: &survey.Select{
				Message: "Choose Machine Type:",
				Options: skuNames,
				Default: skuNames[0],
			},
			Validate: survey.Required,
		},
	}

	var skuAnswers struct{ SKU string }
	if err := ask(skuSurvey, &skuAnswers); err != nil {
		return "", fmt.Errorf("error getting SKU: %w", err)
	}

	sku := skuByName[skuAnswers.SKU]
	machine = sku.Name

	return machine, nil
}
