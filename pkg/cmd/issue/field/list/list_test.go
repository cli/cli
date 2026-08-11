package list

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields(t, NewCmdList, []string{
		"id",
		"name",
		"dataType",
		"options",
	})
}

func TestListRun(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query RepositoryIssueFields\b`),
		httpmock.StringResponse(`{
			"data":{"repository":{"issueFields":{"nodes":[
				{"id":"IF_1","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_1","name":"High"},{"id":"OPT_2","name":"Low"}]},
				{"id":"IF_2","name":"Due date","dataType":"DATE"}
			],"pageInfo":{"hasNextPage":false}}}}
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	opts := &ListOptions{
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		BaseRepo:   func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
		IO:         ios,
	}

	err := listRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "NAME      DATA TYPE      OPTIONS\n")
	assert.Contains(t, stdout.String(), "Priority  single_select  High, Low\n")
	assert.Contains(t, stdout.String(), "Due date  date")
}

func TestListRunJSON(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)
	reg.Register(
		httpmock.GraphQL(`query RepositoryIssueFields\b`),
		httpmock.StringResponse(`{
			"data":{"repository":{"issueFields":{"nodes":[
				{"id":"IF_1","name":"Priority","dataType":"SINGLE_SELECT","options":[{"id":"OPT_1","name":"High"}]}
			],"pageInfo":{"hasNextPage":false}}}}
		}`),
	)

	ios, _, stdout, _ := iostreams.Test()
	exporter := cmdutil.NewJSONExporter()
	exporter.SetFields([]string{"name", "dataType", "options"})
	opts := &ListOptions{
		HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
		BaseRepo:   func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
		IO:         ios,
		Exporter:   exporter,
	}

	err := listRun(opts)
	require.NoError(t, err)
	assert.JSONEq(t, `[{
		"name":"Priority",
		"dataType":"single_select",
		"options":[{"id":"OPT_1","name":"High"}]
	}]`, stdout.String())
}

func TestListRunEmpty(t *testing.T) {
	tests := []struct {
		name       string
		exporter   cmdutil.Exporter
		wantOutput string
		wantError  string
	}{
		{
			name:      "human output",
			wantError: "no issue fields found in OWNER/REPO",
		},
		{
			name:       "JSON output",
			exporter:   cmdutil.NewJSONExporter(),
			wantOutput: "[]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			reg.Register(
				httpmock.GraphQL(`query RepositoryIssueFields\b`),
				httpmock.StringResponse(`{"data":{"repository":{"issueFields":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`),
			)

			ios, _, stdout, _ := iostreams.Test()
			opts := &ListOptions{
				HttpClient: func() (*http.Client, error) { return &http.Client{Transport: reg}, nil },
				BaseRepo:   func() (ghrepo.Interface, error) { return ghrepo.New("OWNER", "REPO"), nil },
				IO:         ios,
				Exporter:   tt.exporter,
			}

			err := listRun(opts)
			if tt.wantError != "" {
				require.EqualError(t, err, tt.wantError)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantOutput, stdout.String())
		})
	}
}
