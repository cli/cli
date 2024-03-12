package download

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewDownloadCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{}
	downloadCmd := &cobra.Command{
		Use:   "download [<file path> | oci://<OCI image URI>]",
		Args:  cobra.ExactArgs(1),
		Short: "Download an artifact's Sigstore bundle(s) for offline use",
		Long: heredoc.Docf(`
			Download an artifact's Sigstore bundle(s) for offline use.
			
			The command requires either:
			* a relative path to a local artifact, or
			* a container image URI (e.g. %[1]soci://<my-OCI-image-URI>%[1]s)

			Note that if you provide an OCI URI for the artifact you must already
			be authenticated with a container registry.

			In addition, the command also requires either:
			* the %[1]s--owner%[1]s flag (e.g. github), or
			* the %[1]s--repo%[1]s flag (e.g. github/example).

			The value of the %[1]s--owner%[1]s flag must match the name of the GitHub
			organization that the artifact is associated with.

			The value of the %[1]s--repo%[1]s flag must match the name of the GitHub
			repository that the artifact is associated with.

			The corresponding Sigstore bundle(s) will be written to a file in the
			current directory named after the artifact's digest. For example, if the
			artifact's digest is "sha256:1234", the file will be named "sha256:1234.jsonl".
		`, "`"),
		Example: heredoc.Doc(`
			# Download Sigstore bundle(s) for a local artifact associated with a GitHub organization
			$ gh attestation download <my-artifact> -o <GitHub organization>

			# Download Sigstore bundle(s) for a local artifact associated with a GitHub repository
			$ gh attestation download <my-artifact> -R <GitHub repo>

			# Download Sigstore bundle(s) for an OCI image associated with a GitHub organization
			$ gh attestation download oci://<my-OCI-image> -o <GitHub organization>
		`),
		// PreRunE is used to validate flags before the command is run
		// If an error is returned, its message will be printed to the terminal
		// along with information about how use the command
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the download command
			opts.Logger = logging.NewLogger(f.IOStreams, false, opts.Verbose)

			// set the artifact path
			opts.ArtifactPath = args[0]

			// check that the provided flags are valid
			if err := opts.AreFlagsValid(); err != nil {
				return err
			}
			return nil
		},
		// Use Run instead of RunE because if an error is returned by RunVerify
		// when RunE is used, the command usage will be printed
		// We only want to print the error, not usage
		Run: func(cmd *cobra.Command, args []string) {
			hc, err := f.HttpClient()
			if err != nil {
				opts.Logger.Println(opts.Logger.ColorScheme.Red(err.Error()))
				os.Exit(1)
			}
			opts.APIClient = api.NewLiveClient(hc, opts.Logger)

			opts.OCIClient = oci.NewLiveClient()

			if err := auth.IsHostSupported(); err != nil {
				opts.Logger.Println(opts.Logger.ColorScheme.Red(err.Error()))
				os.Exit(1)
			}
			if err := RunDownload(opts); err != nil {
				opts.Logger.ColorScheme.Redf("Failed to download the artifact's bundle(s): %s", err.Error())
				os.Exit(1)
			}
		},
	}

	downloadCmd.Flags().StringVarP(&opts.Owner, "owner", "o", "", "a GitHub organization to scope attestation lookup by")
	downloadCmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Repository name in the format <owner>/<repo>")
	downloadCmd.MarkFlagsMutuallyExclusive("owner", "repo")
	downloadCmd.MarkFlagsOneRequired("owner", "repo")
	cmdutil.StringEnumFlag(downloadCmd, &opts.DigestAlgorithm, "digest-alg", "d", "sha256", []string{"sha256", "sha512"}, "The algorithm used to compute a digest of the artifact")
	downloadCmd.Flags().IntVarP(&opts.Limit, "limit", "L", api.DefaultLimit, "Maximum number of attestations to fetch")
	downloadCmd.Flags().BoolVarP(&opts.Verbose, "verbose", "v", false, "If set to true, the CLI will output verbose information.")

	return downloadCmd
}

func RunDownload(opts *Options) error {
	if opts.APIClient == nil {
		return fmt.Errorf("missing API client")
	}
	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to digest artifact: %w", err)
	}

	opts.Logger.VerbosePrintf("Downloading trusted metadata for artifact %s\n\n", opts.ArtifactPath)

	c := verification.FetchAttestationsConfig{
		APIClient: opts.APIClient,
		Digest:    artifact.DigestWithAlg(),
		Limit:     opts.Limit,
		Owner:     opts.Owner,
		Repo:      opts.Repo,
	}
	attestations, err := verification.GetRemoteAttestations(c)
	if err != nil {
		return fmt.Errorf("failed to fetch attestations: %w", err)
	}

	if attestations == nil {
		fmt.Fprintf(opts.Logger.IO.Out, "No attestations found for %s\n", opts.ArtifactPath)
		return nil
	}

	filePath := createJSONLinesFilePath(artifact.DigestWithAlg(), opts.OutputPath)
	fmt.Fprintf(opts.Logger.IO.Out, "Writing attestations to file %s.\nAny previous content will be overwritten\n\n", filePath)

	metadataFilePath, err := createMetadataFile(attestations, filePath)
	if err != nil {
		return fmt.Errorf("failed to write attestation: %w", err)
	}

	fmt.Fprint(opts.Logger.IO.Out,
		opts.Logger.ColorScheme.Greenf(
			"The trusted metadata is now available at %s\n", metadataFilePath,
		),
	)

	return nil
}

func createJSONLinesFilePath(artifact, outputPath string) string {
	path := fmt.Sprintf("%s.jsonl", artifact)
	if outputPath != "" {
		return fmt.Sprintf("%s/%s", outputPath, path)
	}
	return path
}

func createMetadataFile(attestationsResp []*api.Attestation, filePath string) (string, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create trusted metadata file: %w", err)
	}

	for _, resp := range attestationsResp {
		bundle := resp.Bundle
		attBytes, err := json.Marshal(bundle)
		if err != nil {
			return "", fmt.Errorf("failed to marshall attestation to JSON: %w", err)
		}

		withNewline := fmt.Sprintf("%s\n", attBytes)
		_, err = f.Write([]byte(withNewline))
		if err != nil {
			if err = f.Close(); err != nil {
				return "", fmt.Errorf("failed to close file while handling write error: %w", err)
			}

			return "", fmt.Errorf("failed to write trusted metadata: %w", err)
		}
	}

	if err = f.Close(); err != nil {
		return "", fmt.Errorf("failed to close file after writing metadata: %w", err)
	}

	return filePath, nil
}
