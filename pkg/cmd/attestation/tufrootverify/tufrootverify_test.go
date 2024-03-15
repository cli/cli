package tufrootverify

import (
	"bytes"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"

	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
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
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewTUFRootVerifyCmd(f, func() error {
				return nil
			})

			argv, err := shlex.Split(tc.cli)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			if tc.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
