package verify

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/text"
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

func NewVerifyCmd(f *cmdutil.Factory, runF func(*Options) error) *cobra.Command {
	opts := &Options{}
	verifyCmd := &cobra.Command{
		Use:   "verify [<file-path> | oci://<image-uri>] [--owner | --repo]",
		Args:  cmdutil.ExactArgs(1, "must specify file path or container image URI, as well as one of --owner or --repo"),
		Short: "Verify an artifact's integrity using attestations",
		Long: heredoc.Docf(`
			Verify the integrity and provenance of an artifact using its associated
			cryptographically signed attestations.

			## Understanding Verification

			An attestation is a claim (i.e. a provenance statement) made by an actor
			(i.e. a GitHub Actions workflow) regarding a subject (i.e. an artifact).

			In order to verify an attestation, you must provide an artifact and validate:
			* the identity of the actor that produced the attestation
			* the expected attestation predicate type (the nature of the claim)

			By default, this command enforces the %[1]s%[2]s%[1]s
			predicate type. To verify other attestation predicate types use the
			%[1]s--predicate-type%[1]s flag.

			The "actor identity" consists of:
			* the repository or the repository owner the artifact is linked with
			* the Actions workflow that produced the attestation (a.k.a the
			  signer workflow)

			This identity is then validated against the attestation's certificate's
			SourceRepository, SourceRepositoryOwner, and SubjectAlternativeName
			(SAN) fields, among others.

			It is up to you to decide how precisely you want to enforce this identity.

			At a minimum, this command requires either:
			* the %[1]s--owner%[1]s flag (e.g. --owner github), or
			* the %[1]s--repo%[1]s flag (e.g. --repo github/example)

			The more precisely you specify the identity, the more control you will
			have over the security guarantees offered by the verification process.

			Ideally, the path of the signer workflow is also validated using the
			%[1]s--signer-workflow%[1]s or %[1]s--cert-identity%[1]s flags.

			Please note: if your attestation was generated via a reusable workflow then
			that reusable workflow is the signer whose identity needs to be validated.
			In this situation, you must use either the %[1]s--signer-workflow%[1]s or
			the %[1]s--signer-repo%[1]s flag.

			For more options, see the other available flags.

			## Loading Artifacts And Attestations

			To specify the artifact, this command requires:
			* a file path to an artifact, or
			* a container image URI (e.g. %[1]soci://<image-uri>%[1]s)
			  * (note that if you provide an OCI URL, you must already be authenticated with
			its container registry)

			By default, this command will attempt to fetch relevant attestations via the
			GitHub API using the values provided to %[1]s--owner%[1]s or  %[1]s--repo%[1]s.

			To instead fetch attestations from your artifact's OCI registry, use the
			%[1]s--bundle-from-oci%[1]s flag.

			For offline verification using attestations stored on disk (c.f. the download command)
			provide a path to the %[1]s--bundle%[1]s flag.

			## Additional Policy Enforcement

			Given the %[1]s--format=json%[1]s flag, upon successful verification this
			command will output a JSON array containing one entry per verified attestation.

			This output can then be used for additional policy enforcement, i.e. by being
			piped into a policy engine.

			Each object in the array contains two properties:
			* an %[1]sattestation%[1]s object, which contains the bundle that was verified
			* a %[1]sverificationResult%[1]s object, which is a parsed representation of the
			  contents of the bundle that was verified.

			Within the %[1]sverificationResult%[1]s object you will find:
			* %[1]ssignature.certificate%[1]s, which is a parsed representation of the X.509
			  certificate embedded in the attestation,
			* %[1]sverifiedTimestamps%[1]s, an array of objects denoting when the attestation
			  was witnessed by a transparency log or a timestamp authority
			* %[1]sstatement%[1]s, which contains the %[1]ssubject%[1]s array referencing artifacts,
			  the %[1]spredicateType%[1]s field, and the %[1]spredicate%[1]s object which contains
			  additional, often user-controllable, metadata

			IMPORTANT: please note that only the %[1]ssignature.certificate%[1]s and the
			%[1]sverifiedTimestamps%[1]s properties contain values that cannot be
			manipulated by the workflow that originated the attestation.

			When dealing with attestations created within GitHub Actions, the contents of
			%[1]ssignature.certificate%[1]s are populated directly from the OpenID Connect
			token that GitHub has generated. The contents of the %[1]sverifiedTimestamps%[1]s
			array are populated from the signed timestamps originating from either a
			transparency log or a timestamp authority – and likewise cannot be forged by users.

			When designing policy enforcement using this output, special care must be taken
			when examining the contents of the %[1]sstatement.predicate%[1]s property:
			should an attacker gain access to your workflow's execution context, they
			could then falsify the contents of the %[1]sstatement.predicate%[1]s.

			To mitigate this attack vector, consider using a "trusted builder": when generating
			an artifact, have the build and attestation signing occur within a reusable workflow
			whose execution cannot be influenced by input provided through the caller workflow.

			See above re: %[1]s--signer-workflow%[1]s.
			`, "`", verification.SLSAPredicateV1),
		Example: heredoc.Doc(`
			# Verify an artifact linked with a repository
			$ gh attestation verify example.bin --repo github/example

			# Verify an artifact linked with an organization
			$ gh attestation verify example.bin --owner github

			# Verify an artifact and output the full verification result
			$ gh attestation verify example.bin --owner github --format json

			# Verify an OCI image using attestations stored on disk
			$ gh attestation verify oci://<image-uri> --owner github --bundle sha256:foo.jsonl

			# Verify an artifact signed with a reusable workflow
			$ gh attestation verify example.bin --owner github --signer-repo actions/example
		`),
		// PreRunE is used to validate flags before the command is run
		// If an error is returned, its message will be printed to the terminal
		// along with information about how use the command
		PreRunE: func(cmd *cobra.Command, args []string) error {
			// Create a logger for use throughout the verify command
			opts.Logger = io.NewHandler(f.IOStreams)

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
		RunE: func(cmd *cobra.Command, args []string) error {
			hc, err := f.HttpClient()
			if err != nil {
				return err
			}

			opts.OCIClient = oci.NewLiveClient()

			if opts.Hostname == "" {
				opts.Hostname, _ = ghauth.DefaultHost()
			}
			err = auth.IsHostSupported(opts.Hostname)
			if err != nil {
				return err
			}

			opts.APIClient = api.NewLiveClient(hc, opts.Hostname, opts.Logger)

			config := verification.SigstoreConfig{
				TrustedRoot:  opts.TrustedRoot,
				Logger:       opts.Logger,
				NoPublicGood: opts.NoPublicGood,
			}

			// Prepare for tenancy if detected
			if ghauth.IsTenancy(opts.Hostname) {
				td, err := opts.APIClient.GetTrustDomain()
				if err != nil {
					return fmt.Errorf("error getting trust domain, make sure you are authenticated against the host: %w", err)
				}

				tenant, found := ghinstance.TenantName(opts.Hostname)
				if !found {
					return fmt.Errorf("invalid hostname provided: '%s'",
						opts.Hostname)
				}
				config.TrustDomain = td
				opts.Tenant = tenant
			}

			if runF != nil {
				return runF(opts)
			}

			sigstoreVerifier, err := verification.NewLiveSigstoreVerifier(config)
			if err != nil {
				return fmt.Errorf("error creating Sigstore verifier: %w", err)
			}
			opts.SigstoreVerifier = sigstoreVerifier
			opts.Config = f.Config

			if err := runVerify(opts); err != nil {
				return fmt.Errorf("\nError: %v", err)
			}
			return nil
		},
	}

	// general flags
	verifyCmd.Flags().StringVarP(&opts.BundlePath, "bundle", "b", "", "Path to bundle on disk, either a single bundle in a JSON file or a JSON lines file with multiple bundles")
	cmdutil.DisableAuthCheckFlag(verifyCmd.Flags().Lookup("bundle"))
	verifyCmd.Flags().BoolVarP(&opts.UseBundleFromRegistry, "bundle-from-oci", "", false, "When verifying an OCI image, fetch the attestation bundle from the OCI registry instead of from GitHub")
	cmdutil.StringEnumFlag(verifyCmd, &opts.DigestAlgorithm, "digest-alg", "d", "sha256", []string{"sha256", "sha512"}, "The algorithm used to compute a digest of the artifact")
	verifyCmd.Flags().StringVarP(&opts.Owner, "owner", "o", "", "GitHub organization to scope attestation lookup by")
	verifyCmd.Flags().StringVarP(&opts.Repo, "repo", "R", "", "Repository name in the format <owner>/<repo>")
	verifyCmd.MarkFlagsMutuallyExclusive("owner", "repo")
	verifyCmd.MarkFlagsOneRequired("owner", "repo")
	verifyCmd.Flags().BoolVarP(&opts.NoPublicGood, "no-public-good", "", false, "Do not verify attestations signed with Sigstore public good instance")
	verifyCmd.Flags().StringVarP(&opts.TrustedRoot, "custom-trusted-root", "", "", "Path to a trusted_root.jsonl file; likely for offline verification")
	verifyCmd.Flags().IntVarP(&opts.Limit, "limit", "L", api.DefaultLimit, "Maximum number of attestations to fetch")
	cmdutil.AddFormatFlags(verifyCmd, &opts.exporter)
	verifyCmd.Flags().StringVarP(&opts.Hostname, "hostname", "", "", "Configure host to use")
	// policy enforcement flags
	verifyCmd.Flags().StringVarP(&opts.PredicateType, "predicate-type", "", verification.SLSAPredicateV1, "Enforce that verified attestations' predicate type matches the provided value")
	verifyCmd.Flags().BoolVarP(&opts.DenySelfHostedRunner, "deny-self-hosted-runners", "", false, "Fail verification for attestations generated on self-hosted runners")
	verifyCmd.Flags().StringVarP(&opts.SAN, "cert-identity", "", "", "Enforce that the certificate's SubjectAlternativeName matches the provided value exactly")
	verifyCmd.Flags().StringVarP(&opts.SANRegex, "cert-identity-regex", "i", "", "Enforce that the certificate's SubjectAlternativeName matches the provided regex")
	verifyCmd.Flags().StringVarP(&opts.SignerRepo, "signer-repo", "", "", "Enforce that the workflow that signed the attestation's repository matches the provided value (<owner>/<repo>)")
	verifyCmd.Flags().StringVarP(&opts.SignerWorkflow, "signer-workflow", "", "", "Enforce that the workflow that signed the attestation matches the provided value ([host/]<owner>/<repo>/<path>/<to>/<workflow>)")
	verifyCmd.MarkFlagsMutuallyExclusive("cert-identity", "cert-identity-regex", "signer-repo", "signer-workflow")
	verifyCmd.Flags().StringVarP(&opts.OIDCIssuer, "cert-oidc-issuer", "", verification.GitHubOIDCIssuer, "Enforce that the issuer of the OIDC token matches the provided value")
	verifyCmd.Flags().StringVarP(&opts.SignerDigest, "signer-digest", "", "", "Enforce that the digest associated with the signer workflow matches the provided value")
	verifyCmd.Flags().StringVarP(&opts.SourceRef, "source-ref", "", "", "Enforce that the git ref associated with the source repository matches the provided value")
	verifyCmd.Flags().StringVarP(&opts.SourceDigest, "source-digest", "", "", "Enforce that the digest associated with the source repository matches the provided value")

	return verifyCmd
}

func runVerify(opts *Options) error {
	ec, err := newEnforcementCriteria(opts)
	if err != nil {
		opts.Logger.Println(opts.Logger.ColorScheme.Red("✗ Failed to build verification policy"))
		return err
	}

	if err := ec.Valid(); err != nil {
		opts.Logger.Println(opts.Logger.ColorScheme.Red("✗ Invalid verification policy"))
		return err
	}

	artifact, err := artifact.NewDigestedArtifact(opts.OCIClient, opts.ArtifactPath, opts.DigestAlgorithm)
	if err != nil {
		opts.Logger.Printf(opts.Logger.ColorScheme.Red("✗ Loading digest for %s failed\n"), opts.ArtifactPath)
		return err
	}

	opts.Logger.Printf("Loaded digest %s for %s\n", artifact.DigestWithAlg(), artifact.URL)

	attestations, logMsg, err := getAttestations(opts, *artifact)
	if err != nil {
		if ok := errors.Is(err, api.ErrNoAttestationsFound); ok {
			opts.Logger.Printf(opts.Logger.ColorScheme.Red("✗ No attestations found for subject %s\n"), artifact.DigestWithAlg())
			return err
		}
		// Print the message signifying failure fetching attestations
		opts.Logger.Println(opts.Logger.ColorScheme.Red(logMsg))
		return err
	}
	// Print the message signifying success fetching attestations
	opts.Logger.Println(logMsg)

	// Apply predicate type filter to returned attestations
	filteredAttestations := verification.FilterAttestations(ec.PredicateType, attestations)
	if len(filteredAttestations) == 0 {
		opts.Logger.Printf(opts.Logger.ColorScheme.Red("✗ No attestations found with predicate type: %s\n"), opts.PredicateType)
		return fmt.Errorf("no matching predicate found")
	}
	attestations = filteredAttestations

	// print information about the policy that will be enforced against attestations
	opts.Logger.Println("\nThe following policy criteria will be enforced:")
	opts.Logger.Println(ec.BuildPolicyInformation())

	verified, errMsg, err := verifyAttestations(*artifact, attestations, opts.SigstoreVerifier, ec)
	if err != nil {
		opts.Logger.Println(opts.Logger.ColorScheme.Red(errMsg))
		return err
	}

	opts.Logger.Println(opts.Logger.ColorScheme.Green("✓ Verification succeeded!\n"))

	// If an exporter is provided with the --json flag, write the results to the terminal in JSON format
	if opts.exporter != nil {
		// print the results to the terminal as an array of JSON objects
		if err = opts.exporter.Write(opts.Logger.IO, verified); err != nil {
			opts.Logger.Println(opts.Logger.ColorScheme.Red("✗ Failed to write JSON output"))
			return err
		}
		return nil
	}

	opts.Logger.Printf("The following %s matched the policy criteria\n\n", text.Pluralize(len(verified), "attestation"))

	// Otherwise print the results to the terminal
	for i, v := range verified {
		buildConfigURI := v.VerificationResult.Signature.Certificate.Extensions.BuildConfigURI
		sourceRepoAndOrg, sourceWorkflow, err := extractAttestationDetail(opts.Tenant, buildConfigURI)
		if err != nil {
			opts.Logger.Println(opts.Logger.ColorScheme.Red("failed to parse build config URI"))
			return err
		}
		builderSignerURI := v.VerificationResult.Signature.Certificate.Extensions.BuildSignerURI
		signerRepoAndOrg, signerWorkflow, err := extractAttestationDetail(opts.Tenant, builderSignerURI)
		if err != nil {
			opts.Logger.Println(opts.Logger.ColorScheme.Red("failed to parse build signer URI"))
			return err
		}

		opts.Logger.Printf("- Attestation #%d\n", i+1)
		rows := [][]string{
			{"  - Build repo", sourceRepoAndOrg},
			{"  - Build workflow", sourceWorkflow},
			{"  - Signer repo", signerRepoAndOrg},
			{"  - Signer workflow", signerWorkflow},
		}
		//nolint:errcheck
		opts.Logger.PrintBulletPoints(rows)
	}

	// All attestations passed verification and policy evaluation
	return nil
}

func extractAttestationDetail(tenant, builderSignerURI string) (string, string, error) {
	// If given a build signer URI like
	// https://github.com/foo/bar/.github/workflows/release.yml@refs/heads/main
	// We want to extract:
	// * foo/bar
	// * .github/workflows/release.yml@refs/heads/main
	var orgAndRepoRegexp *regexp.Regexp
	var workflowRegexp *regexp.Regexp

	if tenant == "" {
		orgAndRepoRegexp = regexp.MustCompile(`https://github\.com/([^/]+/[^/]+)/`)
		workflowRegexp = regexp.MustCompile(`https://github\.com/[^/]+/[^/]+/(.+)`)
	} else {
		var tr = regexp.QuoteMeta(tenant)
		orgAndRepoRegexp = regexp.MustCompile(fmt.Sprintf(
			`https://%s\.ghe\.com/([^/]+/[^/]+)/`,
			tr))
		workflowRegexp = regexp.MustCompile(fmt.Sprintf(
			`https://%s\.ghe\.com/[^/]+/[^/]+/(.+)`,
			tr))
	}

	match := orgAndRepoRegexp.FindStringSubmatch(builderSignerURI)
	if len(match) < 2 {
		return "", "", fmt.Errorf("no match found for org and repo: %s", builderSignerURI)
	}
	orgAndRepo := match[1]

	match = workflowRegexp.FindStringSubmatch(builderSignerURI)
	if len(match) < 2 {
		return "", "", fmt.Errorf("no match found for workflow: %s", builderSignerURI)
	}
	workflow := match[1]

	return orgAndRepo, workflow, nil
}
