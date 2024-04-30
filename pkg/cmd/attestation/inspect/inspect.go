package inspect

import (
	"fmt"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact/oci"
	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewInspectCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	inspectCmd := &cobra.Command{
		Use:    "inspect [<file path> | oci://<OCI image URI>] --bundle <path-to-bundle>",
		Args:   cmdutil.ExactArgs(1, "must specify file path or container image URI, as well --bundle"),
		Hidden: true,
		Short:  "Inspect a sigstore bundle",
		Long: heredoc.Docf(`
			### NOTE: This feature is currently in beta, and subject to change.

			Inspect a downloaded Sigstore bundle for a given artifact.

			The command requires either:
			* a relative path to a local artifact, or
			* a container image URI (e.g. %[1]soci://<my-OCI-image-URI>%[1]s)

			Note that if you provide an OCI URI for the artifact you must already
			be authenticated with a container registry.

			The command also requires the %[1]s--bundle%[1]s flag, which provides a file
			path to a previously downloaded Sigstore bundle. (See also the %[1]sdownload%[1]s
			command).

			By default, the command will print information about the bundle in a table format.
			If the %[1]s--json-result%[1]s flag is provided, the command will print the
			information in JSON format.
		`, "`"),
		Example: heredoc.Doc(`
			# Inspect a Sigstore bundle and print the results in table format
			$ gh attestation inspect <my-artifact> --bundle <path-to-bundle>

			# Inspect a Sigstore bundle and print the results in JSON format
			$ gh attestation inspect <my-artifact> --bundle <path-to-bundle> --json-result

			# Inspect a Sigsore bundle for an OCI artifact, and print the results in table format
			$ gh attestation inspect oci://<my-OCI-image> --bundle <path-to-bundle>
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the inspect command
			opts.Logger = io.NewHandler(f.IOStreams)

			// set the artifact path
			opts.ArtifactPath = args[0]

			// Clean file path options
			// opts.Clean()

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.OCIClient = oci.NewLiveClient()

			if err := auth.IsHostSupported(); err != nil {
				return err
			}

			if runF != nil {
				return runF(opts)
			}

			config := verification.SigstoreConfig{
				Logger: opts.Logger,
			}

			opts.SigstoreVerifier = verification.NewLiveSigstoreVerifier(config)

			if err := runInspect(opts); err != nil {
				return fmt.Errorf("Failed to inspect the artifact and bundle: %w", err)
			}
			return nil
		},
	}

	inspectCmd.Flags().StringVarP(&opts.BundlePath, "bundle", "b", "", "Path to bundle on disk, either a single bundle in a JSON file or a JSON lines file with multiple bundles")
	inspectCmd.MarkFlagRequired("bundle") //nolint:errcheck
	cmdutil.StringEnumFlag(inspectCmd, &opts.DigestAlgorithm, "digest-alg", "d", "sha256", []string{"sha256", "sha512"}, "The algorithm used to compute a digest of the artifact")
	cmdutil.AddFormatFlags(inspectCmd, &opts.exporter)

	return inspectCmd
}

func runInspect(opts *Options) error {
	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to digest artifact: %s", err)
	}

	opts.Logger.Printf("Verifying attestations for the artifact found at %s\n\n", artifact.URL)

	attestations, err := verification.GetLocalAttestations(opts.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to read attestations for subject: %s", artifact.DigestWithAlg())
	}

	policy, err := buildPolicy(*artifact)
	if err != nil {
		return fmt.Errorf("failed to build policy: %v", err)
	}

	res := opts.SigstoreVerifier.Verify(attestations, policy)
	if res.Error != nil {
		return fmt.Errorf("at least one attestation failed to verify against Sigstore: %v", res.Error)
	}

	opts.Logger.VerbosePrint(opts.Logger.ColorScheme.Green(
		"Successfully verified all attestations against Sigstore!\n\n",
	))

	// If the user provides the --format=json flag, print the results in JSON format
	if opts.exporter != nil {
		details, err := getAttestationDetails(res.VerifyResults)
		if err != nil {
			return fmt.Errorf("failed to get attestation detail: %v", err)
		}

		// print the results to the terminal as an array of JSON objects
		if err = opts.exporter.Write(opts.Logger.IO, details); err != nil {
			return fmt.Errorf("failed to write JSON output")
		}
		return nil
	}

	// otherwise, print results in a table
	details, err := getDetailsAsSlice(res.VerifyResults)
	if err != nil {
		return fmt.Errorf("failed to parse attestation details: %v", err)
	}

	headers := []string{"Repo Name", "Repo ID", "Org Name", "Org ID", "Workflow ID"}
	t := tableprinter.New(opts.Logger.IO, tableprinter.WithHeader(headers...))

	for _, row := range details {
		for _, field := range row {
			t.AddField(field, tableprinter.WithTruncate(nil))
		}
		t.EndRow()
	}

	if err = t.Render(); err != nil {
		return fmt.Errorf("failed to print output: %v", err)
	}

	return nil
}
