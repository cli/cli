package contributions

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testHostConfig string

func (c testHostConfig) DefaultHost() (string, string) { return string(c), "" }

func TestNewCmdContributions(t *testing.T) {
	tests := []struct {
		name    string
		cli     string
		wants   ContributionsOptions
		wantErr string
	}{
		{name: "defaults"},
		{name: "user", cli: "-u octocat", wants: ContributionsOptions{User: "octocat"}},
		{name: "from/to", cli: "--from 2024-01-01 --to 2024-06-30", wants: ContributionsOptions{From: "2024-01-01", To: "2024-06-30"}},
		{name: "rejects positional arg", cli: "octocat", wantErr: `unknown command "octocat"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
				Config: func() (gh.Config, error) {
					return config.NewBlankConfig(), nil
				},
			}

			argv, err := shlex.Split(tt.cli)
			require.NoError(t, err)

			var got *ContributionsOptions
			cmd := NewCmdContributions(f, func(opts *ContributionsOptions) error {
				got = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wants.User, got.User)
			assert.Equal(t, tt.wants.From, got.From)
			assert.Equal(t, tt.wants.To, got.To)
		})
	}
}

func TestParseDate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got, err := parseDate("")
		require.NoError(t, err)
		assert.True(t, got.IsZero())
	})
	t.Run("date only", func(t *testing.T) {
		got, err := parseDate("2024-03-15")
		require.NoError(t, err)
		assert.Equal(t, 2024, got.Year())
		assert.Equal(t, time.March, got.Month())
		assert.Equal(t, 15, got.Day())
	})
	t.Run("rfc3339", func(t *testing.T) {
		got, err := parseDate("2024-03-15T10:30:00Z")
		require.NoError(t, err)
		assert.Equal(t, 10, got.Hour())
	})
	t.Run("invalid", func(t *testing.T) {
		_, err := parseDate("not-a-date")
		require.Error(t, err)
	})
}

func TestContributionsRun_BadDateRange(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	opts := &ContributionsOptions{
		IO:         ios,
		HostConfig: testHostConfig("github.com"),
		HttpClient: func() (*http.Client, error) { return &http.Client{}, nil },
		From:       "2024-06-01",
		To:         "2024-01-01",
	}
	err := contributionsRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--to must be on or after --from")
}

func TestContributionsRun_RendersCalendar(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query ViewerContributions\b`),
		httpmock.StringResponse(`{
			"data": {
				"viewer": {
					"login": "octocat",
					"contributionsCollection": {
						"contributionCalendar": {
							"totalContributions": 42,
							"weeks": [
								{"contributionDays": [
									{"date": "2024-01-07", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-08", "contributionCount": 5, "contributionLevel": "SECOND_QUARTILE"},
									{"date": "2024-01-09", "contributionCount": 1, "contributionLevel": "FIRST_QUARTILE"},
									{"date": "2024-01-10", "contributionCount": 10, "contributionLevel": "FOURTH_QUARTILE"},
									{"date": "2024-01-11", "contributionCount": 7, "contributionLevel": "THIRD_QUARTILE"},
									{"date": "2024-01-12", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-13", "contributionCount": 0, "contributionLevel": "NONE"}
								]},
								{"contributionDays": [
									{"date": "2024-01-14", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-15", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-16", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-17", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-18", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-19", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-20", "contributionCount": 0, "contributionLevel": "NONE"}
								]},
								{"contributionDays": [
									{"date": "2024-01-21", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-22", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-23", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-24", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-25", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-26", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-27", "contributionCount": 0, "contributionLevel": "NONE"}
								]},
								{"contributionDays": [
									{"date": "2024-01-28", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-29", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-30", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-01-31", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-01", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-02", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-03", "contributionCount": 0, "contributionLevel": "NONE"}
								]},
								{"contributionDays": [
									{"date": "2024-02-04", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-05", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-06", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-07", "contributionCount": 2, "contributionLevel": "FIRST_QUARTILE"},
									{"date": "2024-02-08", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-09", "contributionCount": 0, "contributionLevel": "NONE"},
									{"date": "2024-02-10", "contributionCount": 0, "contributionLevel": "NONE"}
								]}
							]
						}
					}
				}
			}
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	opts := &ContributionsOptions{
		IO:         ios,
		HostConfig: testHostConfig("github.com"),
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
	}
	require.NoError(t, contributionsRun(opts))

	out := stdout.String()
	assert.Contains(t, out, "42")
	assert.Contains(t, out, "contributions")
	assert.Contains(t, out, "octocat")
	assert.Contains(t, out, "Jan")
	assert.Contains(t, out, "Feb")
	assert.Contains(t, out, "Less")
	assert.Contains(t, out, "More")
	assert.Contains(t, out, "M")
	assert.Contains(t, out, "W")
	assert.Contains(t, out, "F")
}

func TestContributionsRun_JSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query UserContributions\b`),
		httpmock.StringResponse(`{
			"data": {
				"user": {
					"login": "octocat",
					"contributionsCollection": {
						"contributionCalendar": {
							"totalContributions": 3,
							"weeks": [
								{"contributionDays": [
									{"date": "2024-01-01", "contributionCount": 1, "contributionLevel": "FIRST_QUARTILE"},
									{"date": "2024-01-02", "contributionCount": 2, "contributionLevel": "FIRST_QUARTILE"}
								]}
							]
						}
					}
				}
			}
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	exp := cmdutil.NewJSONExporter()
	exp.SetFields([]string{"login", "totalContributions", "days"})
	opts := &ContributionsOptions{
		IO:         ios,
		HostConfig: testHostConfig("github.com"),
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		User:       "octocat",
		Exporter:   exp,
	}
	require.NoError(t, contributionsRun(opts))

	out := stdout.String()
	assert.Contains(t, out, `"login":"octocat"`)
	assert.Contains(t, out, `"totalContributions":3`)
	assert.Contains(t, out, `"date":"2024-01-01"`)
}

func TestContributionsRun_UserNotFound(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`query UserContributions\b`),
		httpmock.StringResponse(`{"data": {"user": null}}`),
	)

	ios, _, _, _ := iostreams.Test()
	opts := &ContributionsOptions{
		IO:         ios,
		HostConfig: testHostConfig("github.com"),
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		User:       "ghost",
	}
	err := contributionsRun(opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestRenderMonthLabels(t *testing.T) {
	weeks := []ContributionWeek{
		{ContributionDays: []ContributionDay{{Date: "2024-01-07"}}},
		{ContributionDays: []ContributionDay{{Date: "2024-01-14"}}},
		{ContributionDays: []ContributionDay{{Date: "2024-01-21"}}},
		{ContributionDays: []ContributionDay{{Date: "2024-01-28"}}},
		{ContributionDays: []ContributionDay{{Date: "2024-02-04"}}},
	}
	got := renderMonthLabels(weeks)
	assert.True(t, strings.HasPrefix(got, "Jan"), "want Jan prefix, got %q", got)
	assert.Contains(t, got, "Feb")
}
