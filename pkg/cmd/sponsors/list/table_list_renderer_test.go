package listcmd_test

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestListRendererDisplaysTableTTY(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	io.SetStdoutTTY(true)
	r := listcmd.TableListRenderer{
		IO: io,
	}

	err := r.Render(listcmd.Sponsors{{"sponsor1"}, {"sponsor2"}})
	require.NoError(t, err)

	expectedOutput := heredoc.Doc(`
	SPONSOR
	sponsor1
	sponsor2
	`)
	require.Equal(t, expectedOutput, stdout.String())
}

func TestListRendererDisplaysMessageWhenNoSponsorsTTY(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	io.SetStdoutTTY(true)
	r := listcmd.TableListRenderer{
		IO: io,
	}

	err := r.Render(listcmd.Sponsors{})
	require.NoError(t, err)

	expectedOutput := heredoc.Doc(`
	No sponsors found
	`)
	require.Equal(t, expectedOutput, stdout.String())
}

func TestListRendererDisplaysNothingWhenNoSponsorsNonTTY(t *testing.T) {
	io, _, stdout, _ := iostreams.Test()
	io.SetStdoutTTY(false)
	r := listcmd.TableListRenderer{
		IO: io,
	}

	err := r.Render(listcmd.Sponsors{})
	require.NoError(t, err)

	require.Equal(t, "", stdout.String())
}
