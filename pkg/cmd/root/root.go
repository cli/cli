package root

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc"
	actionsCmd "github.com/cli/cli/v2/pkg/cmd/actions"
	agentTaskCmd "github.com/cli/cli/v2/pkg/cmd/agent-task"
	aliasCmd "github.com/cli/cli/v2/pkg/cmd/alias"
	"github.com/cli/cli/v2/pkg/cmd/alias/shared"
	apiCmd "github.com/cli/cli/v2/pkg/cmd/api"
	authCmd "github.com/cli/cli/v2/pkg/cmd/auth"
	browseCmd "github.com/cli/cli/v2/pkg/cmd/browse"
	cacheCmd "github.com/cli/cli/v2/pkg/cmd/cache"
	completionCmd "github.com/cli/cli/v2/pkg/cmd/completion"
	configCmd "github.com/cli/cli/v2/pkg/cmd/config"
	copilotCmd "github.com/cli/cli/v2/pkg/cmd/copilot"
	discussionCmd "github.com/cli/cli/v2/pkg/cmd/discussion"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	gistCmd "github.com/cli/cli/v2/pkg/cmd/gist"
	gpgKeyCmd "github.com/cli/cli/v2/pkg/cmd/gpg-key"
	issueCmd "github.com/cli/cli/v2/pkg/cmd/issue"
	labelCmd "github.com/cli/cli/v2/pkg/cmd/label"
	licensesCmd "github.com/cli/cli/v2/pkg/cmd/licenses"
	orgCmd "github.com/cli/cli/v2/pkg/cmd/org"
	prCmd "github.com/cli/cli/v2/pkg/cmd/pr"
	projectCmd "github.com/cli/cli/v2/pkg/cmd/project"
	releaseCmd "github.com/cli/cli/v2/pkg/cmd/release"
	repoCmd "github.com/cli/cli/v2/pkg/cmd/repo"
	creditsCmd "github.com/cli/cli/v2/pkg/cmd/repo/credits"
	rulesetCmd "github.com/cli/cli/v2/pkg/cmd/ruleset"
	runCmd "github.com/cli/cli/v2/pkg/cmd/run"
	searchCmd "github.com/cli/cli/v2/pkg/cmd/search"
	secretCmd "github.com/cli/cli/v2/pkg/cmd/secret"
	sshKeyCmd "github.com/cli/cli/v2/pkg/cmd/ssh-key"
	statusCmd "github.com/cli/cli/v2/pkg/cmd/status"
	variableCmd "github.com/cli/cli/v2/pkg/cmd/variable"
	versionCmd "github.com/cli/cli/v2/pkg/cmd/version"
	workflowCmd "github.com/cli/cli/v2/pkg/cmd/workflow"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/spf13/cobra"
)

type AuthError struct {
	err error
}

type rootCommandBuilder struct {
	names []string
	build func() *cobra.Command
}

func (b rootCommandBuilder) matches(name string) bool {
	return slices.Contains(b.names, name)
}

func (ae *AuthError) Error() string {
	return ae.err.Error()
}

func NewCmdRoot(f *cmdutil.Factory, version, buildDate string, args []string) (*cobra.Command, error) {
	io := f.IOStreams

	cmd := &cobra.Command{
		Use:   "gh <command> <subcommand> [flags]",
		Short: "GitHub CLI",
		Long:  `Work seamlessly with GitHub from the command line.`,
		Example: heredoc.Doc(`
			$ gh issue create
			$ gh repo clone cli/cli
			$ gh pr checkout 321
		`),
		Annotations: map[string]string{
			"versionInfo": versionCmd.Format(version, buildDate),
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if cmdutil.IsAuthCheckEnabled(cmd) {
				cfg, err := f.Config()
				if err != nil {
					return fmt.Errorf("failed to read configuration: %w", err)
				}
				if !cmdutil.CheckAuth(cfg) {
					fmt.Fprint(io.ErrOut, authHelp())
					return &AuthError{}
				}
			}

			return nil
		},
	}

	// cmd.SetOut(f.IOStreams.Out)    // can't use due to https://github.com/spf13/cobra/issues/1708
	// cmd.SetErr(f.IOStreams.ErrOut) // just let it default to os.Stderr instead

	cmd.PersistentFlags().Bool("help", false, "Show help for command")

	if os.Getenv("GH_COBRA") == "" {
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true

		cmd.Flags().Bool("version", false, "Show gh version")

		cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
			rootHelpFunc(f, c, args)
		})
		cmd.SetUsageFunc(func(c *cobra.Command) error {
			return rootUsageFunc(f.IOStreams.ErrOut, c)
		})
		cmd.SetFlagErrorFunc(rootFlagErrorFunc)
	}

	cmd.AddGroup(&cobra.Group{
		ID:    "core",
		Title: "Core commands",
	})
	cmd.AddGroup(&cobra.Group{
		ID:    "actions",
		Title: "GitHub Actions commands",
	})

	builders := rootCommandBuilders(f, version, buildDate)
	dispatch := rootCommandDispatch(args, builders, os.Getenv("GH_COBRA") != "")
	var referenceCmd *cobra.Command
	aliasesNeeded := false
	if dispatch.command != "" {
		selectedCommand := addSelectedRootCommandBuilder(cmd, builders, dispatch.command)
		aliasesNeeded = needsAliases(cmd, args)
		if aliasesNeeded {
			cmd.RemoveCommand(selectedCommand)
			addRootCommandBuilders(cmd, builders, selectedCommand)
			referenceCmd = addHelpTopics(cmd, f.IOStreams)
		}
	} else if !dispatch.versionOnly {
		addRootCommandBuilders(cmd, builders, nil)
		referenceCmd = addHelpTopics(cmd, f.IOStreams)
		aliasesNeeded = needsAliases(cmd, args)
	}

	cmdutil.DisableAuthCheck(cmd)

	if referenceCmd != nil {
		referenceCmd.Annotations["markdown:prepare-long"] = "true"
		prepareReference := func(c *cobra.Command) {
			if c.Long == "" {
				c.Long = stringifyReference(cmd)
			}
		}
		referenceCmd.PreRunE = func(c *cobra.Command, _ []string) error {
			prepareReference(c)
			return nil
		}
		referenceCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
			prepareReference(c)
			longPager(f.IOStreams)(c, args)
		})
	}
	if aliasesNeeded {
		if err := addAliases(f, cmd); err != nil {
			return nil, err
		}
	}
	cmd.SetArgs(args)
	return cmd, nil
}

