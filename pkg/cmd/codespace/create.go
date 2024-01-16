package codespace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/v2/internal/codespaces"
	"github.com/cli/cli/v2/internal/codespaces/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

const (
	DEVCONTAINER_PROMPT_DEFAULT = "Default Codespaces configuration"
)

const (
	permissionsPollingInterval = 5 * time.Second
	permissionsPollingTimeout  = 1 * time.Minute
)

const (
	displayNameMaxLength = 48 // 48 is the max length of the display name in the API
)

var (
	DEFAULT_DEVCONTAINER_DEFINITIONS = []string{".devcontainer.json", ".devcontainer/devcontainer.json"}
)

type NullableDuration struct {
	*time.Duration
}

func (d *NullableDuration) String() string {
	if d.Duration != nil {
		return d.Duration.String()
	}

	return ""
}

func (d *NullableDuration) Set(str string) error {
	duration, err := time.ParseDuration(str)
	if err != nil {
		return fmt.Errorf("error parsing duration: %w", err)
	}
	d.Duration = &duration
	return nil
}

func (d *NullableDuration) Type() string {
	return "duration"
}

func (d *NullableDuration) Minutes() *int {
	if d.Duration != nil {
		retentionMinutes := int(d.Duration.Minutes())
		return &retentionMinutes
	}

	return nil
}

type createOptions struct {
	repo              string
	branch            string
	location          string
	machine           string
	showStatus        bool
	permissionsOptOut bool
	devContainerPath  string
	idleTimeout       time.Duration
	retentionPeriod   NullableDuration
	displayName       string
	useWeb            bool
}

func newCreateCmd(app *App) *cobra.Command {
	opts := createOptions{}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a codespace",
		Args:  noArgsConstraint,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			return cmdutil.MutuallyExclusive(
				"using --web with --display-name, --idle-timeout, or --retention-period is not supported",
				opts.useWeb,
				opts.displayName != "" || opts.idleTimeout != 0 || opts.retentionPeriod.Duration != nil,
			)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Create(cmd.Context(), opts)
		},
	}

	createCmd.Flags().BoolVarP(&opts.useWeb, "web", "w", false, "create codespace from browser, cannot be used with --display-name, --idle-timeout, or --retention-period")

	createCmd.Flags().StringVarP(&opts.repo, "repo", "R", "", "repository name with owner: user/repo")
	if err := addDeprecatedRepoShorthand(createCmd, &opts.repo); err != nil {
		fmt.Fprintf(app.io.ErrOut, "%v\n", err)
	}

	createCmd.Flags().StringVarP(&opts.branch, "branch", "b", "", "repository branch")
	createCmd.Flags().StringVarP(&opts.location, "location", "l", "", "location: {EastUs|SouthEastAsia|WestEurope|WestUs2} (determined automatically if not provided)")
	createCmd.Flags().StringVarP(&opts.machine, "machine", "m", "", "hardware specifications for the VM")
	createCmd.Flags().BoolVarP(&opts.permissionsOptOut, "default-permissions", "", false, "do not prompt to accept additional permissions requested by the codespace")
	createCmd.Flags().BoolVarP(&opts.showStatus, "status", "s", false, "show status of post-create command and dotfiles")
	createCmd.Flags().DurationVar(&opts.idleTimeout, "idle-timeout", 0, "allowed inactivity before codespace is stopped, e.g. \"10m\", \"1h\"")
	createCmd.Flags().Var(&opts.retentionPeriod, "retention-period", "allowed time after shutting down before the codespace is automatically deleted (maximum 30 days), e.g. \"1h\", \"72h\"")
	createCmd.Flags().StringVar(&opts.devContainerPath, "devcontainer-path", "", "path to the devcontainer.json file to use when creating codespace")
	createCmd.Flags().StringVarP(&opts.displayName, "display-name", "d", "", fmt.Sprintf("display name for the codespace (%d characters or less)", displayNameMaxLength))

	return createCmd
}

