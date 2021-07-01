package extensions

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func NewCmdExtensions(io *iostreams.IOStreams) *cobra.Command {
	m := NewManager()

	extCmd := cobra.Command{
		Use:   "extensions",
		Short: "Manage gh extensions",
	}

	extCmd.AddCommand(
		&cobra.Command{
			Use:   "list",
			Short: "List installed extension commands",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cmds := m.List()
				if len(cmds) == 0 {
					return errors.New("no extensions installed")
				}
				for _, c := range cmds {
					name := filepath.Base(c)
					parts := strings.SplitN(name, "-", 2)
					fmt.Fprintf(io.Out, "%s %s\n", parts[0], parts[1])
				}
				return nil
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
				if !strings.HasPrefix(repo.RepoName(), "gh-") {
					return errors.New("the repository name must start with `gh-`")
				}
				protocol := "https" // TODO: respect user's preferred protocol
				return m.Install(ghrepo.FormatRemoteURL(repo, protocol), io.Out, io.ErrOut)
			},
		},
		&cobra.Command{
			Use:   "upgrade",
			Short: "Upgrade installed extensions",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return m.Upgrade(io.Out, io.ErrOut)
			},
		},
	)

	extCmd.Hidden = true
	return &extCmd
}