type rootCommandRoute struct {
	command     string
	versionOnly bool
}

func rootCommandDispatch(args []string, builders []rootCommandBuilder, cobraDefaults bool) rootCommandRoute {
	if !cobraDefaults && slices.Equal(args, []string{"--version"}) {
		return rootCommandRoute{versionOnly: true}
	}
	if cobraDefaults || len(args) == 0 || args[0] == "" || strings.HasPrefix(args[0], "-") {
		return rootCommandRoute{}
	}
	switch args[0] {
	case "help", "reference", "completion", cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
		return rootCommandRoute{}
	}
	for _, builder := range builders {
		if builder.matches(args[0]) {
			return rootCommandRoute{command: builder.names[0]}
		}
	}
	return rootCommandRoute{}
}

func rootCommandBuilders(f *cmdutil.Factory, version, buildDate string) []rootCommandBuilder {
	repoResolvingCmdFactory := *f
	repoResolvingCmdFactory.BaseRepo = factory.SmartBaseRepoFunc(f)

	return []rootCommandBuilder{
		{names: []string{"version"}, build: func() *cobra.Command { return versionCmd.NewCmdVersion(f, version, buildDate) }},
		{names: []string{"actions"}, build: func() *cobra.Command { return actionsCmd.NewCmdActions(f) }},
		{names: []string{"alias"}, build: func() *cobra.Command { return aliasCmd.NewCmdAlias(f) }},
		{names: []string{"auth"}, build: func() *cobra.Command { return authCmd.NewCmdAuth(f) }},
		{names: []string{"config"}, build: func() *cobra.Command { return configCmd.NewCmdConfig(f) }},
		{names: []string{"gist"}, build: func() *cobra.Command { return gistCmd.NewCmdGist(f) }},
		{names: []string{"gpg-key"}, build: func() *cobra.Command { return gpgKeyCmd.NewCmdGPGKey(f) }},
		{names: []string{"completion"}, build: func() *cobra.Command { return completionCmd.NewCmdCompletion(f.IOStreams) }},
		{names: []string{"search"}, build: func() *cobra.Command { return searchCmd.NewCmdSearch(f) }},
		{names: []string{"secret"}, build: func() *cobra.Command { return secretCmd.NewCmdSecret(f) }},
		{names: []string{"variable"}, build: func() *cobra.Command { return variableCmd.NewCmdVariable(f) }},
		{names: []string{"ssh-key"}, build: func() *cobra.Command { return sshKeyCmd.NewCmdSSHKey(f) }},
		{names: []string{"project"}, build: func() *cobra.Command { return projectCmd.NewCmdProject(f) }},
		{names: []string{"copilot"}, build: func() *cobra.Command { return copilotCmd.NewCmdCopilot(f, nil) }},
		{names: []string{"status"}, build: func() *cobra.Command { return statusCmd.NewCmdStatus(f, nil) }},
		{names: []string{"credits"}, build: func() *cobra.Command { return creditsCmd.NewCmdCredits(f, nil) }},
		{names: []string{"licenses"}, build: func() *cobra.Command { return licensesCmd.NewCmdLicenses(f) }},
		{names: []string{"agent-task", "agent-tasks", "agent", "agents"}, build: func() *cobra.Command { return agentTaskCmd.NewCmdAgentTask(&repoResolvingCmdFactory) }},
		{names: []string{"browse"}, build: func() *cobra.Command { return browseCmd.NewCmdBrowse(&repoResolvingCmdFactory, nil) }},
		{names: []string{"discussion"}, build: func() *cobra.Command { return discussionCmd.NewCmdDiscussion(&repoResolvingCmdFactory) }},
		{names: []string{"pr"}, build: func() *cobra.Command { return prCmd.NewCmdPR(&repoResolvingCmdFactory) }},
		{names: []string{"org"}, build: func() *cobra.Command { return orgCmd.NewCmdOrg(&repoResolvingCmdFactory) }},
		{names: []string{"issue"}, build: func() *cobra.Command { return issueCmd.NewCmdIssue(&repoResolvingCmdFactory) }},
		{names: []string{"release"}, build: func() *cobra.Command { return releaseCmd.NewCmdRelease(&repoResolvingCmdFactory) }},
		{names: []string{"repo"}, build: func() *cobra.Command { return repoCmd.NewCmdRepo(&repoResolvingCmdFactory) }},
		{names: []string{"ruleset", "rs"}, build: func() *cobra.Command { return rulesetCmd.NewCmdRuleset(&repoResolvingCmdFactory) }},
		{names: []string{"run"}, build: func() *cobra.Command { return runCmd.NewCmdRun(&repoResolvingCmdFactory) }},
		{names: []string{"workflow"}, build: func() *cobra.Command { return workflowCmd.NewCmdWorkflow(&repoResolvingCmdFactory) }},
		{names: []string{"label"}, build: func() *cobra.Command { return labelCmd.NewCmdLabel(&repoResolvingCmdFactory) }},
		{names: []string{"cache"}, build: func() *cobra.Command { return cacheCmd.NewCmdCache(&repoResolvingCmdFactory) }},
		{names: []string{"api"}, build: func() *cobra.Command { return apiCmd.NewCmdApi(&repoResolvingCmdFactory, nil) }},
	}
}