// Create creates a new Codespace
func (a *App) Create(ctx context.Context, opts createOptions) error {
	// Overrides for Codespace developers to target test environments
	vscsLocation := os.Getenv("VSCS_LOCATION")
	vscsTarget := os.Getenv("VSCS_TARGET")
	vscsTargetUrl := os.Getenv("VSCS_TARGET_URL")

	userInputs := struct {
		Repository string
		Branch     string
		Location   string
	}{
		Repository: opts.repo,
		Branch:     opts.branch,
		Location:   opts.location,
	}

	if opts.useWeb && userInputs.Repository == "" {
		return a.browser.Browse(fmt.Sprintf("%s/codespaces/new", a.apiClient.ServerURL()))
	}

	prompter := &Prompter{}
	promptForRepoAndBranch := userInputs.Repository == "" && !opts.useWeb
	if promptForRepoAndBranch {
		var defaultRepo string
		if remotes, _ := a.remotes(); remotes != nil {
			if defaultRemote, _ := remotes.ResolvedRemote(); defaultRemote != nil {
				// this is a remote explicitly chosen via `repo set-default`
				defaultRepo = ghrepo.FullName(defaultRemote)
			} else if len(remotes) > 0 {
				// as a fallback, just pick the first remote
				defaultRepo = ghrepo.FullName(remotes[0])
			}
		}

		repoQuestions := []*survey.Question{
			{
				Name: "repository",
				Prompt: &survey.Input{
					Message: "Repository:",
					Help:    "Search for repos by name. To search within an org or user, or to see private repos, enter at least ':user/'.",
					Default: defaultRepo,
					Suggest: func(toComplete string) []string {
						return getRepoSuggestions(ctx, a.apiClient, toComplete)
					},
				},
				Validate: survey.Required,
			},
		}
		if err := prompter.Ask(repoQuestions, &userInputs); err != nil {
			return fmt.Errorf("failed to prompt: %w", err)
		}
	}

	if userInputs.Location == "" && vscsLocation != "" {
		userInputs.Location = vscsLocation
	}

	var repository *api.Repository
	err := a.RunWithProgress("Fetching repository", func() (err error) {
		repository, err = a.apiClient.GetRepository(ctx, userInputs.Repository)
		return
	})
	if err != nil {
		return fmt.Errorf("error getting repository: %w", err)
	}

	var billableOwner *api.User
	err = a.RunWithProgress("Validating repository for codespaces", func() (err error) {
		billableOwner, err = a.apiClient.GetCodespaceBillableOwner(ctx, userInputs.Repository)
		return
	})
	if err != nil {
		return fmt.Errorf("error checking codespace ownership: %w", err)
	} else if billableOwner != nil && (billableOwner.Type == "Organization" || billableOwner.Type == "User") {
		cs := a.io.ColorScheme()
		fmt.Fprintln(a.io.ErrOut, cs.Blue("  ✓ Codespaces usage for this repository is paid for by "+billableOwner.Login))
	}

	if promptForRepoAndBranch {
		branchPrompt := "Branch (leave blank for default branch):"
		if userInputs.Branch != "" {
			branchPrompt = "Branch:"
		}
		branchQuestions := []*survey.Question{
			{
				Name: "branch",
				Prompt: &survey.Input{
					Message: branchPrompt,
					Default: userInputs.Branch,
				},
			},
		}

		if err := prompter.Ask(branchQuestions, &userInputs); err != nil {
			return fmt.Errorf("failed to prompt: %w", err)
		}
	}

	branch := userInputs.Branch
	if branch == "" {
		branch = repository.DefaultBranch
	}

	devContainerPath := opts.devContainerPath

	// now that we have repo+branch, we can list available devcontainer.json files (if any)
	if opts.devContainerPath == "" {
		var devcontainers []api.DevContainerEntry
		err = a.RunWithProgress("Fetching devcontainer.json files", func() (err error) {
			devcontainers, err = a.apiClient.ListDevContainers(ctx, repository.ID, branch, 100)
			return
		})
		if err != nil {
			return fmt.Errorf("error getting devcontainer.json paths: %w", err)
		}

		if len(devcontainers) > 0 {

			// if there is only one devcontainer.json file and it is one of the default paths we can auto-select it
			if len(devcontainers) == 1 && stringInSlice(devcontainers[0].Path, DEFAULT_DEVCONTAINER_DEFINITIONS) {
				devContainerPath = devcontainers[0].Path
			} else {
				promptOptions := []string{}

				if !stringInSlice(devcontainers[0].Path, DEFAULT_DEVCONTAINER_DEFINITIONS) {
					promptOptions = []string{DEVCONTAINER_PROMPT_DEFAULT}
				}

				for _, devcontainer := range devcontainers {
					promptOptions = append(promptOptions, devcontainer.Path)
				}

				devContainerPathQuestion := &survey.Question{
					Name: "devContainerPath",
					Prompt: &survey.Select{
						Message: "Devcontainer definition file:",
						Options: promptOptions,
					},
				}

				if err := prompter.Ask([]*survey.Question{devContainerPathQuestion}, &devContainerPath); err != nil {
					return fmt.Errorf("failed to prompt: %w", err)
				}
			}
		}

		if devContainerPath == DEVCONTAINER_PROMPT_DEFAULT {
			// special arg allows users to opt out of devcontainer.json selection
			devContainerPath = ""
		}
	}

	machine := opts.machine
	// skip this if we have useWeb and no machine name provided,
	// because web UI will select default machine type if none is provided
	// web UI also provide a way to select machine type
	// therefore we let the user choose from the web UI instead of prompting from CLI
	if !(opts.useWeb && opts.machine == "") {
		machine, err = getMachineName(ctx, a.apiClient, prompter, repository.ID, opts.machine, branch, userInputs.Location, devContainerPath)
		if err != nil {
			return fmt.Errorf("error getting machine type: %w", err)
		}
		if machine == "" {
			return errors.New("there are no available machine types for this repository")
		}
	}

	if len(opts.displayName) > displayNameMaxLength {
		return fmt.Errorf("error creating codespace: display name should contain a maximum of %d characters", displayNameMaxLength)
	}

	createParams := &api.CreateCodespaceParams{
		RepositoryID:           repository.ID,
		Branch:                 branch,
		Machine:                machine,
		Location:               userInputs.Location,
		VSCSTarget:             vscsTarget,
		VSCSTargetURL:          vscsTargetUrl,
		IdleTimeoutMinutes:     int(opts.idleTimeout.Minutes()),
		RetentionPeriodMinutes: opts.retentionPeriod.Minutes(),
		DevContainerPath:       devContainerPath,
		PermissionsOptOut:      opts.permissionsOptOut,
		DisplayName:            opts.displayName,
	}

	if opts.useWeb {
		return a.browser.Browse(fmt.Sprintf("%s/codespaces/new?repo=%d&ref=%s&machine=%s&location=%s", a.apiClient.ServerURL(), createParams.RepositoryID, createParams.Branch, createParams.Machine, createParams.Location))
	}

	var codespace *api.Codespace
	err = a.RunWithProgress("Creating codespace", func() (err error) {
		codespace, err = a.apiClient.CreateCodespace(ctx, createParams)
		return
	})

	if err != nil {
		var aerr api.AcceptPermissionsRequiredError
		if !errors.As(err, &aerr) || aerr.AllowPermissionsURL == "" {
			return fmt.Errorf("error creating codespace: %w", err)
		}

		codespace, err = a.handleAdditionalPermissions(ctx, prompter, createParams, aerr.AllowPermissionsURL)
		if err != nil {
			// this error could be a cmdutil.SilentError (in the case that the user opened the browser) so we don't want to wrap it
			return err
		}
	}

	if opts.showStatus {
		if err := a.showStatus(ctx, codespace); err != nil {
			return fmt.Errorf("show status: %w", err)
		}
	}

	cs := a.io.ColorScheme()

	fmt.Fprintln(a.io.Out, codespace.Name)

	if a.io.IsStderrTTY() && codespace.IdleTimeoutNotice != "" {
		fmt.Fprintln(a.io.ErrOut, cs.Yellow("Notice:"), codespace.IdleTimeoutNotice)
	}

	return nil
}

