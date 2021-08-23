package extension

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/extensions"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

func NewCmdExtension(f *cmdutil.Factory) *cobra.Command {
	m := f.ExtensionManager
	io := f.IOStreams

	extCmd := cobra.Command{
		Use:   "extension",
		Short: "Manage gh extensions",
		Long: heredoc.Docf(`
			GitHub CLI extensions are repositories that provide additional gh commands.

			The name of the extension repository must start with "gh-" and it must contain an
			executable of the same name. All arguments passed to the %[1]sgh <extname>%[1]s invocation
			will be forwarded to the %[1]sgh-<extname>%[1]s executable of the extension.

			An extension cannot override any of the core gh commands.
		`, "`"),
		Aliases: []string{"extensions"},
	}

	extCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed extension commands",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cmds := m.List(true)
				if len(cmds) == 0 {
					return errors.New("no extensions installed")
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
					var updateAvailable string
					if c.UpdateAvailable() {
						updateAvailable = "Upgrade available"
					}
					t.AddField(updateAvailable, nil, cs.Green)
					t.EndRow()
				}
				return t.Render()
			},
		},
		&cobra.Command{
			Use:   "install <repo>",
			Short: "Install a gh extension from a repository",
			Args:  cmdutil.MinimumArgs(1, "must specify a repository to install from"),
			RunE: func(cmd *cobra.Command, args []string) error {
				if args[0] == "." {
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

				cfg, err := f.Config()
				if err != nil {
					return err
				}
				protocol, _ := cfg.Get(repo.RepoHost(), "git_protocol")
				return m.Install(ghrepo.FormatRemoteURL(repo, protocol), io.Out, io.ErrOut)
			},
		},
		func() *cobra.Command {
			var flagAll bool
			var flagForce bool
			cmd := &cobra.Command{
				Use:   "upgrade {<name> | --all}",
				Short: "Upgrade installed extensions",
				Args: func(cmd *cobra.Command, args []string) error {
					if len(args) == 0 && !flagAll {
						return &cmdutil.FlagError{Err: errors.New("must specify an extension to upgrade")}
					}
					if len(args) > 0 && flagAll {
						return &cmdutil.FlagError{Err: errors.New("cannot use `--all` with extension name")}
					}
					if len(args) > 1 {
						return &cmdutil.FlagError{Err: errors.New("too many arguments")}
					}
					return nil
				},
				RunE: func(cmd *cobra.Command, args []string) error {
					var name string
					if len(args) > 0 {
						name = normalizeExtensionSelector(args[0])
					}
					return m.Upgrade(name, flagForce, io.Out, io.ErrOut)
				},
			}
			cmd.Flags().BoolVar(&flagAll, "all", false, "Upgrade all extensions")
			cmd.Flags().BoolVar(&flagForce, "force", false, "Force upgrade extension")
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
			Use:   "create <name>",
			Short: "Create a new extension",
			Args:  cmdutil.ExactArgs(1, "must specify a name for the extension"),
			RunE: func(cmd *cobra.Command, args []string) error {
				extName := args[0]
				if !strings.HasPrefix(extName, "gh-") {
					extName = "gh-" + extName
				}
				if err := m.Create(extName); err != nil {
					return err
				}
				if !io.IsStdoutTTY() {
					return nil
				}
				link := "https://docs.github.com/github-cli/github-cli/creating-github-cli-extensions"
				cs := io.ColorScheme()
				out := heredoc.Docf(`
					%[1]s Created directory %[2]s
					%[1]s Initialized git repository
					%[1]s Set up extension scaffolding

					%[2]s is ready for development

					Install locally with: cd %[2]s && gh extension install .

					Publish to GitHub with: gh repo create %[2]s

					For more information on writing extensions:
					%[3]s
				`, cs.SuccessIcon(), extName, link)
				fmt.Fprint(io.Out, out)
				return nil
			},
		},
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

	for _, ext := range m.List(false) {
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
