package download

import (
	"errors"
	"fmt"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewDownloadCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	downloadCmd := &cobra.Command{
		Use:   "download [<file-path> | oci://<image-uri>] [--owner | --repo]",
		Args:  cmdutil.ExactArgs(1, "must specify file path or container image URI, as well as one of --owner or --repo"),
		Short: "Download an artifact's attestations for offline use",
		Long: heredoc.Docf(`
			### NOTE: This feature is currently in public preview, and subject to change.

			Download attestations associated with an artifact for offline use.

			The command requires either:
			* a file path to an artifact, or
			* a container image URI (e.g. %[1]soci://<image-uri>%[1]s)
			  * (note that if you provide an OCI URL, you must already be authenticated with
			its container registry)

			In addition, the command requires either:
			* the %[1]s--repo%[1]s flag (e.g. --repo github/example).
			* the %[1]s--owner%[1]s flag (e.g. --owner github), or

			The %[1]s--repo%[1]s flag value must match the name of the GitHub repository
			that the artifact is linked with.

			The %[1]s--owner%[1]s flag value must match the name of the GitHub organization
			that the artifact's linked repository belongs to.

			Any associated bundle(s) will be written to a file in the
			current directory named after the artifact's digest. For example, if the
			digest is "sha256:1234", the file will be named "sha256:1234.jsonl".

			Colons are special characters on Windows and cannot be used in
			file names. To accommodate, a dash will be used to separate the algorithm
			from the digest in the attestations file name. For example, if the digest
			is "sha256:1234", the file will be named "sha256-1234.jsonl".
		`, "`"),
		Example: heredoc.Doc(`
			# Download attestations for a local artifact linked with an organization
			$ gh attestation download example.bin -o github

			# Download attestations for a local artifact linked with a repository
			$ gh attestation download example.bin -R github/example

			# Download attestations for an OCI image linked with an organization
			$ gh attestation download oci://example.com/foo/bar:latest -o github
		`),
		// PreRunE is used to validate flags before the command is run
		// If an error is returned, its message will be printed to the terminal
		// along with information about how use the command
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the download command
			opts.Logger = io.NewHandler(f.IOStreams)

			// set the artifact path
			opts.ArtifactPath = args[0]

			// check that the provided flags are valid
			if err := opts.AreFlagsValid(); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			hc, err := f.HttpClient()
			if err != nil {
				return err
			}

			if opts.Hostname == "" {
				opts.Hostname, _ = ghauth.DefaultHost()
			}
			if err := auth.IsHostSupported(opts.Hostname); err != nil {
				return err
			}

			opts.APIClient = api.NewLiveClient(hc, opts.Hostname, opts.Logger)
			opts.OCIClient = oci.NewLiveClient()
			opts.Store = NewLiveStore("")

			if runF != nil {
				return runF(opts)
			}

			if err := runDownload(opts); err != nil {
				return fmt.Errorf("Failed to download the artifact's bundle(s): %v", err)
			}
			return nil
		},
	}

	downloadCmd.Flags().StringVarP(&opts.Owner, "owner", "o", "", "a GitHub organization to scope attestation lookup by")
	downloadCmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Repository name in the format <owner>/<repo>")
	downloadCmd.MarkFlagsMutuallyExclusive("owner", "repo")
	downloadCmd.MarkFlagsOneRequired("owner", "repo")
	downloadCmd.Flags().StringVarP(&opts.PredicateType, "predicate-type", "", "", "Filter attestations by provided predicate type")
	cmdutil.StringEnumFlag(downloadCmd, &opts.DigestAlgorithm, "digest-alg", "d", "sha256", []string{"sha256", "sha512"}, "The algorithm used to compute a digest of the artifact")
	downloadCmd.Flags().IntVarP(&opts.Limit, "limit", "L", api.DefaultLimit, "Maximum number of attestations to fetch")
	downloadCmd.Flags().StringVarP(&opts.Hostname, "hostname", "", "", "Configure host to use")

	return downloadCmd
}

func runDownload(opts *Options) error {
	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to digest artifact: %v", err)
	}

	opts.Logger.VerbosePrintf("Downloading trusted metadata for artifact %s\n\n", opts.ArtifactPath)

	params := verification.FetchRemoteAttestationsParams{
		Digest: artifact.DigestWithAlg(),
		Limit:  opts.Limit,
		Owner:  opts.Owner,
		Repo:   opts.Repo,
	}
	attestations, err := verification.GetRemoteAttestations(opts.APIClient, params)
	if err != nil {
		if errors.Is(err, api.ErrNoAttestations{}) {
			fmt.Fprintf(opts.Logger.IO.Out, "No attestations found for %s\n", opts.ArtifactPath)
			return nil
		}
		return fmt.Errorf("failed to fetch attestations: %v", err)
	}

	// Apply predicate type filter to returned attestations
	if opts.PredicateType != "" {
		filteredAttestations := verification.FilterAttestations(opts.PredicateType, attestations)

		if len(filteredAttestations) == 0 {
			return fmt.Errorf("no attestations found with predicate type: %s", opts.PredicateType)
		}

		attestations = filteredAttestations
	}

	metadataFilePath, err := opts.Store.createMetadataFile(artifact.DigestWithAlg(), attestations)
	if err != nil {
		return fmt.Errorf("failed to write attestation: %v", err)
	}
	fmt.Fprintf(opts.Logger.IO.Out, "Wrote attestations to file %s.\nAny previous content has been overwritten\n\n", metadataFilePath)

	fmt.Fprint(opts.Logger.IO.Out,
		opts.Logger.ColorScheme.Greenf(
			"The trusted metadata is now available at %s\n", metadataFilePath,
		),
	)

	return nil
}
