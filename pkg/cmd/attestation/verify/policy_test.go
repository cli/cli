package verify

import (
	"regexp"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
	"github.com/stretchr/testify/require"
)

func TestNewEnforcementCriteria(t *testing.T) {
	artifactPath := "../test/data/sigstore-js-2.1.0.tgz"

	t.Run("sets SANRegex and SAN using SANRegex and SAN", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:   artifactPath,
			Owner:          "foo",
			Repo:           "foo/bar",
			SAN:            "https://github/foo/bar/.github/workflows/attest.yml",
			SANRegex:       "(?i)^https://github/foo",
			SignerRepo:     "wrong/value",
			SignerWorkflow: "wrong/value/.github/workflows/attest.yml",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://github/foo/bar/.github/workflows/attest.yml", c.SAN)
		require.Equal(t, "(?i)^https://github/foo", c.SANRegex)
	})

	t.Run("sets SANRegex using SignerRepo", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:   artifactPath,
			Owner:          "wrong",
			Repo:           "wrong/value",
			SignerRepo:     "foo/bar",
			SignerWorkflow: "wrong/value/.github/workflows/attest.yml",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `(?i)^https://github\.com/foo/bar/`, c.SANRegex)
		require.Zero(t, c.SAN)
	})

	t.Run("sets SANRegex using SignerRepo and Tenant", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:   artifactPath,
			Owner:          "wrong",
			Repo:           "wrong/value",
			SignerRepo:     "foo/bar",
			SignerWorkflow: "wrong/value/.github/workflows/attest.yml",
			Tenant:         "baz",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `(?i)^https://baz\.ghe\.com/foo/bar/`, c.SANRegex)
		require.Zero(t, c.SAN)
	})

	t.Run("sets SANRegex using SignerWorkflow matching host regex", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:   artifactPath,
			Owner:          "wrong",
			Repo:           "wrong/value",
			SignerWorkflow: "foo/bar/.github/workflows/attest.yml",
			Hostname:       "github.com",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `^https://github\.com/foo/bar/\.github/workflows/attest\.yml(@(refs/.*|[0-9a-fA-F]+))?$`, c.SANRegex)
		require.Zero(t, c.SAN)
	})

	t.Run("sets SANRegex using opts.Repo", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "wrong",
			Repo:         "foo/bar",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `(?i)^https://github\.com/foo/bar/`, c.SANRegex)
	})

	t.Run("sets SANRegex using opts.Owner", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `(?i)^https://github\.com/foo/`, c.SANRegex)
	})

	t.Run("SANRegex escapes regex metacharacters in repo names", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			SignerRepo:   "my.org/my.repo",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, `(?i)^https://github\.com/my\.org/my\.repo/`, c.SANRegex)

		// Verify the generated regex does NOT match a lookalike repo
		re := regexp.MustCompile(c.SANRegex)
		require.True(t, re.MatchString("https://github.com/my.org/my.repo/.github/workflows/build.yml"))
		require.False(t, re.MatchString("https://github.com/myXorg/myXrepo/.github/workflows/build.yml"))
	})

	t.Run("sets Extensions.RunnerEnvironment to GitHubRunner value if opts.DenySelfHostedRunner is true", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:         artifactPath,
			Owner:                "foo",
			Repo:                 "foo/bar",
			DenySelfHostedRunner: true,
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, verification.GitHubRunner, c.Certificate.RunnerEnvironment)
	})

	t.Run("sets Extensions.RunnerEnvironment to * value if opts.DenySelfHostedRunner is false", func(t *testing.T) {
		opts := &Options{
			ArtifactPath:         artifactPath,
			Owner:                "foo",
			Repo:                 "foo/bar",
			DenySelfHostedRunner: false,
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Zero(t, c.Certificate.RunnerEnvironment)
	})

	t.Run("sets Extensions.SourceRepositoryURI using opts.Repo and opts.Tenant", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
			Tenant:       "baz",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://baz.ghe.com/foo/bar", c.Certificate.SourceRepositoryURI)
	})

	t.Run("sets Extensions.SourceRepositoryURI using opts.Repo", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://github.com/foo/bar", c.Certificate.SourceRepositoryURI)
	})

	t.Run("sets SANRegex and SAN using SANRegex and SAN, sets Extensions.SourceRepositoryURI using opts.Repo", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "baz",
			Repo:         "baz/xyz",
			SAN:          "https://github/foo/bar/.github/workflows/attest.yml",
			SANRegex:     "(?i)^https://github/foo",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://github/foo/bar/.github/workflows/attest.yml", c.SAN)
		require.Equal(t, "(?i)^https://github/foo", c.SANRegex)
		require.Equal(t, "https://github.com/baz/xyz", c.Certificate.SourceRepositoryURI)
	})

	t.Run("sets Extensions.SourceRepositoryOwnerURI using opts.Owner and opts.Tenant", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
			Tenant:       "baz",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://baz.ghe.com/foo", c.Certificate.SourceRepositoryOwnerURI)
	})

	t.Run("sets Extensions.SourceRepositoryOwnerURI using opts.Owner", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://github.com/foo", c.Certificate.SourceRepositoryOwnerURI)
	})

	t.Run("sets OIDCIssuer using opts.Tenant", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
			Tenant:       "baz",
			OIDCIssuer:   verification.GitHubOIDCIssuer,
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://token.actions.baz.ghe.com", c.Certificate.Issuer)
	})

	t.Run("sets OIDCIssuer using opts.OIDCIssuer", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
			Repo:         "foo/bar",
			OIDCIssuer:   "https://foo.com",
			Tenant:       "baz",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "https://foo.com", c.Certificate.Issuer)
	})

	t.Run("sets Certificate.BuildSignerDigest using opts.SignerDigest", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "wrong",
			Repo:         "wrong/value",
			SignerDigest: "foo",
			Hostname:     "github.com",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "foo", c.Certificate.BuildSignerDigest)
	})

	t.Run("sets Certificate.SourceRepositoryDigest using opts.SourceDigest", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "wrong",
			Repo:         "wrong/value",
			SourceDigest: "foo",
			Hostname:     "github.com",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "foo", c.Certificate.SourceRepositoryDigest)
	})

	t.Run("sets Certificate.SourceRepositoryRef using opts.SourceRef", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "wrong",
			Repo:         "wrong/value",
			SourceRef:    "refs/heads/main",
			Hostname:     "github.com",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "refs/heads/main", c.Certificate.SourceRepositoryRef)
	})
}

