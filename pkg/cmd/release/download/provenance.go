package download

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/text"
	attapi "github.com/cli/cli/v2/pkg/cmd/attestation/api"
	"github.com/cli/cli/v2/pkg/cmd/attestation/artifact"
	"github.com/cli/cli/v2/pkg/cmd/attestation/auth"
	att_io "github.com/cli/cli/v2/pkg/cmd/attestation/io"
	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/release/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

var errNoProvenance = errors.New("no build provenance attestation found")

// workflowRefSuffix matches the "@<ref>" that terminates a workflow URI in a
// Fulcio certificate, e.g. ".../release.yml@refs/heads/main".
var workflowRefSuffix = regexp.MustCompile(`@(refs/.*|[0-9a-fA-F]+)$`)

// parseReleaseAssetURL parses a URL of the form
// https://HOST/OWNER/REPO/releases/download/TAG/ASSET
func parseReleaseAssetURL(s string) (ghrepo.Interface, string, string, error) {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return nil, "", "", fmt.Errorf("invalid release asset URL %q", s)
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 6 || segments[2] != "releases" || segments[3] != "download" {
		return nil, "", "", fmt.Errorf("expected a release asset URL like https://github.com/OWNER/REPO/releases/download/TAG/ASSET, got %q", s)
	}
	tag := strings.Join(segments[4:len(segments)-1], "/")
	asset := segments[len(segments)-1]
	if segments[0] == "" || segments[1] == "" || tag == "" || asset == "" {
		return nil, "", "", fmt.Errorf("invalid release asset URL %q", s)
	}
	return ghrepo.NewWithHost(segments[0], segments[1], u.Hostname()), tag, asset, nil
}

