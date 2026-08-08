package skills_test

import (
	"testing"

	"github.com/cli/cli/v2/internal/gh/ghtelemetry"
	"github.com/cli/cli/v2/internal/telemetry"
	"github.com/cli/cli/v2/pkg/cmd/skills"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/stretchr/testify/require"
)

func TestSkillCommandsAreSampledAt100(t *testing.T) {
	spy := &telemetry.CommandRecorderSpy{}
	factory := &cmdutil.Factory{}
	cmd := skills.NewCmdSkills(factory, spy)
	cmd.PersistentPreRunE(nil, []string{})
	require.Equal(t, ghtelemetry.SAMPLE_ALL, spy.LastSampleRate)
}
