package trustedroot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/spf13/cobra"
)

type Options struct {
	TufUrl      string
	TufRootPath string
}

type tufClientInstantiator func(o *tuf.Options) (*tuf.Client, error)

func NewTrustedRootCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	trustedRootCmd := cobra.Command{
		Use:   "trusted-root [--tuf-url <url> --tuf-root <file-path>]",
		Args:  cobra.ExactArgs(0),
		Short: "Get a trusted_root.jsonl file, likely for offline verification",
		Long: heredoc.Docf(`
			### NOTE: This feature is currently in beta, and subject to change.

            Get a trusted_root.jsonl file, likely for offline verification.

            When using %[1]sgh attestation verify%[1]s, if your machine is on the internet,
            this will happen automatically. But to do offline verification, you need to
            supply a trusted root file with %[1]s--custom-trusted-root%[1]s; this command
            will help you fetch a %[1]strusted_root.jsonl%[1]s file for that purpose.

            You can call this command without any flags to get a trusted root file covering
            the Sigstore Public Good Instance as well as GitHub's Sigstore instance.

            Otherwise you can use %[1]s--tuf-url%[1]s to specify the URL of a custom TUF
            repository mirror, and %[1]s--tuf-root%[1]s should be the path to the
            %[1]sroot.json%[1]s file that you securely obtained out-of-band.
		`, "`"),
		Example: heredoc.Doc(`
			# Get a trusted_root.jsonl for both Sigstore Public Good and GitHub's instance
			gh attestation trusted-root
		`),
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

	trustedRootCmd.Flags().StringVarP(&opts.TufUrl, "tuf-url", "", "", "URL to the TUF repository mirror")
	trustedRootCmd.Flags().StringVarP(&opts.TufRootPath, "tuf-root", "", "", "Path to the TUF root.json file on disk")
	trustedRootCmd.MarkFlagsRequiredTogether("tuf-url", "tuf-root")

	return &trustedRootCmd
}

func getTrustedRoot(makeTUF tufClientInstantiator, opts *Options) error {
	var tufOptions []*tuf.Options

	tufOpt := verification.DefaultOptionsWithCacheSetting()
	// The purpose is the verify the TUF root and repository, make
	// sure there is no caching enabled
	tufOpt.CacheValidity = 0

	if opts.TufUrl != "" && opts.TufRootPath != "" {
		tufRoot, err := os.ReadFile(opts.TufRootPath)
		if err != nil {
			return fmt.Errorf("failed to read root file %s: %v", opts.TufRootPath, err)
		}

		tufOpt.Root = tufRoot
		tufOpt.RepositoryBaseURL = opts.TufUrl
		tufOptions = append(tufOptions, tufOpt)
	} else {
		// Get from both Sigstore public good and GitHub private instance
		tufOptions = append(tufOptions, tufOpt)

		tufOpt = verification.GitHubTUFOptions()
		tufOpt.CacheValidity = 0
		tufOptions = append(tufOptions, tufOpt)
	}

	for _, tufOpt = range tufOptions {
		tufClient, err := makeTUF(tufOpt)
		if err != nil {
			return fmt.Errorf("failed to create TUF client: %v", err)
		}

		t, err := tufClient.GetTarget("trusted_root.json")
		if err != nil {
			return err
		}

		output := new(bytes.Buffer)
		err = json.Compact(output, t)
		if err != nil {
			return err
		}

		fmt.Println(output)
	}

	return nil
}
