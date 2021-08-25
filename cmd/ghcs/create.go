package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/camelcase"
	"github.com/github/ghcs/api"
	"github.com/github/ghcs/cmd/ghcs/output"
	"github.com/github/ghcs/internal/codespaces"
	"github.com/spf13/cobra"
)

var repo, branch, machine string

type CreateOptions struct {
	Repo       string
	Branch     string
	Machine    string
	ShowStatus bool
}

func newCreateCmd() *cobra.Command {
	opts := &CreateOptions{}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Create(opts)
		},
	}

	createCmd.Flags().StringVarP(&opts.Repo, "repo", "r", "", "repository name with owner: user/repo")
	createCmd.Flags().StringVarP(&opts.Branch, "branch", "b", "", "repository branch")
	createCmd.Flags().StringVarP(&opts.Machine, "machine", "m", "", "hardware specifications for the VM")
	createCmd.Flags().BoolVarP(&opts.ShowStatus, "status", "s", false, "show status of post-create command and dotfiles")

	return createCmd
}

func init() {
	rootCmd.AddCommand(newCreateCmd())
}

func Create(opts *CreateOptions) error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	locationCh := getLocation(ctx, apiClient)
	userCh := getUser(ctx, apiClient)
	log := output.NewLogger(os.Stdout, os.Stderr, false)

	repo, err := getRepoName(opts.Repo)
	if err != nil {
		return fmt.Errorf("error getting repository name: %v", err)
	}
	branch, err := getBranchName(opts.Branch)
	if err != nil {
		return fmt.Errorf("error getting branch name: %v", err)
	}

	repository, err := apiClient.GetRepository(ctx, repo)
	if err != nil {
		return fmt.Errorf("error getting repository: %v", err)
	}

	locationResult := <-locationCh
	if locationResult.Err != nil {
		return fmt.Errorf("error getting codespace region location: %v", locationResult.Err)
	}

	userResult := <-userCh
	if userResult.Err != nil {
		return fmt.Errorf("error getting codespace user: %v", userResult.Err)
	}

	machine, err := getMachineName(ctx, opts.Machine, userResult.User, repository, locationResult.Location, apiClient)
	if err != nil {
		return fmt.Errorf("error getting machine type: %v", err)
	}
	if machine == "" {
		return errors.New("There are no available machine types for this repository")
	}

	log.Println("Creating your codespace...")

	codespace, err := apiClient.CreateCodespace(ctx, userResult.User, repository, machine, branch, locationResult.Location)
	if err != nil {
		return fmt.Errorf("error creating codespace: %v", err)
	}

	if opts.ShowStatus {
		if err := showStatus(ctx, log, apiClient, userResult.User, codespace); err != nil {
			return fmt.Errorf("show status: %w", err)
		}
	}

	log.Printf("Codespace created: %s\n", codespace.Name)

	return nil
}

func showStatus(ctx context.Context, log *output.Logger, apiClient *api.API, user *api.User, codespace *api.Codespace) error {
	states, err := codespaces.PollPostCreateStates(ctx, log, apiClient, user, codespace)
	if err != nil {
		return fmt.Errorf("poll post create states: %v", err)
	}

	var lastState codespaces.PostCreateState
	var breakNextState bool

	for {
		stateUpdate := <-states
		if stateUpdate.Err != nil {
			return fmt.Errorf("receive state update: %v", err)
		}

		var inProgress bool
		for _, state := range stateUpdate.PostCreateStates {
			switch state.Status {
			case codespaces.PostCreateStateRunning:
				if lastState != state {
					lastState = state
					log.Print(state.Name)
				} else {
					log.Print(".")
				}

				inProgress = true
				break
			case codespaces.PostCreateStateFailed:
				if lastState.Name == state.Name && lastState.Status != state.Status {
					lastState = state
					log.Print(".Failed\n")
				}
			case codespaces.PostCreateStateSuccess:
				if lastState.Name == state.Name && lastState.Status != state.Status {
					lastState = state
					log.Print(".Success\n")
				}
			}
		}

		if !inProgress && !breakNextState {
			breakNextState = true
		} else if !inProgress && breakNextState {
			break
		}
	}

	return nil
}

type getUserResult struct {
	User *api.User
	Err  error
}

func getUser(ctx context.Context, apiClient *api.API) <-chan getUserResult {
	ch := make(chan getUserResult)
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

func getLocation(ctx context.Context, apiClient *api.API) <-chan locationResult {
	ch := make(chan locationResult)
	go func() {
		location, err := apiClient.GetCodespaceRegionLocation(ctx)
		ch <- locationResult{location, err}
	}()
	return ch
}

func getRepoName(repo string) (string, error) {
	if repo != "" {
		return repo, nil
	}

	repoSurvey := []*survey.Question{
		{
			Name:     "repository",
			Prompt:   &survey.Input{Message: "Repository"},
			Validate: survey.Required,
		},
	}
	err := survey.Ask(repoSurvey, &repo)
	return repo, err
}

func getBranchName(branch string) (string, error) {
	if branch != "" {
		return branch, nil
	}

	branchSurvey := []*survey.Question{
		{
			Name:     "branch",
			Prompt:   &survey.Input{Message: "Branch"},
			Validate: survey.Required,
		},
	}
	err := survey.Ask(branchSurvey, &branch)
	return branch, err
}

func getMachineName(ctx context.Context, machine string, user *api.User, repo *api.Repository, location string, apiClient *api.API) (string, error) {
	skus, err := apiClient.GetCodespacesSkus(ctx, user, repo, location)
	if err != nil {
		return "", fmt.Errorf("error getting codespace skus: %v", err)
	}

	// if user supplied a machine type, it must be valid
	// if no machine type was supplied, we don't error if there are no machine types for the current repo
	if machine != "" {
		for _, sku := range skus {
			if machine == sku.Name {
				return machine, nil
			}
		}

		availableSkus := make([]string, len(skus))
		for i := 0; i < len(skus); i++ {
			availableSkus[i] = skus[i].Name
		}

		return "", fmt.Errorf("there are is no such machine for the repository: %s\nAvailable machines: %v", machine, availableSkus)
	} else if len(skus) == 0 {
		return "", nil
	}

	skuNames := make([]string, 0, len(skus))
	skuByName := make(map[string]*api.Sku)
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

	skuAnswers := struct{ SKU string }{}
	if err := survey.Ask(skuSurvey, &skuAnswers); err != nil {
		return "", fmt.Errorf("error getting SKU: %v", err)
	}

	sku := skuByName[skuAnswers.SKU]
	machine = sku.Name

	return machine, nil
}
