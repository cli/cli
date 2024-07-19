package trustedroot

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
)

func TestNewTrustedRootCmd(t *testing.T) {
	testIO, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: testIO,
	}

	testcases := []struct {
		name     string
		cli      string
		wantsErr bool
	}{
		{
			name:     "Happy path",
			cli:      "",
			wantsErr: false,
		},
		{
			name:     "Happy path",
			cli:      "--verify-only",
			wantsErr: false,
		},
		{
			name:     "Custom TUF happy path",
			cli:      "--tuf-url https://tuf-repo.github.com --tuf-root ../verification/embed/tuf-repo.github.com/root.json",
			wantsErr: false,
		},
		{
			name:     "Missing tuf-root flag",
			cli:      "--tuf-url https://tuf-repo.github.com",
			wantsErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewTrustedRootCmd(f, func(_ *Options) error {
				return nil
			})

			argv := []string{}
			if tc.cli != "" {
				argv = strings.Split(tc.cli, " ")
			}
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err := cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}

var newTUFErrClient tufClientInstantiator = func(o *tuf.Options) (*tuf.Client, error) {
	return nil, fmt.Errorf("failed to create TUF client")
}

func TestGetTrustedRoot(t *testing.T) {
	mirror := "https://tuf-repo.github.com"
	root := test.NormalizeRelativePath("../verification/embed/tuf-repo.github.com/root.json")

	opts := &Options{
		TufUrl:      mirror,
		TufRootPath: root,
	}

	t.Run("failed to create TUF root", func(t *testing.T) {
		err := getTrustedRoot(newTUFErrClient, opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to create TUF client")
	})

	t.Run("fails because the root cannot be found", func(t *testing.T) {
		opts.TufRootPath = test.NormalizeRelativePath("./does/not/exist/root.json")
		err := getTrustedRoot(tuf.New, opts)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to read root file")
	})

}