func (a *App) handleAdditionalPermissions(ctx context.Context, prompter SurveyPrompter, createParams *api.CreateCodespaceParams, allowPermissionsURL string) (*api.Codespace, error) {
	var (
		isInteractive = a.io.CanPrompt()
		cs            = a.io.ColorScheme()
	)

	fmt.Fprintf(a.io.ErrOut, "You must authorize or deny additional permissions requested by this codespace before continuing.\n")

	if !isInteractive {
		fmt.Fprintf(a.io.ErrOut, "%s in your browser to review and authorize additional permissions: %s\n", cs.Bold("Open this URL"), allowPermissionsURL)
		fmt.Fprintf(a.io.ErrOut, "Alternatively, you can run %q with the %q option to continue without authorizing additional permissions.\n", a.io.ColorScheme().Bold("create"), cs.Bold("--default-permissions"))
		return nil, cmdutil.SilentError
	}

	choices := []string{
		"Continue in browser to review and authorize additional permissions (Recommended)",
		"Continue without authorizing additional permissions",
	}

	permsSurvey := []*survey.Question{
		{
			Name: "accept",
			Prompt: &survey.Select{
				Message: "What would you like to do?",
				Options: choices,
				Default: choices[0],
			},
			Validate: survey.Required,
		},
	}

	var answers struct {
		Accept string
	}

	if err := prompter.Ask(permsSurvey, &answers); err != nil {
		return nil, fmt.Errorf("error getting answers: %w", err)
	}

	// if the user chose to continue in the browser, open the URL
	if answers.Accept == choices[0] {
		if err := a.browser.Browse(allowPermissionsURL); err != nil {
			return nil, fmt.Errorf("error opening browser: %w", err)
		}

		// Poll until the user has accepted the permissions or timeout
		if err := a.pollForPermissions(ctx, createParams); err != nil {
			return nil, fmt.Errorf("error polling for permissions: %w", err)
		}
	} else {
		// If the user chose to create the codespace without the permissions,
		// we can continue with the create opting out of the additional permissions
		createParams.PermissionsOptOut = true
	}

	var codespace *api.Codespace
	err := a.RunWithProgress("Creating codespace", func() (err error) {
		codespace, err = a.apiClient.CreateCodespace(ctx, createParams)
		return
	})

	if err != nil {
		return nil, fmt.Errorf("error creating codespace: %w", err)
	}

	return codespace, nil
}

