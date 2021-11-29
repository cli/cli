package codespace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/spf13/cobra"
)

type createOptions struct {
	repo        string
	branch      string
	machine     string
	showStatus  bool
	idleTimeout time.Duration
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
	createCmd.Flags().DurationVar(&opts.idleTimeout, "idle-timeout", 0, "allowed inactivity before codespace is stopped, e.g. \"10m\", \"1h\"")

	return createCmd
}

// Create creates a new Codespace
func (a *App) Create(ctx context.Context, opts createOptions) error {
	locationCh := getLocation(ctx, a.apiClient)

	userInputs := struct {
		Repository string
		Branch     string
	}{
		Repository: opts.repo,
		Branch:     opts.branch,
	}

	if userInputs.Repository == "" {
		branchPrompt := "Branch (leave blank for default branch):"
		if userInputs.Branch != "" {
			branchPrompt = "Branch:"
		}
		questions := []*survey.Question{
			{
				Name:     "repository",
				Prompt:   &survey.Input{Message: "Repository:"},
				Validate: survey.Required,
			},
			{
				Name: "branch",
				Prompt: &survey.Input{
					Message: branchPrompt,
					Default: userInputs.Branch,
				},
			},
		}
		if err := ask(questions, &userInputs); err != nil {
			return fmt.Errorf("failed to prompt: %w", err)
		}
	}

	a.StartProgressIndicatorWithLabel("Fetching repository")
	repository, err := a.apiClient.GetRepository(ctx, userInputs.Repository)
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error getting repository: %w", err)
	}

	branch := userInputs.Branch
	if branch == "" {
		branch = repository.DefaultBranch
	}

	locationResult := <-locationCh
	if locationResult.Err != nil {
		return fmt.Errorf("error getting codespace region location: %w", locationResult.Err)
	}

	machine, err := getMachineName(ctx, a.apiClient, repository.ID, opts.machine, branch, locationResult.Location)
	if err != nil {
		return fmt.Errorf("error getting machine type: %w", err)
	}
	if machine == "" {
		return errors.New("there are no available machine types for this repository")
	}

	a.StartProgressIndicatorWithLabel("Creating codespace")
	codespace, err := a.apiClient.CreateCodespace(ctx, &api.CreateCodespaceParams{
		RepositoryID:       repository.ID,
		Branch:             branch,
		Machine:            machine,
		Location:           locationResult.Location,
		IdleTimeoutMinutes: int(opts.idleTimeout.Minutes()),
	})
	a.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("error creating codespace: %w", err)
	}

	if opts.showStatus {
		if err := a.showStatus(ctx, codespace); err != nil {
			return fmt.Errorf("show status: %w", err)
		}
	}

	fmt.Fprintln(a.io.Out, codespace.Name)
	return nil
}

// showStatus polls the codespace for a list of post create states and their status. It will keep polling
// until all states have finished. Once all states have finished, we poll once more to check if any new
// states have been introduced and stop polling otherwise.
func (a *App) showStatus(ctx context.Context, codespace *api.Codespace) error {
	var (
		lastState      codespaces.PostCreateState
		breakNextState bool
	)

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
				a.StartProgressIndicatorWithLabel(state.Name)

				if state.Status == codespaces.PostCreateStateRunning {
					inProgress = true
					lastState = state
					break
				}

				finishedStates[state.Name] = true
				a.StopProgressIndicator()
			} else {
				if state.Status == codespaces.PostCreateStateRunning {
					inProgress = true
					break
				}

				finishedStates[state.Name] = true
				a.StopProgressIndicator()
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

	err := codespaces.PollPostCreateStates(ctx, a, a.apiClient, codespace, poller)
	if err != nil {
		if errors.Is(err, context.Canceled) && breakNextState {
			return nil // we cancelled the context to stop polling, we can ignore the error
		}

		return fmt.Errorf("failed to poll state changes from codespace: %w", err)
	}

	return nil
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

// getMachineName prompts the user to select the machine type, or validates the machine if non-empty.
func getMachineName(ctx context.Context, apiClient apiClient, repoID int, machine, branch, location string) (string, error) {
	machines, err := apiClient.GetCodespacesMachines(ctx, repoID, branch, location)
	if err != nil {
		return "", fmt.Errorf("error requesting machine instance types: %w", err)
	}

	// if user supplied a machine type, it must be valid
	// if no machine type was supplied, we don't error if there are no machine types for the current repo
	if machine != "" {
		for _, m := range machines {
			if machine == m.Name {
				return machine, nil
			}
		}

		availableMachines := make([]string, len(machines))
		for i := 0; i < len(machines); i++ {
			availableMachines[i] = machines[i].Name
		}

		return "", fmt.Errorf("there is no such machine for the repository: %s\nAvailable machines: %v", machine, availableMachines)
	} else if len(machines) == 0 {
		return "", nil
	}

	if len(machines) == 1 {
		// VS Code does not prompt for machine if there is only one, this makes us consistent with that behavior
		return machines[0].Name, nil
	}

	machineNames := make([]string, 0, len(machines))
	machineByName := make(map[string]*api.Machine)
	for _, m := range machines {
		machineName := buildDisplayName(m.DisplayName, m.PrebuildAvailability)
		machineNames = append(machineNames, machineName)
		machineByName[machineName] = m
	}

	machineSurvey := []*survey.Question{
		{
			Name: "machine",
			Prompt: &survey.Select{
				Message: "Choose Machine Type:",
				Options: machineNames,
				Default: machineNames[0],
			},
			Validate: survey.Required,
		},
	}

	var machineAnswers struct{ Machine string }
	if err := ask(machineSurvey, &machineAnswers); err != nil {
		return "", fmt.Errorf("error getting machine: %w", err)
	}

	selectedMachine := machineByName[machineAnswers.Machine]

	return selectedMachine.Name, nil
}

// buildDisplayName returns display name to be used in the machine survey prompt.
func buildDisplayName(displayName string, prebuildAvailability string) string {
	prebuildText := ""

	if prebuildAvailability == "blob" || prebuildAvailability == "pool" {
		prebuildText = " (Prebuild ready)"
	}

	return fmt.Sprintf("%s%s", displayName, prebuildText)
}
