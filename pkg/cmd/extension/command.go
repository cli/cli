package extension

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

func NewCmdExtension(f *cmdutil.Factory) *cobra.Command {
	m := f.ExtensionManager
	io := f.IOStreams
	prompter := f.Prompter

	extCmd := cobra.Command{
		Use:   "extension",
		Short: "Manage gh extensions",
		Long: heredoc.Docf(`
			GitHub CLI extensions are repositories that provide additional gh commands.

			The name of the extension repository must start with "gh-" and it must contain an
			executable of the same name. All arguments passed to the %[1]sgh <extname>%[1]s invocation
			will be forwarded to the %[1]sgh-<extname>%[1]s executable of the extension.

			An extension cannot override any of the core gh commands. If an extension name conflicts
			with a core gh command you can use %[1]sgh extension exec <extname>%[1]s.

			See the list of available extensions at <https://github.com/topics/gh-extension>.
		`, "`"),
		Aliases: []string{"extensions", "ext"},
	}

	extCmd.AddCommand(
		&cobra.Command{
			Use:     "list",
			Short:   "List installed extension commands",
			Aliases: []string{"ls"},
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cmds := m.List()
				if len(cmds) == 0 {
					return cmdutil.NewNoResultsError("no installed extensions found")
				}
				cs := io.ColorScheme()
				t := utils.NewTablePrinter(io)
				for _, c := range cmds {
					var repo string
					if u, err := git.ParseURL(c.URL()); err == nil {
						if r, err := ghrepo.FromURL(u); err == nil {
							repo = ghrepo.FullName(r)
						}
					}

					t.AddField(fmt.Sprintf("gh %s", c.Name()), nil, nil)
					t.AddField(repo, nil, nil)
					version := displayExtensionVersion(c, c.CurrentVersion())
					if c.IsPinned() {
						t.AddField(version, nil, cs.Cyan)
					} else {
						t.AddField(version, nil, nil)
					}

					t.EndRow()
				}
				return t.Render()
			},
		},
		func() *cobra.Command {
			var pinFlag string
			cmd := &cobra.Command{
				Use:   "install <repository>",
				Short: "Install a gh extension from a repository",
				Long: heredoc.Doc(`
					Install a GitHub repository locally as a GitHub CLI extension.

					The repository argument can be specified in "owner/repo" format as well as a full URL.
					The URL format is useful when the repository is not hosted on github.com.

					To install an extension in development from the current directory, use "." as the
					value of the repository argument.

					See the list of available extensions at <https://github.com/topics/gh-extension>.
				`),
				Example: heredoc.Doc(`
					$ gh extension install owner/gh-extension
					$ gh extension install https://git.example.com/owner/gh-extension
					$ gh extension install .
				`),
				Args: cmdutil.MinimumArgs(1, "must specify a repository to install from"),
				RunE: func(cmd *cobra.Command, args []string) error {
					if args[0] == "." {
						if pinFlag != "" {
							return fmt.Errorf("local extensions cannot be pinned")
						}
						wd, err := os.Getwd()
						if err != nil {
							return err
						}
						return m.InstallLocal(wd)
					}

					repo, err := ghrepo.FromFullName(args[0])
					if err != nil {
						return err
					}

					if err := checkValidExtension(cmd.Root(), m, repo.RepoName()); err != nil {
						return err
					}

					cs := io.ColorScheme()
					if err := m.Install(repo, pinFlag); err != nil {
						if errors.Is(err, releaseNotFoundErr) {
							return fmt.Errorf("%s Could not find a release of %s for %s",
								cs.FailureIcon(), args[0], cs.Cyan(pinFlag))
						} else if errors.Is(err, commitNotFoundErr) {
							return fmt.Errorf("%s %s does not exist in %s",
								cs.FailureIcon(), cs.Cyan(pinFlag), args[0])
						}
						return err
					}

					if io.IsStdoutTTY() {
						fmt.Fprintf(io.Out, "%s Installed extension %s\n", cs.SuccessIcon(), args[0])
						if pinFlag != "" {
							fmt.Fprintf(io.Out, "%s Pinned extension at %s\n", cs.SuccessIcon(), cs.Cyan(pinFlag))
						}
					}
					return nil
				},
			}
			cmd.Flags().StringVar(&pinFlag, "pin", "", "pin extension to a release tag or commit ref")
			return cmd
		}(),
		func() *cobra.Command {
			var flagAll bool
			var flagForce bool
			var flagDryRun bool
			cmd := &cobra.Command{
				Use:   "upgrade {<name> | --all}",
				Short: "Upgrade installed extensions",
				Args: func(cmd *cobra.Command, args []string) error {
					if len(args) == 0 && !flagAll {
						return cmdutil.FlagErrorf("specify an extension to upgrade or `--all`")
					}
					if len(args) > 0 && flagAll {
						return cmdutil.FlagErrorf("cannot use `--all` with extension name")
					}
					if len(args) > 1 {
						return cmdutil.FlagErrorf("too many arguments")
					}
					return nil
				},
				RunE: func(cmd *cobra.Command, args []string) error {
					var name string
					if len(args) > 0 {
						name = normalizeExtensionSelector(args[0])
					}
					if flagDryRun {
						m.EnableDryRunMode()
					}
					cs := io.ColorScheme()
					err := m.Upgrade(name, flagForce)
					if err != nil && !errors.Is(err, upToDateError) {
						if name != "" {
							fmt.Fprintf(io.ErrOut, "%s Failed upgrading extension %s: %s\n", cs.FailureIcon(), name, err)
						} else if errors.Is(err, noExtensionsInstalledError) {
							return cmdutil.NewNoResultsError("no installed extensions found")
						} else {
							fmt.Fprintf(io.ErrOut, "%s Failed upgrading extensions\n", cs.FailureIcon())
						}
						return cmdutil.SilentError
					}
					if io.IsStdoutTTY() {
						successStr := "Successfully"
						if flagDryRun {
							successStr = "Would have"
						}
						if errors.Is(err, upToDateError) {
							fmt.Fprintf(io.Out, "%s Extension already up to date\n", cs.SuccessIcon())
						} else if name != "" {
							fmt.Fprintf(io.Out, "%s %s upgraded extension %s\n", cs.SuccessIcon(), successStr, name)
						} else {
							fmt.Fprintf(io.Out, "%s %s upgraded extensions\n", cs.SuccessIcon(), successStr)
						}
					}
					return nil
				},
			}
			cmd.Flags().BoolVar(&flagAll, "all", false, "Upgrade all extensions")
			cmd.Flags().BoolVar(&flagForce, "force", false, "Force upgrade extension")
			cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Only display upgrades")
			return cmd
		}(),
		&cobra.Command{
			Use:   "remove <name>",
			Short: "Remove an installed extension",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				extName := normalizeExtensionSelector(args[0])
				if err := m.Remove(extName); err != nil {
					return err
				}
				if io.IsStdoutTTY() {
					cs := io.ColorScheme()
					fmt.Fprintf(io.Out, "%s Removed extension %s\n", cs.SuccessIcon(), extName)
				}
				return nil
			},
		},
		&cobra.Command{
			Use:   "manage",
			Short: "Enter a UI for browsing, adding, and removing extensions",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				if !io.CanPrompt() {
					return errors.New("this command runs an interactive UI and needs to be run in a terminal")
				}
				// TODO
				return nil
			},
		},
		&cobra.Command{
			Use:   "exec <name> [args]",
			Short: "Execute an installed extension",
			Long: heredoc.Doc(`
				Execute an extension using the short name. For example, if the extension repository is
				"owner/gh-extension", you should pass "extension". You can use this command when
				the short name conflicts with a core gh command.

				All arguments after the extension name will be forwarded to the executable
				of the extension.
			`),
			Example: heredoc.Doc(`
				# execute a label extension instead of the core gh label command
				$ gh extension exec label
			`),
			Args:               cobra.MinimumNArgs(1),
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				if found, err := m.Dispatch(args, io.In, io.Out, io.ErrOut); !found {
					return fmt.Errorf("extension %q not found", args[0])
				} else {
					return err
				}
			},
		},
		func() *cobra.Command {
			promptCreate := func() (string, extensions.ExtTemplateType, error) {
				extName, err := prompter.Input("Extension name:", "")
				if err != nil {
					return extName, -1, err
				}
				options := []string{"Script (Bash, Ruby, Python, etc)", "Go", "Other Precompiled (C++, Rust, etc)"}
				extTmplType, err := prompter.Select("What kind of extension?",
					options[0],
					options)
				return extName, extensions.ExtTemplateType(extTmplType), err
			}
			var flagType string
			cmd := &cobra.Command{
				Use:   "create [<name>]",
				Short: "Create a new extension",
				Example: heredoc.Doc(`
					# Use interactively
					gh extension create

					# Create a script-based extension
					gh extension create foobar

					# Create a Go extension
					gh extension create --precompiled=go foobar

					# Create a non-Go precompiled extension
					gh extension create --precompiled=other foobar
				`),
				Args: cobra.MaximumNArgs(1),
				RunE: func(cmd *cobra.Command, args []string) error {
					if cmd.Flags().Changed("precompiled") {
						if flagType != "go" && flagType != "other" {
							return cmdutil.FlagErrorf("value for --precompiled must be 'go' or 'other'. Got '%s'", flagType)
						}
					}
					var extName string
					var err error
					tmplType := extensions.GitTemplateType
					if len(args) == 0 {
						if io.IsStdoutTTY() {
							extName, tmplType, err = promptCreate()
							if err != nil {
								return fmt.Errorf("could not prompt: %w", err)
							}
						}
					} else {
						extName = args[0]
						if flagType == "go" {
							tmplType = extensions.GoBinTemplateType
						} else if flagType == "other" {
							tmplType = extensions.OtherBinTemplateType
						}
					}

					var fullName string

					if strings.HasPrefix(extName, "gh-") {
						fullName = extName
						extName = extName[3:]
					} else {
						fullName = "gh-" + extName
					}
					if err := m.Create(fullName, tmplType); err != nil {
						return err
					}
					if !io.IsStdoutTTY() {
						return nil
					}

					var goBinChecks string

					steps := fmt.Sprintf(
						"- run 'cd %[1]s; gh extension install .; gh %[2]s' to see your new extension in action",
						fullName, extName)

					cs := io.ColorScheme()
					if tmplType == extensions.GoBinTemplateType {
						goBinChecks = heredoc.Docf(`
						%[1]s Downloaded Go dependencies
						%[1]s Built %[2]s binary
						`, cs.SuccessIcon(), fullName)
						steps = heredoc.Docf(`
						- run 'cd %[1]s; gh extension install .; gh %[2]s' to see your new extension in action
						- use 'go build && gh %[2]s' to see changes in your code as you develop`, fullName, extName)
					} else if tmplType == extensions.OtherBinTemplateType {
						steps = heredoc.Docf(`
						- run 'cd %[1]s; gh extension install .' to install your extension locally
						- fill in script/build.sh with your compilation script for automated builds
						- compile a %[1]s binary locally and run 'gh %[2]s' to see changes`, fullName, extName)
					}
					link := "https://docs.github.com/github-cli/github-cli/creating-github-cli-extensions"
					out := heredoc.Docf(`
						%[1]s Created directory %[2]s
						%[1]s Initialized git repository
						%[1]s Set up extension scaffolding
						%[6]s
						%[2]s is ready for development!

						%[4]s
						%[5]s
						- commit and use 'gh repo create' to share your extension with others

						For more information on writing extensions:
						%[3]s
					`, cs.SuccessIcon(), fullName, link, cs.Bold("Next Steps"), steps, goBinChecks)
					fmt.Fprint(io.Out, out)
					return nil
				},
			}
			cmd.Flags().StringVar(&flagType, "precompiled", "", "Create a precompiled extension. Possible values: go, other")
			return cmd
		}(),
	)

	return &extCmd
}

func checkValidExtension(rootCmd *cobra.Command, m extensions.ExtensionManager, extName string) error {
	if !strings.HasPrefix(extName, "gh-") {
		return errors.New("extension repository name must start with `gh-`")
	}

	commandName := strings.TrimPrefix(extName, "gh-")
	if c, _, err := rootCmd.Traverse([]string{commandName}); err != nil {
		return err
	} else if c != rootCmd {
		return fmt.Errorf("%q matches the name of a built-in command", commandName)
	}

	for _, ext := range m.List() {
		if ext.Name() == commandName {
			return fmt.Errorf("there is already an installed extension that provides the %q command", commandName)
		}
	}

	return nil
}

func normalizeExtensionSelector(n string) string {
	if idx := strings.IndexRune(n, '/'); idx >= 0 {
		n = n[idx+1:]
	}
	return strings.TrimPrefix(n, "gh-")
}

func displayExtensionVersion(ext extensions.Extension, version string) string {
	if !ext.IsBinary() && len(version) > 8 {
		return version[:8]
	}
	return version
}
