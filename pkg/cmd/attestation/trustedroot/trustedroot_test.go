package trustedroot

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cli/cli/v2/pkg/cmd/attestation/io"
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
			name:     "Public happy path",
			cli:      "--public",
			wantsErr: false,
		},
		{
			name:     "Private happy path",
			cli:      "--private",
			wantsErr: false,
		},
		{
			name:     "Custom TUF happy path",
			cli:      "--tuf-url https://tuf-repo.github.com --tuf-root ../verification/embed/tuf-repo.github.com/root.json",
			wantsErr: false,
		},
		{
			name:     "Both public and private error",
			cli:      "--public --private",
			wantsErr: true,
		},
		{
			name:     "Missing tuf-root flag",
			cli:      "--tuf-url https://tuf-repo.github.com",
			wantsErr: true,
		},
		{
			name:     "Error missing basic flags with advanced setup",
			cli:      "--public --tuf-url https://tuf-repo.github.com --tuf-root ../verification/embed/tuf-repo.github.com/root.json",
			wantsErr: true,
		},
		{
			name:     "Error on no flags",
			cli:      "",
			wantsErr: true,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewTrustedRootCmd(f, func(_ *Options) error {
				return nil
			})

			argv := strings.Split(tc.cli, " ")
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
		Logger:      io.NewTestHandler(),
	}

	t.Run("successfully verifies TUF root", func(t *testing.T) {
		err := getTrustedRoot(tuf.New, opts)
		require.NoError(t, err)
	})

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
