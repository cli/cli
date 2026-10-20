package root

import (
	"testing"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/require"
)

func TestRootCommandBuilderNames(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser.Stub{},
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
	}
	builders := rootCommandBuilders(f, "v1.2.3", "")

	for _, builder := range builders {
		cmd := builder.build()
		require.Equal(t, builder.names[0], cmd.Name())
		require.Len(t, cmd.Aliases, len(builder.names)-1)
		for i, alias := range builder.names[1:] {
			require.Equal(t, alias, cmd.Aliases[i])
		}
		for _, name := range builder.names {
			route := rootCommandDispatch([]string{name}, builders, false)
			if name == "completion" {
				require.Empty(t, route.command)
			} else {
				require.Equal(t, builder.names[0], route.command)
			}
		}
	}

	require.Empty(t, rootCommandDispatch([]string{"pr"}, builders, true).command)
	require.Empty(t, rootCommandDispatch([]string{"unknown"}, builders, false).command)
	require.True(t, rootCommandDispatch([]string{"--version"}, builders, false).versionOnly)
	require.False(t, rootCommandDispatch([]string{"--version", "--version=false"}, builders, false).versionOnly)
	require.Empty(t, rootCommandDispatch([]string{"--version", "--version=false"}, builders, false).command)
	require.Empty(t, rootCommandDispatch([]string{"--version=true"}, builders, false).command)
}

func TestNewCmdRootBuildsSelectedCommandFamily(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser.Stub{},
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
	}

	cmd, err := NewCmdRoot(f, "v1.2.3", "", []string{"issue", "list"})
	require.NoError(t, err)
	require.Len(t, cmd.Commands(), 1)
	require.Equal(t, "issue", cmd.Commands()[0].Name())
}

func TestNewCmdRootBuildsFullTreeForCompletion(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser.Stub{},
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
	}

	cmd, err := NewCmdRoot(f, "v1.2.3", "", []string{"completion", "-s", "bash"})
	require.NoError(t, err)
	_, _, err = cmd.Find([]string{"workflow"})
	require.NoError(t, err)
}

func TestNewCmdRootBuildsFullTreeWithCobraDefaults(t *testing.T) {
	t.Setenv("GH_COBRA", "1")
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser.Stub{},
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
	}

	cmd, err := NewCmdRoot(f, "v1.2.3", "", []string{"pr", "view"})
	require.NoError(t, err)
	_, _, err = cmd.Find([]string{"workflow"})
	require.NoError(t, err)
}

func TestNewCmdRootSkipsCommandFamiliesForVersionFlag(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	f := &cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser.Stub{},
		Config:    func() (gh.Config, error) { return config.NewMockConfig(), nil },
	}

	cmd, err := NewCmdRoot(f, "v1.2.3", "", []string{"--version"})
	require.NoError(t, err)
	require.Empty(t, cmd.Commands())
}
