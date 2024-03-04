package inspect

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/logging"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewInspectCmd(f *cmdutil.Factory) *cobra.Command {
	opts := &Options{}
	inspectCmd := &cobra.Command{
		Use:   "inspect [<file path> | oci://<OCI image URI>]",
		Args:  cobra.ExactArgs(1),
		Short: "Inspect an artifact's trusted metadata bundle",
		Long: heredoc.Docf(`
			Inspect downloaded trusted metadata associated with a given artifact.
				
			The command accepts either:
			* a relative path to a local artifact
			* a container image URI (e.g. %[1]soci://<my-OCI-image-URI>%[1]s) 

			Note that you must already be authenticated with a container registry 
			if you provide an OCI image URI as the artifact.

			The command also requires you provide the path a local trusted metadata bundle with 
			the %[1]s--bundle%[1]s flag.
			You can download a trusted metadata bundle using the %[1]sdownload%[1]s command.

			By default, the command will print information about the bundle in a table format.
			If the %[1]s--json-result%[1]s flag is provided, the command will print the 
			information in JSON format.
		`, "`"),
		Example: heredoc.Doc(`
			# Inspect a local artifact bundle and print the results in table format
			$ gh attestation inspect <my-artifact> --bundle <path-to-bundle>

			# Inspect a local artifact bundle and print the results in JSON format
			$ gh attestation inspect <my-artifact> --bundle <path-to-bundle> --json-result

			# Inspect an OCI image bundle and print the results in table format
			$ gh attestation inspect oci://<my-OCI-image> --bundle <path-to-bundle>
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the inspect command
			opts.Logger = logging.NewDefaultLogger(f.IOStreams)

			// set the artifact path
			opts.ArtifactPath = args[0]

			// Check that the given flag combination is valid
			if err := opts.AreFlagsValid(); err != nil {
				return err
			}

			// Clean file path options
			opts.Clean()

			return nil
		},
		// Use Run instead of RunE because if an error is returned by RunInspect
		// when RunE is used, the command usage will be printed
		// We only want to print the error, not usage
		Run: func(cmd *cobra.Command, args []string) {
			if err := RunInspect(opts); err != nil {
				opts.Logger.Println(opts.Logger.ColorScheme.Redf("Failed to inspect the artifact and bundle: %s", err.Error()))
				os.Exit(1)
			}
		},
	}

	inspectCmd.Flags().StringVarP(&opts.BundlePath, "bundle", "b", "", "Path to bundle on disk, either a single bundle in a JSON file or a JSON lines file with multiple bundles")
	inspectCmd.MarkFlagRequired("bundle") //nolint:errcheck
	inspectCmd.Flags().StringVarP(&opts.DigestAlgorithm, "digest-alg", "d", "sha256", "The algorithm used to compute a digest of the artifact (sha256 or sha512)")
	inspectCmd.Flags().BoolVarP(&opts.JsonResult, "json-result", "j", false, "Output inspect result as JSON lines")

	return inspectCmd
}

func RunInspect(opts *Options) error {
	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	if err != nil {
		return fmt.Errorf("failed to digest artifact: %s", err)
	}

	opts.Logger.Printf("Verifying attestations for the artifact found at %s\n\n", artifact.URL)

	attestations, err := verification.GetLocalAttestations(opts.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to read attestations for subject: %s", artifact.DigestWithAlg())
	}

	config := verification.SigstoreConfig{
		Logger: opts.Logger,
	}

	policy, err := buildPolicy(*artifact)
	if err != nil {
		return fmt.Errorf("failed to build policy: %w", err)
	}

	sigstore, err := verification.NewSigstoreVerifier(config, policy)
	if err != nil {
		return err
	}

	res := sigstore.Verify(attestations)
	if res.Error != nil {
		return fmt.Errorf("at least one attestation failed to verify against Sigstore: %w", res.Error)
	}

	opts.Logger.VerbosePrint(opts.Logger.ColorScheme.Green(
		"Successfully verified all attestations against Sigstore!\n\n",
	))

	if opts.JsonResult {
		details, err := getAttestationDetails(res.VerifyResults)
		if err != nil {
			return fmt.Errorf("failed to get attestation detail: %w", err)
		}

		jsonResults := make([]string, len(details))
		for i, detail := range details {
			jsonBytes, err := json.Marshal(detail)
			if err != nil {
				return fmt.Errorf("failed to create JSON output")
			}

			jsonResults[i] = string(jsonBytes)
		}

		rows := make([][]string, 1)
		rows[0] = jsonResults
		opts.Logger.PrintTableToStdOut(nil, rows)

		return nil
	}

	details, err := getDetailsAsSlice(res.VerifyResults)
	if err != nil {
		return fmt.Errorf("failed to parse attestation details: %w", err)
	}

	headerRow := []string{"Repo Name", "Repo ID", "Org Name", "Org ID", "Workflow ID"}
	opts.Logger.PrintTableToStdOut(headerRow, details)

	return nil
}
