package verifytufroot

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logger"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	
	"github.com/MakeNowJust/heredoc"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/spf13/cobra"
)

func NewVerifyTUFRootCmd(f *cmdutil.Factory) *cobra.Command {
	var mirror string
	var root string
	var cmd = cobra.Command{
		Use:   "verify-tuf-root --mirror <mirror-url> --root <root.json>",
		Args:  cobra.ExactArgs(0),
		Short: "Verify the TUF repository from a provided TUF root",
		Long: heredoc.Docf(`
			Verify a TUF repository from a local TUF root.

			The command requires you provide the %[1]s--mirror%[1]s flag, which should be the URL 
			of the TUF repository mirror.
			
			The command also requires you provide the %[1]s--root%[1]s flag, which should be the 
			path to the TUF root file.
		`, "`"),
		Example: heredoc.Doc(`
			# Verify the TUF repository from a provided TUF root
			gh attestation tuf-root-verify --mirror https://tuf-repo.github.com --root /path/to/1.root.json
		`),
		Run: func(cmd *cobra.Command, args []string) {
			logger := logger.NewDefaultLogger(f.IOStreams)
			if err := verifyTUFRoot(mirror, root); err != nil {
				fmt.Sprintln(logger.IO.Out, logger.ColorScheme.Redf("Failed to verify the TUF repository: %s", err))
				os.Exit(1)
			}
			fmt.Sprintln(logger.IO.Out, logger.ColorScheme.Green("Successfully verified the TUF repository"))
		},
	}

	cmd.Flags().StringVarP(&mirror, "mirror", "m", "", "URL to the TUF repository mirror")
	cmd.MarkFlagRequired("mirror") //nolint:errcheck
	cmd.Flags().StringVarP(&root, "root", "r", "", "Path to the TUF root file on disk")
	cmd.MarkFlagRequired("root") //nolint:errcheck

	return &cmd
}

func verifyTUFRoot(mirror, root string) error {
	rb, err := os.ReadFile(root)
	if err != nil {
		return fmt.Errorf("failed to read root file %s: %w", root, err)
	}
	opts, err := verification.GitHubTUFOptions()
	if err != nil {
		return err
	}
	opts.Root = rb
	opts.RepositoryBaseURL = mirror
	// The purpose is the verify the TUF root and repository, make
	// sure there is no caching enabled
	opts.CacheValidity = 0
	if _, err = tuf.New(opts); err != nil {
		return fmt.Errorf("failed to create TUF client: %w", err)
	}

	return nil
}