func addSelectedRootCommandBuilder(root *cobra.Command, builders []rootCommandBuilder, selected string) *cobra.Command {
	for _, builder := range builders {
		if builder.matches(selected) {
			cmd := builder.build()
			root.AddCommand(cmd)
			return cmd
		}
	}
	panic("root command route has no builder")
}

func addRootCommandBuilders(root *cobra.Command, builders []rootCommandBuilder, selected *cobra.Command) {
	for _, builder := range builders {
		if selected != nil && builder.matches(selected.Name()) {
			root.AddCommand(selected)
		} else {
			root.AddCommand(builder.build())
		}
	}
}

func addHelpTopics(root *cobra.Command, io *iostreams.IOStreams) *cobra.Command {
	var referenceCmd *cobra.Command
	for _, ht := range HelpTopics {
		helpTopicCmd := NewCmdHelpTopic(io, ht)
		root.AddCommand(helpTopicCmd)
		if ht.name == "reference" {
			referenceCmd = helpTopicCmd
		}
	}
	return referenceCmd
}

func needsAliases(root *cobra.Command, args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		args = args[1:]
	}
	cmd, _, err := root.Find(args)
	if err != nil {
		return true
	}
	if cmd == root {
		if err := root.ParseFlags(args); err == nil {
			if version, err := root.Flags().GetBool("version"); err == nil && version {
				return false
			}
		}
	}
	return cmd.HasSubCommands() || cmd == root || cmd.Name() == "reference"
}

func addAliases(f *cmdutil.Factory, cmd *cobra.Command) error {
	io := f.IOStreams
	cfg, err := f.Config()
	if err != nil {
		return fmt.Errorf("failed to read configuration: %w", err)
	}
	aliases := cfg.Aliases()
	validAliasName := shared.ValidAliasNameFunc(cmd)
	validAliasExpansion := shared.ValidAliasExpansionFunc(cmd)
	for k, v := range aliases.All() {
		aliasName := k
		aliasValue := v
		if validAliasName(aliasName) && validAliasExpansion(aliasValue) {
			split, _ := shlex.Split(aliasName)
			parentCmd, parentArgs, _ := cmd.Find(split)
			if !parentCmd.ContainsGroup("alias") {
				parentCmd.AddGroup(&cobra.Group{
					ID:    "alias",
					Title: "Alias commands",
				})
			}
			if strings.HasPrefix(aliasValue, "!") {
				shellAliasCmd := NewCmdShellAlias(io, parentArgs[0], aliasValue)
				parentCmd.AddCommand(shellAliasCmd)
			} else {
				aliasCmd := NewCmdAlias(io, parentArgs[0], aliasValue)
				split, _ := shlex.Split(aliasValue)
				child, _, _ := cmd.Find(split)
				aliasCmd.SetUsageFunc(func(_ *cobra.Command) error {
					return rootUsageFunc(f.IOStreams.ErrOut, child)
				})
				aliasCmd.SetHelpFunc(func(_ *cobra.Command, args []string) {
					rootHelpFunc(f, child, args)
				})
				parentCmd.AddCommand(aliasCmd)
			}
		}
	}

	return nil
}
