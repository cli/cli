package list

import (
	"sort"
	"testing"
	"time"

	"github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
	"github.com/stretchr/testify/assert"
)

func TestVulnerabilityAlert_byCreatedAt(t *testing.T) {
	today := time.Now()
	yesterday := today.AddDate(0, 0, -1)
	lastWeek := today.AddDate(0, 0, -7)

	tests := []dependabot.VulnerabilityAlert{
		{
			CreatedAt: today,
		},
		{
			CreatedAt: yesterday,
		},
		{
			CreatedAt: lastWeek,
		},
	}
	byCreatedAt := byCreatedAt(tests)
	sort.Sort(byCreatedAt)
	assert.Equal(t, tests[0].CreatedAt, lastWeek)
	assert.Equal(t, tests[1].CreatedAt, yesterday)
	assert.Equal(t, tests[2].CreatedAt, today)
}

func TestVulnerabilityAlert_bySeverity(t *testing.T) {
	tests := []dependabot.VulnerabilityAlert{
		{
			SecurityVulnerability: dependabot.SecurityVulnerability{
				Severity: dependabot.LowSeverity,
			},
		},

		{
			SecurityVulnerability: dependabot.SecurityVulnerability{
				Severity: dependabot.HighSeverity,
			},
		},
		{
			SecurityVulnerability: dependabot.SecurityVulnerability{
				Severity: dependabot.CriticalSeverity,
			},
		},
		{
			SecurityVulnerability: shared.SecurityVulnerability{
				Severity: shared.ModerateSeverity,
			},
		},
	}
	bySeverity := bySeverity(tests)
	sort.Sort(sort.Reverse(bySeverity))
	assert.Equal(t, tests[0].SecurityVulnerability.Severity, dependabot.CriticalSeverity)
	assert.Equal(t, tests[1].SecurityVulnerability.Severity, dependabot.HighSeverity)
	assert.Equal(t, tests[2].SecurityVulnerability.Severity, dependabot.ModerateSeverity)
	assert.Equal(t, tests[3].SecurityVulnerability.Severity, dependabot.LowSeverity)
}

func TestVulnerabilityAlert_byNumber(t *testing.T) {
	tests := []dependabot.VulnerabilityAlert{
		{
			Number: 2,
		},
		{
			Number: 1,
		},
		{
			Number: 3,
		},
	}
	byNumber := byNumber(tests)
	sort.Sort(byNumber)
	assert.Equal(t, tests[0].Number, 1)
	assert.Equal(t, tests[1].Number, 2)
	assert.Equal(t, tests[2].Number, 3)
}