func isURLArg(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

type provenanceResult struct {
	asset    shared.ReleaseAsset
	digest   string // hex-encoded sha256
	verified *verification.AttestationProcessingResult
	err      error
}

type provenanceChecker struct {
	attClient attapi.Client
	verifier  verification.SigstoreVerifier
	repo      ghrepo.Interface
	tenant    string
}

func newProvenanceChecker(opts *DownloadOptions, httpClient *http.Client, repo ghrepo.Interface) (*provenanceChecker, error) {
	host := repo.RepoHost()
	if err := auth.IsHostSupported(host); err != nil {
		return nil, fmt.Errorf("`--verify` is not supported for %s: %w", host, err)
	}

	tenant, isTenant := ghinstance.TenantName(host)
	if !isTenant {
		tenant = ""
	}
	c := &provenanceChecker{
		attClient: opts.AttClient,
		verifier:  opts.SigstoreVerifier,
		repo:      repo,
		tenant:    tenant,
	}
	if c.attClient != nil && c.verifier != nil {
		return c, nil
	}

	externalClient, err := opts.ExternalHttpClient()
	if err != nil {
		return nil, err
	}
	logger := att_io.NewHandler(opts.IO)
	if c.attClient == nil {
		c.attClient = attapi.NewLiveClient(httpClient, externalClient, host, logger)
	}
	if c.verifier == nil {
		config := verification.SigstoreConfig{
			ExternalHttpClient: externalClient,
			Logger:             logger,
		}
		if tenant != "" {
			td, err := c.attClient.GetTrustDomain()
			if err != nil {
				return nil, fmt.Errorf("error getting trust domain: %w", err)
			}
			config.TrustDomain = td
		}
		c.verifier, err = verification.NewLiveSigstoreVerifier(config)
		if err != nil {
			return nil, fmt.Errorf("error creating Sigstore verifier: %w", err)
		}
	}
	return c, nil
}

func (c *provenanceChecker) hostURL() string {
	if c.tenant == "" {
		return "https://github.com"
	}
	return fmt.Sprintf("https://%s.ghe.com", c.tenant)
}

// check verifies the SLSA build provenance for an asset using the digest
// reported by the API, so it can run before the asset is downloaded. The
// downloaded bytes are compared against the same digest afterwards.
func (c *provenanceChecker) check(asset shared.ReleaseAsset) provenanceResult {
	res := provenanceResult{asset: asset}

	if asset.Digest == nil || !strings.HasPrefix(*asset.Digest, "sha256:") {
		res.err = errors.New("GitHub did not report a sha256 digest for this asset")
		return res
	}
	res.digest = strings.TrimPrefix(*asset.Digest, "sha256:")
	art := artifact.NewDigestedArtifactForRelease(res.digest, "sha256")

	attestations, err := c.attClient.GetByDigest(attapi.FetchParams{
		Digest:        art.DigestWithAlg(),
		Repo:          ghrepo.FullName(c.repo),
		PredicateType: verification.SLSAPredicateV1,
		Limit:         attapi.DefaultLimit,
	})
	if err != nil {
		var httpErr api.HTTPError
		if errors.Is(err, attapi.ErrNoAttestationsFound) || (errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound) {
			res.err = errNoProvenance
		} else {
			res.err = fmt.Errorf("failed to fetch attestations: %w", err)
		}
		return res
	}

	hostURL := c.hostURL()
	issuer := verification.GitHubOIDCIssuer
	if c.tenant != "" {
		issuer = fmt.Sprintf(verification.GitHubTenantOIDCIssuer, c.tenant)
	}
	ec := verification.EnforcementCriteria{
		PredicateType: verification.SLSAPredicateV1,
		// Any GitHub Actions workflow may sign, which allows reusable workflows
		// in other repositories. The source repository is pinned below via the
		// certificate extensions, which come from the OIDC token.
		SANRegex: "^" + regexp.QuoteMeta(hostURL+"/"),
		Certificate: certificate.Summary{
			Issuer:                   issuer,
			SourceRepositoryOwnerURI: fmt.Sprintf("%s/%s", hostURL, c.repo.RepoOwner()),
			SourceRepositoryURI:      fmt.Sprintf("%s/%s", hostURL, ghrepo.FullName(c.repo)),
		},
	}

	policy, err := buildPolicy(ec, *art)
	if err != nil {
		res.err = err
		return res
	}

	verified, err := c.verifier.Verify(attestations, policy)
	if err != nil {
		res.err = fmt.Errorf("attestation signature verification failed: %w", err)
		return res
	}
	verified, err = verification.VerifyCertExtensions(verified, ec)
	if err != nil {
		res.err = fmt.Errorf("attestation was not built from %s: %w", ghrepo.FullName(c.repo), err)
		return res
	}
	for _, v := range verified {
		if v.VerificationResult.Statement.GetPredicateType() == verification.SLSAPredicateV1 {
			res.verified = v
			return res
		}
	}
	res.err = errNoProvenance
	return res
}

func buildPolicy(ec verification.EnforcementCriteria, art artifact.DigestedArtifact) (verify.PolicyBuilder, error) {
	digestOption, err := verification.BuildDigestPolicyOption(art)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}
	sanMatcher, err := verify.NewSANMatcher(ec.SAN, ec.SANRegex)
	if err != nil {
		return verify.PolicyBuilder{}, err
	}
	// The issuer is enforced by VerifyCertExtensions.
	issuerMatcher, err := verify.NewIssuerMatcher("", ".*")
	if err != nil {
		return verify.PolicyBuilder{}, err
	}
	certID, err := verify.NewCertificateIdentity(sanMatcher, issuerMatcher, certificate.Extensions{})
	if err != nil {
		return verify.PolicyBuilder{}, err
	}
	return verify.NewPolicy(digestOption, verify.WithCertificateIdentity(certID)), nil
}

