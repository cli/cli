package tufrootverify

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/spf13/cobra"
)

type tufClientInstantiator func(o *tuf.Options) (*tuf.Client, error)

func NewTUFRootVerifyCmd(f *cmdutil.Factory, runF func() error) *cobra.Command {
	var mirror string
	var root string
	var cmd = cobra.Command{
		Use:    "tuf-root-verify --mirror <mirror-url> --root <root.json>",
		Args:   cobra.ExactArgs(0),
		Short:  "Verify the TUF repository from a provided TUF root",
		Hidden: true,
		Long: heredoc.Docf(`
			### NOTE: This feature is currently in beta, and subject to change.

			Verify a TUF repository with a local TUF root.

			The command requires you provide the %[1]s--mirror%[1]s flag, which should be the URL
			of the TUF repository mirror.

			The command also requires you provide the %[1]s--root%[1]s flag, which should be the
			path to the TUF root file.

			GitHub relies on TUF to securely deliver the trust root for our signing authority.
			For more information on TUF, see the official documentation: <https://theupdateframework.github.io/>.
		`, "`"),
		Example: heredoc.Doc(`
			# Verify the TUF repository from a provided TUF root
			gh attestation tuf-root-verify --mirror https://tuf-repo.github.com --root /path/to/1.root.json
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.IsHostSupported(); err != nil {
				return err
			}

			if runF != nil {
				return runF()
			}

			if err := tufRootVerify(tuf.New, mirror, root); err != nil {
				return fmt.Errorf("Failed to verify the TUF repository: %w", err)
			}

			io := f.IOStreams
			fmt.Sprintln(io.Out, io.ColorScheme().Green("Successfully verified the TUF repository"))
			return nil
		},
	}

	cmd.Flags().StringVarP(&mirror, "mirror", "m", "", "URL to the TUF repository mirror")
	cmd.MarkFlagRequired("mirror") //nolint:errcheck
	cmd.Flags().StringVarP(&root, "root", "r", "", "Path to the TUF root file on disk")
	cmd.MarkFlagRequired("root") //nolint:errcheck

	return &cmd
}

func tufRootVerify(makeTUF tufClientInstantiator, mirror, root string) error {
	rb, err := os.ReadFile(root)
	if err != nil {
		return fmt.Errorf("failed to read root file %s: %v", root, err)
	}
	opts := verification.GitHubTUFOptions()
	opts.Root = rb
	opts.RepositoryBaseURL = mirror
	// The purpose is the verify the TUF root and repository, make
	// sure there is no caching enabled
	opts.CacheValidity = 0
	if _, err = makeTUF(opts); err != nil {
		return fmt.Errorf("failed to create TUF client: %v", err)
	}

	return nil
}
