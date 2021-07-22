package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/camelcase"
	"github.com/github/ghcs/api"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a GitHub Codespace.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return Create()
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}

var createSurvey = []*survey.Question{
	{
		Name:     "repository",
		Prompt:   &survey.Input{Message: "Repository"},
		Validate: survey.Required,
	},
	{
		Name:     "branch",
		Prompt:   &survey.Input{Message: "Branch"},
		Validate: survey.Required,
	},
}

func Create() error {
	ctx := context.Background()
	apiClient := api.New(os.Getenv("GITHUB_TOKEN"))
	locationCh := getLocation(ctx, apiClient)
	userCh := getUser(ctx, apiClient)

	answers := struct {
		Repository string
		Branch     string
	}{}

	if err := survey.Ask(createSurvey, &answers); err != nil {
		return fmt.Errorf("error getting answers: %v", err)
	}

	repository, err := apiClient.GetRepository(ctx, answers.Repository)
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

	skus, err := apiClient.GetCodespacesSkus(ctx, userResult.User, repository, locationResult.Location)
	if err != nil {
		return fmt.Errorf("error getting codespace skus: %v", err)
	}

	if len(skus) == 0 {
		fmt.Println("There are no available machine types for this repository")
		return nil
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
		return fmt.Errorf("error getting SKU: %v", err)
	}

	sku := skuByName[skuAnswers.SKU]
	fmt.Println("Creating your codespace...")

	codespace, err := apiClient.CreateCodespace(ctx, userResult.User, repository, sku, answers.Branch, locationResult.Location)
	if err != nil {
		return fmt.Errorf("error creating codespace: %v", err)
	}

	fmt.Println("Codespace created: " + codespace.Name)

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
