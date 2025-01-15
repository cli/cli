package inspect

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmdutil"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
	"github.com/digitorus/timestamp"
	in_toto "github.com/in-toto/attestation/go/v1"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
)

func NewInspectCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	inspectCmd := &cobra.Command{
		Use:    "inspect <path-to-sigstore-bundle>",
		Args:   cmdutil.ExactArgs(1, "must specify bundle file path"),
		Hidden: true,
		Short:  "Inspect a Sigstore bundle",
		Long: heredoc.Docf(`
			Inspect a Sigstore bundle that has been downloaded to disk. To download bundles
			associated with your artifact(s), see the %[1]sgh at download%[1]s command.

			Given a .json or .jsonl file, this command will:
			- Extract the bundle's statement and predicate
			- Provide a certificate summary, if present, and indicate whether the cert
			  was issued by GitHub or by Sigstore's Public Good Instance (PGI)
			- Check the bundles' "authenticity"

			For our purposes, a bundle is authentic if we have the trusted materials to
			verify the included certificate(s), transparency log entries, and signed
			timestamps, and if the included signatures match the provided public key.

			This command cannot be used to verify a bundle. To verify a bundle, see the
		 %[1]sgh at verify%[1]s command.

			By default, this command prints a condensed table. To see full results, provide the
			%[1]s--format=json%[1]s flag.
		`, "`"),
		Example: heredoc.Doc(`
			# Inspect a Sigstore bundle and print the results in table format
			$ gh attestation inspect <path-to-bundle>

			# Inspect a Sigstore bundle and print the results in JSON format
			$ gh attestation inspect <path-to-bundle> --format=json
		`),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the inspect command
			opts.Logger = io.NewHandler(f.IOStreams)

			// set the bundle path
			opts.BundlePath = args[0]

			// Clean file path options
			opts.Clean()

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// handle tenancy
			if opts.Hostname == "" {
				opts.Hostname, _ = ghauth.DefaultHost()
			}

			err := auth.IsHostSupported(opts.Hostname)
			if err != nil {
				return err
			}

			config := verification.SigstoreConfig{
				Logger: opts.Logger,
			}

			if ghauth.IsTenancy(opts.Hostname) {
				hc, err := f.HttpClient()
				if err != nil {
					return err
				}
				apiClient := api.NewLiveClient(hc, opts.Hostname, opts.Logger)
				td, err := apiClient.GetTrustDomain()
				if err != nil {
					return fmt.Errorf("error getting trust domain, make sure you are authenticated against the host: %w", err)
				}
				_, found := ghinstance.TenantName(opts.Hostname)
				if !found {
					return fmt.Errorf("invalid hostname provided: '%s'",
						opts.Hostname)
				}

				config.TrustDomain = td
			}

			opts.SigstoreVerifier = verification.NewLiveSigstoreVerifier(config)

			if runF != nil {
				return runF(opts)
			}

			if err := runInspect(opts); err != nil {
				return fmt.Errorf("Failed to inspect the artifact and bundle: %w", err)
			}
			return nil
		},
	}

	inspectCmd.Flags().StringVarP(&opts.Hostname, "hostname", "", "", "Configure host to use")
	cmdutil.AddFormatFlags(inspectCmd, &opts.exporter)

	return inspectCmd
}

type BundleInspectResult struct {
	InspectedBundles []BundleInspection `json:"inspectedBundles"`
}

type BundleInspection struct {
	Authentic              bool                  `json:"authentic"`
	Certificate            CertificateInspection `json:"certificate"`
	TransparencyLogEntries []TlogEntryInspection `json:"transparencyLogEntries"`
	SignedTimestamps       []time.Time           `json:"signedTimestamps"`
	Statement              *in_toto.Statement    `json:"statement"`
}

type CertificateInspection struct {
	certificate.Summary
	NotBefore time.Time `json:"notBefore"`
	NotAfter  time.Time `json:"notAfter"`
}

type TlogEntryInspection struct {
	IntegratedTime time.Time
	LogID          string
}