// confirmProvenance prints the immutability and provenance details for the
// release and each asset, and decides whether the download should proceed.
// It returns the verified sha256 digest for each asset, keyed by name.
func confirmProvenance(opts *DownloadOptions, httpClient *http.Client, repo ghrepo.Interface, release *shared.Release, assets []shared.ReleaseAsset) (map[string]string, error) {
	checker, err := newProvenanceChecker(opts, httpClient, repo)
	if err != nil {
		return nil, err
	}

	results := make([]provenanceResult, len(assets))
	for i, a := range assets {
		opts.IO.StartProgressIndicatorWithLabel(fmt.Sprintf("Verifying provenance of %s", a.Name))
		results[i] = checker.check(a)
	}
	opts.IO.StopProgressIndicator()

	out := opts.IO.ErrOut
	cs := opts.IO.ColorScheme()

	fmt.Fprintf(out, "%s %s\n", cs.Bold(ghrepo.FullName(repo)), cs.Bold(release.TagName))
	if release.IsImmutable {
		fmt.Fprintf(out, "%s Immutable release\n", cs.SuccessIcon())
	} else {
		fmt.Fprintf(out, "%s Not an immutable release; assets can be replaced after publishing\n", cs.WarningIcon())
	}

	digests := make(map[string]string, len(results))
	var unverified int
	for _, r := range results {
		fmt.Fprintf(out, "\n%s\n", cs.Bold(r.asset.Name))
		if r.err != nil {
			unverified++
			fmt.Fprintf(out, "%s %s\n", cs.FailureIcon(), capitalize(r.err.Error()))
			continue
		}
		digests[r.asset.Name] = r.digest
		fmt.Fprintf(out, "%s Verified SLSA build provenance\n", cs.SuccessIcon())
		printProvenanceDetails(opts, r)
	}
	fmt.Fprintln(out)

	if !opts.IO.CanPrompt() {
		if unverified > 0 {
			return nil, fmt.Errorf("refusing to download: %s lacking verified build provenance; run without `--verify` to download anyway",
				text.Pluralize(unverified, "asset"))
		}
		return digests, nil
	}

	question := fmt.Sprintf("Download %s?", text.Pluralize(len(assets), "asset"))
	if len(assets) == 1 {
		question = fmt.Sprintf("Download %s?", assets[0].Name)
	}
	ok, err := opts.Prompter.Confirm(question, unverified == 0)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, cmdutil.CancelError
	}
	return digests, nil
}

func printProvenanceDetails(opts *DownloadOptions, r provenanceResult) {
	cs := opts.IO.ColorScheme()
	cert := r.verified.VerificationResult.Signature.Certificate
	sourceRepo := cert.SourceRepositoryURI

	rows := [][2]string{
		{"Build instructions", workflowURL(sourceRepo, cert.BuildConfigURI, cert.BuildConfigDigest)},
	}
	if cert.BuildSignerURI != "" && cert.BuildSignerURI != cert.BuildConfigURI {
		rows = append(rows, [2]string{"Signer workflow", cert.BuildSignerURI})
	}
	source := sourceRepo
	if cert.SourceRepositoryDigest != "" {
		source = fmt.Sprintf("%s/tree/%s", sourceRepo, cert.SourceRepositoryDigest)
	}
	rows = append(rows,
		[2]string{"Source", source},
		[2]string{"Source ref", cert.SourceRepositoryRef},
	)
	if cert.RunInvocationURI != "" {
		rows = append(rows, [2]string{"Build run", cert.RunInvocationURI})
	}
	if cert.RunnerEnvironment != "" {
		runner := cert.RunnerEnvironment
		if runner != verification.GitHubRunner {
			runner = cs.Yellow(runner)
		}
		rows = append(rows, [2]string{"Runner", runner})
	}

	width := 0
	for _, row := range rows {
		width = max(width, len(row[0]))
	}
	for _, row := range rows {
		if row[1] == "" {
			continue
		}
		fmt.Fprintf(opts.IO.ErrOut, "  - %-*s %s\n", width+1, row[0]+":", row[1])
	}
}

// workflowURL turns a certificate workflow URI such as
// https://github.com/OWNER/REPO/.github/workflows/release.yml@refs/heads/main
// into a browsable link pinned to the commit that ran it.
func workflowURL(sourceRepo, configURI, configDigest string) string {
	path, ok := strings.CutPrefix(configURI, sourceRepo+"/")
	if !ok || configDigest == "" {
		return configURI
	}
	path = workflowRefSuffix.ReplaceAllString(path, "")
	return fmt.Sprintf("%s/blob/%s/%s", sourceRepo, configDigest, path)
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