func TestValidateSignerWorkflow(t *testing.T) {
	type testcase struct {
		name                   string
		providedSignerWorkflow string
		expectedWorkflowRegex  string
		host                   string
		expectErr              bool
		errContains            string
	}

	testcases := []testcase{
		{
			name:                   "workflow with no host specified",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectErr:              true,
			errContains:            "unknown signer workflow host",
		},
		{
			name:                   "workflow with default host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  `^https://github\.com/github/artifact-attestations-workflows/\.github/workflows/attest\.yml(@(refs/.*|[0-9a-fA-F]+))?$`,
			host:                   "github.com",
		},
		{
			name:                   "workflow with workflow URL included",
			providedSignerWorkflow: "github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  `^https://github\.com/github/artifact-attestations-workflows/\.github/workflows/attest\.yml(@(refs/.*|[0-9a-fA-F]+))?$`,
			host:                   "github.com",
		},
		{
			name:                   "workflow with GH_HOST set",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  `^https://myhost\.github\.com/github/artifact-attestations-workflows/\.github/workflows/attest\.yml(@(refs/.*|[0-9a-fA-F]+))?$`,
			host:                   "myhost.github.com",
		},
		{
			name:                   "workflow with authenticated host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  `^https://authedhost\.github\.com/github/artifact-attestations-workflows/\.github/workflows/attest\.yml(@(refs/.*|[0-9a-fA-F]+))?$`,
			host:                   "authedhost.github.com",
		},
	}

	for _, tc := range testcases {
		// All host resolution is done verify.go:RunE
		workflowRegex, err := validateSignerWorkflow(tc.host, tc.providedSignerWorkflow)
		require.Equal(t, tc.expectedWorkflowRegex, workflowRegex)

		if tc.expectErr {
			require.Error(t, err)
			require.ErrorContains(t, err, tc.errContains)
		} else {
			require.NoError(t, err)
			require.Equal(t, tc.expectedWorkflowRegex, workflowRegex)
		}
	}
}