func (a *App) pollForPermissions(ctx context.Context, createParams *api.CreateCodespaceParams) error {
	return a.RunWithProgress("Waiting for permissions to be accepted in the browser", func() (err error) {
		ctx, cancel := context.WithTimeout(ctx, permissionsPollingTimeout)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			for {
				accepted, err := a.apiClient.GetCodespacesPermissionsCheck(ctx, createParams.RepositoryID, createParams.Branch, createParams.DevContainerPath)
				if err != nil {
					done <- err
					return
				}

				if accepted {
					done <- nil
					return
				}

				// Wait before polling again
				time.Sleep(permissionsPollingInterval)
			}
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for permissions to be accepted in the browser")
		}
	})
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

// getMachineName prompts the user to select the machine type, or validates the machine if non-empty.
func getMachineName(ctx context.Context, apiClient apiClient, prompter SurveyPrompter, repoID int, machine, branch, location string, devcontainerPath string) (string, error) {
	machines, err := apiClient.GetCodespacesMachines(ctx, repoID, branch, location, devcontainerPath)
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
	if err := prompter.Ask(machineSurvey, &machineAnswers); err != nil {
		return "", fmt.Errorf("error getting machine: %w", err)
	}

	selectedMachine := machineByName[machineAnswers.Machine]

	return selectedMachine.Name, nil
}

func getRepoSuggestions(ctx context.Context, apiClient apiClient, partialSearch string) []string {
	searchParams := api.RepoSearchParameters{
		// The prompt shows 7 items so 7 effectively turns off scrolling which is similar behavior to other clients
		MaxRepos: 7,
		Sort:     "repo",
	}

	repos, err := apiClient.GetCodespaceRepoSuggestions(ctx, partialSearch, searchParams)
	if err != nil {
		return nil
	}

	return repos
}

// buildDisplayName returns display name to be used in the machine survey prompt.
// prebuildAvailability will be migrated to use enum values: "none", "ready", "in_progress" before Prebuild GA
func buildDisplayName(displayName string, prebuildAvailability string) string {
	switch prebuildAvailability {
	case "ready":
		return displayName + " (Prebuild ready)"
	case "in_progress":
		return displayName + " (Prebuild in progress)"
	default:
		return displayName
	}
}

func stringInSlice(a string, slice []string) bool {
	for _, b := range slice {
		if b == a {
			return true
		}
	}
	return false
}
