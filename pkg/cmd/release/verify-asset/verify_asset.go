package verifyasset

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	att_io "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type VerifyAssetOptions struct {
	TagName       string
	BaseRepo      ghrepo.Interface
	Exporter      cmdutil.Exporter
	AssetFilePath string
	TrustedRoot   string
}

type VerifyAssetConfig struct {
	HttpClient  *http.Client
	IO          *iostreams.IOStreams
	Opts        *VerifyAssetOptions
	AttClient   api.Client
	AttVerifier shared.Verifier
}

func NewCmdVerifyAsset(f *cmdutil.Factory, runF func(*VerifyAssetConfig) error) *cobra.Command {
	opts := &VerifyAssetOptions{}

	cmd := &cobra.Command{
		Use:   "verify-asset [<tag>] <file-path>",
		Short: "Verify that a given asset originated from a release",
		Long: heredoc.Doc(`
			Verify that a given asset file originated from a specific GitHub Release using cryptographically signed attestations.

			An attestation is a claim made by GitHub regarding a release and its assets.

  			This command checks that the asset you provide matches a valid attestation for the specified release (or the latest release, if no tag is given).
			It ensures the asset's integrity by validating that the asset's digest matches the subject in the attestation and that the attestation is associated with the release.
		`),
		Args: cobra.MaximumNArgs(2),
		Example: heredoc.Doc(`
			# Verify an asset from the latest release
			$ gh release verify-asset ./dist/my-asset.zip

			# Verify an asset from a specific release tag
			$ gh release verify-asset v1.2.3 ./dist/my-asset.zip

			# Verify an asset from a specific release tag and output the attestation in JSON format
			$ gh release verify-asset v1.2.3 ./dist/my-asset.zip --format json
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 2 {
				opts.TagName = args[0]
				opts.AssetFilePath = args[1]
			} else if len(args) == 1 {
				opts.AssetFilePath = args[0]
			} else {
				return cmdutil.FlagErrorf("you must specify an asset filepath")
			}

			opts.AssetFilePath = filepath.Clean(opts.AssetFilePath)

			baseRepo, err := f.BaseRepo()
			if err != nil {
				return fmt.Errorf("failed to determine base repository: %w", err)
			}
			opts.BaseRepo = baseRepo

			httpClient, err := f.HttpClient()
			if err != nil {
				return err
			}

			io := f.IOStreams
			attClient := api.NewLiveClient(httpClient, baseRepo.RepoHost(), att_io.NewHandler(io))

			attVerifier := &shared.AttestationVerifier{
				AttClient:   attClient,
				HttpClient:  httpClient,
				IO:          io,
				TrustedRoot: opts.TrustedRoot,
			}

			config := &VerifyAssetConfig{
				Opts:        opts,
				HttpClient:  httpClient,
				AttClient:   attClient,
				AttVerifier: attVerifier,
				IO:          io,
			}

			if runF != nil {
				return runF(config)
			}

			return verifyAssetRun(config)
		},
	}
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)
	cmd.Flags().StringVarP(&opts.TrustedRoot, "custom-trusted-root", "", "", "Path to a trusted_root.jsonl file; likely for offline verification.")
	cmd.Flags().MarkHidden("custom-trusted-root")

	return cmd
}

func verifyAssetRun(config *VerifyAssetConfig) error {
	ctx := context.Background()
	opts := config.Opts
	baseRepo := opts.BaseRepo
	tagName := opts.TagName

	if tagName == "" {
		release, err := shared.FetchLatestRelease(ctx, config.HttpClient, baseRepo)
		if err != nil {
			return err
		}
		tagName = release.TagName
	}

	fileName := getFileName(opts.AssetFilePath)

	// Calculate the digest of the file
	fileDigest, err := artifact.NewDigestedArtifact(nil, opts.AssetFilePath, "sha256")
	if err != nil {
		return err
	}

	ref, err := shared.FetchRefSHA(ctx, config.HttpClient, baseRepo, tagName)
	if err != nil {
		return err
	}

	releaseRefDigest := artifact.NewDigestedArtifactForRelease(ref, "sha1")

	// Find attestations for the release tag SHA
	attestations, err := config.AttClient.GetByDigest(api.FetchParams{
		Digest:        releaseRefDigest.DigestWithAlg(),
		PredicateType: "release",
		Owner:         baseRepo.RepoOwner(),
		Repo:          baseRepo.RepoOwner() + "/" + baseRepo.RepoName(),
		// TODO: Allow this value to be set via a flag.
		// The limit is set to 100 to ensure we fetch all attestations for a given SHA.
		// While multiple attestations can exist for a single SHA,
		// only one attestation is associated with each release tag.
		Initiator: "github",
		Limit:     100,
	})
	if err != nil {
		return fmt.Errorf("no attestations found for tag %s (%s)", tagName, releaseRefDigest.DigestWithAlg())
	}

	// Filter attestations by tag name
	filteredAttestations, err := shared.FilterAttestationsByTag(attestations, tagName)
	if err != nil {
		return fmt.Errorf("error parsing attestations for tag %s: %w", tagName, err)
	}

	if len(filteredAttestations) == 0 {
		return fmt.Errorf("no attestations found for release %s in %s/%s", tagName, baseRepo.RepoOwner(), baseRepo.RepoName())
	}

	// Filter attestations by subject digest
	filteredAttestations, err = shared.FilterAttestationsByFileDigest(filteredAttestations, fileDigest.Digest())
	if err != nil {
		return fmt.Errorf("error parsing attestations for digest %s: %w", fileDigest.DigestWithAlg(), err)
	}

	if len(filteredAttestations) == 0 {
		return fmt.Errorf("attestation for %s does not contain subject %s", tagName, fileDigest.DigestWithAlg())
	}

	// Verify attestation
	verified, err := config.AttVerifier.VerifyAttestation(releaseRefDigest, filteredAttestations[0])
	if err != nil {
		return fmt.Errorf("failed to verify attestation for tag %s: %w", tagName, err)
	}

	// If an exporter is provided with the --json flag, write the results to the terminal in JSON format
	if opts.Exporter != nil {
		return opts.Exporter.Write(config.IO, verified)
	}

	io := config.IO
	cs := io.ColorScheme()
	fmt.Fprintf(io.Out, "Calculated digest for %s: %s\n", fileName, fileDigest.DigestWithAlg())
	fmt.Fprintf(io.Out, "Resolved tag %s to %s\n", tagName, releaseRefDigest.DigestWithAlg())
	fmt.Fprint(io.Out, "Loaded attestation from GitHub API\n\n")
	fmt.Fprintf(io.Out, cs.Green("%s Verification succeeded! %s is present in release %s\n"), cs.SuccessIcon(), fileName, tagName)

	return nil
}

func getFileName(filePath string) string {
	// Get the file name from the file path
	_, fileName := filepath.Split(filePath)
	return fileName
}
