//go:build integration

package inspect

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func TestNewInspectCmd_PrintOutputJSONFormat(t *testing.T) {
	testIO, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: testIO,
		HttpClient: func() (*http.Client, error) {
			return http.DefaultClient, nil
		},
	}

	t.Run("Print output in JSON format", func(t *testing.T) {
		var opts *Options
		cmd := NewInspectCmd(f, func(o *Options) error {
			opts = o
			return nil
		})

		argv := strings.Split(fmt.Sprintf("%s --format json", bundlePath), " ")
		cmd.SetArgs(argv)
		cmd.SetIn(&bytes.Buffer{})
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		_, err := cmd.ExecuteC()
		assert.NoError(t, err)

		assert.Equal(t, bundlePath, opts.BundlePath)
		assert.NotNil(t, opts.Logger)
		assert.NotNil(t, opts.exporter)
	})
}
