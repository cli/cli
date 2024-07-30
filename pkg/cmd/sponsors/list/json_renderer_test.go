package listcmd_test

import (
	"testing"

	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestRenderingDisplaysTableTTY(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	io.SetStdoutTTY(true)
	r := listcmd.JSONListRenderer{
		IO:       io,
		Exporter: cmdutil.NewJSONExporter(),
	}

	err := r.Render(listcmd.Sponsors{
		{"sponsor1"},
		{"sponsor2"},
	})
	require.NoError(t, err)

	require.JSONEq(t,
		`[{"login":"sponsor1"},{"login":"sponsor2"}]`,
		stdout.String())
}
