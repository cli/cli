package verify

import (
	"context"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	v1 "github.com/in-toto/attestation/go/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	att_io "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/spf13/cobra"
)

type VerifyOptions struct {
	TagName  string
	BaseRepo ghrepo.Interface
	Exporter cmdutil.Exporter
}

type VerifyConfig struct {
	HttpClient  *http.Client
	IO          *iostreams.IOStreams
	Opts        *VerifyOptions
	AttClient   api.Client
	AttVerifier shared.Verifier
}

func NewCmdVerify(f *cmdutil.Factory, runF func(config *VerifyConfig) error) *cobra.Command {
	opts := &VerifyOptions{}

	cmd := &cobra.Command{
		Use:    "verify [<tag>]",
		Short:  "Verify the attestation for a GitHub Release.",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		Long: heredoc.Doc(`
			Verify that a GitHub Release is accompanied by a valid cryptographically signed attestation.

			## Understanding Verification

			An attestation is a claim made by GitHub regarding a release and its assets.

			## What This Command Does

			This command checks that the specified release (or the latest release, if no tag is given) has a valid attestation.
			It fetches the attestation for the release and prints out metadata about all assets referenced in the attestation, including their digests.
		`),
		Example: heredoc.Doc(`
			# Verify the latest release
			gh release verify
			
			# Verify a specific release by tag
			gh release verify v1.2.3

			# Verify a specific release by tag and output the attestation in JSON format
			gh release verify v1.2.3 --format json
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.TagName = args[0]
			}

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
				AttClient:  attClient,
				HttpClient: httpClient,
				IO:         io,
			}

			config := &VerifyConfig{
				Opts:        opts,
				HttpClient:  httpClient,
				AttClient:   attClient,
				AttVerifier: attVerifier,
				IO:          io,
			}

			if runF != nil {
				return runF(config)
			}
			return verifyRun(config)
		},
	}
	cmdutil.AddFormatFlags(cmd, &opts.Exporter)

	return cmd
}

func verifyRun(config *VerifyConfig) error {
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

	// Retrieve the ref for the release tag
	ref, err := shared.FetchRefSHA(ctx, config.HttpClient, baseRepo, tagName)
	if err != nil {
		return err
	}

	releaseRefDigest := artifact.NewDigestedArtifactForRelease(ref, "sha1")

	// Find all the attestations for the release tag SHA
	attestations, err := config.AttClient.GetByDigest(api.FetchParams{
		Digest:        releaseRefDigest.DigestWithAlg(),
		PredicateType: shared.ReleasePredicateType,
		Owner:         baseRepo.RepoOwner(),
		Repo:          baseRepo.RepoOwner() + "/" + baseRepo.RepoName(),
		// TODO: Allow this value to be set via a flag.
		// The limit is set to 100 to ensure we fetch all attestations for a given SHA.
		// While multiple attestations can exist for a single SHA,
		// only one attestation is associated with each release tag.
		Limit: 100,
	})
	if err != nil {
		return fmt.Errorf("no attestations for tag %s (%s)", tagName, releaseRefDigest.DigestWithAlg())
	}

	// Filter attestations by tag name
	filteredAttestations, err := shared.FilterAttestationsByTag(attestations, tagName)
	if err != nil {
		return fmt.Errorf("error parsing attestations for tag %s: %w", tagName, err)
	}

	if len(filteredAttestations) == 0 {
		return fmt.Errorf("no attestations found for release %s in %s", tagName, baseRepo.RepoName())
	}

	if len(filteredAttestations) > 1 {
		return fmt.Errorf("duplicate attestations found for release %s in %s", tagName, baseRepo.RepoName())
	}

	// Verify attestation
	verified, err := config.AttVerifier.VerifyAttestation(releaseRefDigest, filteredAttestations[0])
	if err != nil {
		return fmt.Errorf("failed to verify attestations for tag %s: %w", tagName, err)
	}

	// If an exporter is provided with the --json flag, write the results to the terminal in JSON format
	if opts.Exporter != nil {
		return opts.Exporter.Write(config.IO, verified)
	}

	io := config.IO
	cs := io.ColorScheme()
	fmt.Fprintf(io.Out, "Resolved tag %s to %s\n", tagName, releaseRefDigest.DigestWithAlg())
	fmt.Fprint(io.Out, "Loaded attestation from GitHub API\n")
	fmt.Fprintf(io.Out, cs.Green("%s Release %s verified!\n"), cs.SuccessIcon(), tagName)
	fmt.Fprintln(io.Out)

	if err := printVerifiedSubjects(io, verified); err != nil {
		return err
	}

	return nil
}

func printVerifiedSubjects(io *iostreams.IOStreams, att *verification.AttestationProcessingResult) error {
	cs := io.ColorScheme()
	w := io.Out

	statement := att.Attestation.Bundle.GetDsseEnvelope().Payload
	var statementData v1.Statement

	err := protojson.Unmarshal([]byte(statement), &statementData)
	if err != nil {
		return err
	}

	// If there aren't at least two subjects, there are no assets to display
	if len(statementData.Subject) < 2 {
		return nil
	}

	fmt.Fprintln(w, cs.Bold("Assets"))
	table := tableprinter.New(io, tableprinter.WithHeader("Name", "Digest"))

	for _, s := range statementData.Subject {
		name := s.Name
		digest := s.Digest

		if name != "" {
			digestStr := ""
			for key, value := range digest {
				digestStr = key + ":" + value
			}

			table.AddField(name)
			table.AddField(digestStr)
			table.EndRow()
		}
	}
	err = table.Render()
	if err != nil {
		return err
	}
	fmt.Fprintln(w)

	return nil
}