func runInspect(opts *Options) error {
	attestations, err := verification.GetLocalAttestations(opts.BundlePath)
	if err != nil {
		return fmt.Errorf("failed to read attestations")
	}

	inspectedBundles := []BundleInspection{}
	unsafeSigstorePolicy := verify.NewPolicy(verify.WithoutArtifactUnsafe(), verify.WithoutIdentitiesUnsafe())

	for _, a := range attestations {
		inspectedBundle := BundleInspection{}

		// we ditch the verificationResult to avoid even implying that it is "verified"
		// you can't meaningfully "verify" a bundle with such an Unsafe policy!
		_, err := opts.SigstoreVerifier.Verify([]*api.Attestation{a}, unsafeSigstorePolicy)

		// food for thought for later iterations:
		// if the err is present, we keep on going because we want to be able to
		// inspect bundles we might not have trusted materials for.
		// but maybe we should print the error?
		if err == nil {
			inspectedBundle.Authentic = true
		}

		entity := a.Bundle
		verificationContent, err := entity.VerificationContent()
		if err != nil {
			return fmt.Errorf("failed to fetch verification content: %w", err)
		}

		// summarize cert if present
		if leafCert := verificationContent.GetCertificate(); leafCert != nil {

			certSummary, err := certificate.SummarizeCertificate(leafCert)
			if err != nil {
				return fmt.Errorf("failed to summarize certificate: %w", err)
			}

			inspectedBundle.Certificate = CertificateInspection{
				Summary:   certSummary,
				NotBefore: leafCert.NotBefore,
				NotAfter:  leafCert.NotAfter,
			}

		}

		// parse the sig content and pop the statement
		sigContent, err := entity.SignatureContent()
		if err != nil {
			return fmt.Errorf("failed to fetch signature content: %w", err)
		}

		if envelope := sigContent.EnvelopeContent(); envelope != nil {
			stmt, err := envelope.Statement()
			if err != nil {
				return fmt.Errorf("failed to fetch envelope statement: %w", err)
			}

			inspectedBundle.Statement = stmt
		}

		// fetch the observer timestamps
		tlogTimestamps, err := dumpTlogs(entity)
		if err != nil {
			return fmt.Errorf("failed to dump tlog: %w", err)
		}
		inspectedBundle.TransparencyLogEntries = tlogTimestamps

		signedTimestamps, err := dumpSignedTimestamps(entity)
		if err != nil {
			return fmt.Errorf("failed to dump tsa: %w", err)
		}
		inspectedBundle.SignedTimestamps = signedTimestamps

		inspectedBundles = append(inspectedBundles, inspectedBundle)
	}

	inspectionResult := BundleInspectResult{InspectedBundles: inspectedBundles}

	// If the user provides the --format=json flag, print the results in JSON format
	if opts.exporter != nil {
		if err = opts.exporter.Write(opts.Logger.IO, inspectionResult); err != nil {
			return fmt.Errorf("failed to write JSON output")
		}
		return nil
	}

	printInspectionSummary(opts.Logger, inspectionResult.InspectedBundles)

	return nil
}

func printInspectionSummary(logger *io.Handler, bundles []BundleInspection) {
	logger.Printf("Inspecting bundles…\n")
	logger.Printf("Found %s:\n---\n", text.Pluralize(len(bundles), "attestation"))

	bundleSummaries := make([][][]string, len(bundles))
	for i, iB := range bundles {
		bundleSummaries[i] = [][]string{
			{"Authentic", formatAuthentic(iB.Authentic, iB.Certificate.CertificateIssuer)},
			{"Source Repo", formatNwo(iB.Certificate.SourceRepositoryURI)},
			{"PredicateType", iB.Statement.GetPredicateType()},
			{"SubjectAlternativeName", iB.Certificate.SubjectAlternativeName},
			{"RunInvocationURI", iB.Certificate.RunInvocationURI},
			{"CertificateNotBefore", iB.Certificate.NotBefore.Format(time.RFC3339)},
		}
	}

	// "SubjectAlternativeName" has 22 chars
	maxNameLength := 22

	scheme := logger.ColorScheme
	for i, bundle := range bundleSummaries {
		for _, pair := range bundle {
			colName := pair[0]
			dots := maxNameLength - len(colName)
			logger.OutPrintf("%s:%s %s\n", scheme.Bold(colName), strings.Repeat(".", dots), pair[1])
		}
		if i < len(bundleSummaries)-1 {
			logger.OutPrintln("---")
		}
	}
}

func formatNwo(longUrl string) string {
	repo, err := ghrepo.FromFullName(longUrl)
	if err != nil {
		return longUrl
	}

	return ghrepo.FullName(repo)
}

func formatAuthentic(authentic bool, certIssuer string) string {
	if strings.HasSuffix(certIssuer, "O=GitHub\\, Inc.") {
		certIssuer = "(GitHub)"
	} else if strings.HasSuffix(certIssuer, "O=sigstore.dev") {
		certIssuer = "(Sigstore PGI)"
	} else {
		certIssuer = "(Unknown)"
	}

	return strconv.FormatBool(authentic) + " " + certIssuer
}

func dumpTlogs(entity *bundle.Bundle) ([]TlogEntryInspection, error) {
	inspectedTlogEntries := []TlogEntryInspection{}

	entries, err := entity.TlogEntries()
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		inspectedEntry := TlogEntryInspection{
			IntegratedTime: entry.IntegratedTime(),
			LogID:          entry.LogKeyID(),
		}

		inspectedTlogEntries = append(inspectedTlogEntries, inspectedEntry)
	}

	return inspectedTlogEntries, nil
}

func dumpSignedTimestamps(entity *bundle.Bundle) ([]time.Time, error) {
	timestamps := []time.Time{}

	signedTimestamps, err := entity.Timestamps()
	if err != nil {
		return nil, err
	}

	for _, signedTsBytes := range signedTimestamps {
		tsaTime, err := timestamp.ParseResponse(signedTsBytes)

		if err != nil {
			return nil, err
		}

		timestamps = append(timestamps, tsaTime.Time)
	}

	return timestamps, nil
}
