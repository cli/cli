package verify

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/verification"
	"github.com/cli/cli/v2/pkg/cmd/factory"

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
		require.Equal(t, "(?i)^https://github.com/foo/bar/", c.SANRegex)
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
		require.Equal(t, "(?i)^https://baz.ghe.com/foo/bar/", c.SANRegex)
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
		require.Equal(t, "^https://github.com/foo/bar/.github/workflows/attest.yml", c.SANRegex)
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
		require.Equal(t, "(?i)^https://github.com/foo/bar/", c.SANRegex)
	})

	t.Run("sets SANRegex using opts.Owner", func(t *testing.T) {
		opts := &Options{
			ArtifactPath: artifactPath,
			Owner:        "foo",
		}

		c, err := newEnforcementCriteria(opts)
		require.NoError(t, err)
		require.Equal(t, "(?i)^https://github.com/foo/", c.SANRegex)
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
			errContains:            "unknown host",
		},
		{
			name:                   "workflow with default host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "github.com",
		},
		{
			name:                   "workflow with workflow URL included",
			providedSignerWorkflow: "github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "github.com",
		},
		{
			name:                   "workflow with GH_HOST set",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://myhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "myhost.github.com",
		},
		{
			name:                   "workflow with authenticated host",
			providedSignerWorkflow: "github/artifact-attestations-workflows/.github/workflows/attest.yml",
			expectedWorkflowRegex:  "^https://authedhost.github.com/github/artifact-attestations-workflows/.github/workflows/attest.yml",
			host:                   "authedhost.github.com",
		},
	}

	for _, tc := range testcases {
		opts := &Options{
			Config:         factory.New("test").Config,
			SignerWorkflow: tc.providedSignerWorkflow,
		}

		// All host resolution is done verify.go:RunE
		opts.Hostname = tc.host
		workflowRegex, err := validateSignerWorkflow(opts)
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
