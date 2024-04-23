package tufrootverify

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/attestation/test"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/sigstore/sigstore-go/pkg/tuf"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTUFRootVerifyCmd(t *testing.T) {
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
			name:     "Missing mirror flag",
			cli:      "--root ../verification/embed/tuf-repo.github.com/root.json",
			wantsErr: true,
		},
		{
			name:     "Missing root flag",
			cli:      "--mirror https://tuf-repo.github.com",
			wantsErr: true,
		},
		{
			name:     "Has all required flags",
			cli:      "--mirror https://tuf-repo.github.com --root ../verification/embed/tuf-repo.github.com/root.json",
			wantsErr: false,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewTUFRootVerifyCmd(f, func() error {
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

var newTUFMockClient tufClientInstantiator = func(o *tuf.Options) (*tuf.Client, error) {
	return &tuf.Client{}, nil
}

var newTUFErrClient tufClientInstantiator = func(o *tuf.Options) (*tuf.Client, error) {
	return nil, fmt.Errorf("failed to create TUF client")
}

func TestTUFRootVerify(t *testing.T) {
	mirror := "https://tuf-repo.github.com"
	root := test.NormalizeRelativePath("../verification/embed/tuf-repo.github.com/root.json")

	t.Run("successfully verifies TUF root", func(t *testing.T) {
		err := tufRootVerify(newTUFMockClient, mirror, root)
		require.NoError(t, err)
	})

	t.Run("fails because the root cannot be found", func(t *testing.T) {
		notFoundRoot := test.NormalizeRelativePath("./does/not/exist/root.json")
		err := tufRootVerify(newTUFMockClient, mirror, notFoundRoot)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to read root file")
	})

	t.Run("failed to create TUF root", func(t *testing.T) {
		err := tufRootVerify(newTUFErrClient, mirror, root)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to create TUF client")
	})
}