// TestSignerWorkflowSANMatching drives the pattern built by validateSignerWorkflow
// through the sigstore-go matcher that consumes it, because that matcher applies the
// pattern with regexp.MatchString and so does not anchor it on the caller's behalf.
func TestSignerWorkflowSANMatching(t *testing.T) {
	const pinnedWorkflow = "owner/builder/.github/workflows/release.yml"
	const workflowDir = "https://github.com/owner/builder/.github/workflows/"

	testcases := []struct {
		name        string
		san         string
		expectMatch bool
	}{
		{
			name:        "pinned workflow at a branch ref",
			san:         workflowDir + "release.yml@refs/heads/main",
			expectMatch: true,
		},
		{
			name:        "pinned workflow at a commit SHA",
			san:         workflowDir + "release.yml@09b495c3f12c7881b3cc17209a327792065c1a1d",
			expectMatch: true,
		},
		{
			name:        "pinned workflow at a tag containing @",
			san:         workflowDir + "release.yml@refs/tags/pkg@1.2.3",
			expectMatch: true,
		},
		{
			name:        "pinned workflow at a branch containing @",
			san:         workflowDir + "release.yml@refs/heads/feature@v2",
			expectMatch: true,
		},
		{
			name:        "pinned workflow with no ref",
			san:         workflowDir + "release.yml",
			expectMatch: true,
		},
		{
			name:        "workflow whose file name extends the pinned name",
			san:         workflowDir + "release.yml.attacker.yml@refs/heads/attacker",
			expectMatch: false,
		},
		{
			name:        "workflow whose file name contains the ref separator",
			san:         workflowDir + "release.yml@attacker.yml@refs/heads/attacker",
			expectMatch: false,
		},
		{
			name:        "different workflow in the pinned repository",
			san:         workflowDir + "other.yml@refs/heads/main",
			expectMatch: false,
		},
		{
			name:        "pinned workflow in a lookalike repository",
			san:         "https://github.com/owner/builder-attacker/.github/workflows/release.yml@refs/heads/main",
			expectMatch: false,
		},
	}

	sanRegex, err := validateSignerWorkflow("github.com", pinnedWorkflow)
	require.NoError(t, err)

	matcher, err := verify.NewSANMatcher("", sanRegex)
	require.NoError(t, err)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := matcher.Verify(certificate.Summary{SubjectAlternativeName: tc.san})
			if tc.expectMatch {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestSignerWorkflowSANMatchingObservedCertificate uses the SubjectAlternativeName from
// a certificate Fulcio actually issued for a workflow stored at
// .github/workflows/provenance-image.yml@evil.yml. Actions loads a workflow file whose
// name contains "@", and neither the OIDC claim nor Fulcio escapes it, so the separator
// really can appear more than once in a SAN.
func TestSignerWorkflowSANMatchingObservedCertificate(t *testing.T) {
	const observedSAN = "https://github.com/bdehamer/attest-demo/.github/workflows/provenance-image.yml@evil.yml@refs/heads/main"

	sanRegex, err := validateSignerWorkflow("github.com", "bdehamer/attest-demo/.github/workflows/provenance-image.yml")
	require.NoError(t, err)

	matcher, err := verify.NewSANMatcher("", sanRegex)
	require.NoError(t, err)

	require.Error(t, matcher.Verify(certificate.Summary{SubjectAlternativeName: observedSAN}))
}

// TestSignerWorkflowSANMatchingPinnedRef covers a --signer-workflow value that pins a
// ref as well as a workflow. Such a value has to match the end of the SAN exactly,
// because a git ref may itself be named so that it begins with the pinned ref and is
// followed by something ref-shaped.
func TestSignerWorkflowSANMatchingPinnedRef(t *testing.T) {
	const pinnedWorkflow = "owner/builder/.github/workflows/release.yml@refs/heads/main"
	const workflowURL = "https://github.com/owner/builder/.github/workflows/release.yml"

	testcases := []struct {
		name        string
		san         string
		expectMatch bool
	}{
		{
			name:        "the pinned ref",
			san:         workflowURL + "@refs/heads/main",
			expectMatch: true,
		},
		{
			name:        "a branch whose name extends the pinned ref",
			san:         workflowURL + "@refs/heads/mainattacker",
			expectMatch: false,
		},
		{
			name:        "a branch named so that a second ref follows the pinned ref",
			san:         workflowURL + "@refs/heads/main@refs/heads/attacker",
			expectMatch: false,
		},
		{
			name:        "a branch named so that an object ID follows the pinned ref",
			san:         workflowURL + "@refs/heads/main@09b495c3f12c7881b3cc17209a327792065c1a1d",
			expectMatch: false,
		},
		{
			name:        "a different ref",
			san:         workflowURL + "@refs/heads/attacker",
			expectMatch: false,
		},
	}

	sanRegex, err := validateSignerWorkflow("github.com", pinnedWorkflow)
	require.NoError(t, err)

	matcher, err := verify.NewSANMatcher("", sanRegex)
	require.NoError(t, err)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			err := matcher.Verify(certificate.Summary{SubjectAlternativeName: tc.san})
			if tc.expectMatch {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// TestSignerWorkflowSANMatchingObservedNestedRef uses the job_workflow_ref claim
// observed from a workflow run on a branch named "main@refs/heads/evil". Actions runs a
// workflow from such a branch, so a pin on refs/heads/main must not admit it.
func TestSignerWorkflowSANMatchingObservedNestedRef(t *testing.T) {
	const observedSAN = "https://github.com/bdehamer/attest-demo/.github/workflows/control-probe.yml@refs/heads/main@refs/heads/evil"

	sanRegex, err := validateSignerWorkflow("github.com", "bdehamer/attest-demo/.github/workflows/control-probe.yml@refs/heads/main")
	require.NoError(t, err)

	matcher, err := verify.NewSANMatcher("", sanRegex)
	require.NoError(t, err)

	require.Error(t, matcher.Verify(certificate.Summary{SubjectAlternativeName: observedSAN}))
}

// TestSignerWorkflowSANMatchingRepositoryValue covers a --signer-workflow value that
// names a repository instead of a workflow. The value is still matched as a complete
// identity, so it cannot match a repository whose name merely starts with it.
func TestSignerWorkflowSANMatchingRepositoryValue(t *testing.T) {
	sanRegex, err := validateSignerWorkflow("github.com", "owner/builder")
	require.NoError(t, err)

	matcher, err := verify.NewSANMatcher("", sanRegex)
	require.NoError(t, err)

	san := "https://github.com/owner/builder-attacker/.github/workflows/release.yml@refs/heads/main"
	require.Error(t, matcher.Verify(certificate.Summary{SubjectAlternativeName: san}))
}
