package trustedroot

import (
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/spf13/cobra"
)

type Options struct {
	Public      bool
	Private     bool
	TufUrl      string
	TufRootPath string
	Logger      *io.Handler
}

type tufClientInstantiator func(o *tuf.Options) (*tuf.Client, error)

func NewTrustedRootCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	trustedRootCmd := cobra.Command{
		Use:   "trusted-root [--public | --private | --tuf-url <url> --tuf-root <file-path>]",
		Args:  cobra.ExactArgs(0),
		Short: "Get a trusted_root.json file, likely for offline verification",
		Long: heredoc.Docf(`
			### NOTE: This feature is currently in beta, and subject to change.

            Get a trusted_root.json file, likely for offline verification.

            When using %[1]sgh attestation verify%[1]s, if your machine is on the internet,
            this will happen automatically. But to do offline verification, you need to
            supply a %[1]strusted_root.json%[1]s file with %[1]s--custom-trusted-root%[1]s;
            this command will help you fetch that %[1]strusted_root.json%[1]s file.

            How you call this command depends on if you are verifying attestations from a
            public or private repository.

            This command requires one of:
            * the %[1]s--public%[1]s flag (if verifying from a public repository)
            * the %[1]s--private%[1]s flag (if verifying from a private repository)
            * the %[1]s--tuf-url%[1]s and %[1]s--tuf-root%[1]s flags for advanced users

            If providing %[1]s--tuf-url%[1]s it should be the URL of your TUF repository
            mirror, and %[1]s--tuf-root%[1]s should be the path to the %[1]sroot.json%[1]s
            file that you securely obtained out-of-band.
		`, "`"),
		Example: heredoc.Doc(`
			# Get the trusted_root.json for a public repository
			gh attestation trusted-root --public

			# Get the trusted_root.json for a private repository
			gh attestation trusted-root --private
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the verify command
			opts.Logger = io.NewHandler(f.IOStreams)

			if !opts.Public && !opts.Private && opts.TufUrl == "" && opts.TufRootPath == "" {
				return fmt.Errorf("Please specify one of:\n  --public to get a trusted_root.json to use with public repositories\n  --private for private repositories\n\nFor more information, use --help.")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.IsHostSupported(); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			if err := getTrustedRoot(tuf.New, opts); err != nil {
				return fmt.Errorf("Failed to verify the TUF repository: %w", err)
			}

			return nil
		},
	}

	trustedRootCmd.Flags().BoolVarP(&opts.Public, "public", "", false, "Get trusted_root.json for public repository")
	trustedRootCmd.Flags().BoolVarP(&opts.Private, "private", "", false, "Get trusted_root.json for private repository")
	trustedRootCmd.MarkFlagsMutuallyExclusive("public", "private")
	trustedRootCmd.Flags().StringVarP(&opts.TufUrl, "tuf-url", "", "", "URL to the TUF repository mirror")
	trustedRootCmd.Flags().StringVarP(&opts.TufRootPath, "tuf-root", "", "", "Path to the TUF root.json file on disk")
	trustedRootCmd.MarkFlagsRequiredTogether("tuf-url", "tuf-root")
	trustedRootCmd.MarkFlagsMutuallyExclusive("public", "tuf-url")
	trustedRootCmd.MarkFlagsMutuallyExclusive("public", "tuf-root")
	trustedRootCmd.MarkFlagsMutuallyExclusive("private", "tuf-url")
	trustedRootCmd.MarkFlagsMutuallyExclusive("private", "tuf-root")

	return &trustedRootCmd
}

func getTrustedRoot(makeTUF tufClientInstantiator, opts *Options) error {
	tufOpts := verification.DefaultOptionsWithCacheSetting()

	if opts.Private {
		tufOpts = verification.GitHubTUFOptions()
	} else if opts.TufUrl != "" && opts.TufRootPath != "" {
		tufRoot, err := os.ReadFile(opts.TufRootPath)
		if err != nil {
			return fmt.Errorf("failed to read root file %s: %v", opts.TufRootPath, err)
		}

		tufOpts.Root = tufRoot
		tufOpts.RepositoryBaseURL = opts.TufUrl
	}

	// The purpose is the verify the TUF root and repository, make
	// sure there is no caching enabled
	tufOpts.CacheValidity = 0
	tufClient, err := makeTUF(tufOpts)
	if err != nil {
		return fmt.Errorf("failed to create TUF client: %v", err)
	}

	t, err := tufClient.GetTarget("trusted_root.json")
	if err != nil {
		return err
	}

	fmt.Print(string(t))

	return nil
}
