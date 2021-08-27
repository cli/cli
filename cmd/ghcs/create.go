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
	"github.com/spf13/cobra"
)

var repo, branch, machine string

func newCreateCmd() *cobra.Command {
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a Codespace",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return Create()
		},
	}

	createCmd.Flags().StringVarP(&repo, "repo", "r", "", "repository name with owner: user/repo")
	createCmd.Flags().StringVarP(&branch, "branch", "b", "", "repository branch")
	createCmd.Flags().StringVarP(&machine, "machine", "m", "", "hardware specifications for the VM")

	return createCmd
}

func init() {
	rootCmd.AddCommand(newCreateCmd())
}

func Create() error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	locationCh := getLocation(ctx, apiClient)
	userCh := getUser(ctx, apiClient)
	log := output.NewLogger(os.Stdout, os.Stderr, false)

	repo, err := getRepoName()
	if err != nil {
		return fmt.Errorf("error getting repository name: %v", err)
	}
	branch, err := getBranchName()
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

	machine, err := getMachineName(ctx, userResult.User, repository, locationResult.Location, apiClient)
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

	log.Printf("Codespace created: ")

	fmt.Fprintln(os.Stdout, codespace.Name)

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

func getRepoName() (string, error) {
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

func getBranchName() (string, error) {
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

func getMachineName(ctx context.Context, user *api.User, repo *api.Repository, location string, apiClient *api.API) (string, error) {
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
	if err := ask(skuSurvey, &skuAnswers); err != nil {
		return "", fmt.Errorf("error getting SKU: %v", err)
	}

	sku := skuByName[skuAnswers.SKU]
	machine = sku.Name

	return machine, nil
}

// ask asks survery questions using standard options.
func ask(qs []*survey.Question, response interface{}) error {
	return survey.Ask(qs, response, survey.WithShowCursor(true))
}
