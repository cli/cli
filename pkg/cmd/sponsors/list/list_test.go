package list

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdList(t *testing.T) {
	tests := []struct {
		name  string
		cli   string
		wants string
	}{
		{
			name:  "happy path",
			cli:   "octocat",
			wants: "octocat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			f := &cmdutil.Factory{
				IOStreams: ios,
			}

			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ListOptions
			cmd := NewCmdList(f, func(opts *ListOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			assert.NoError(t, err)

			assert.Equal(t, tt.wants, gotOpts.Username)
		})
	}
}

func Test_listRun(t *testing.T) {
	tests := []struct {
		name  string
		opts  *ListOptions
		wants []string
	}{
		{
			name: "happy path",
			opts: &ListOptions{
				HttpClient: func() (*http.Client, error) {
					r := &httpmock.Registry{}

					r.Register(
						httpmock.GraphQL(`query SponsorsList\b`),
						httpmock.StringResponse(`{"data": {"user": {"sponsors": {"totalCount": 2, "nodes": [{"login": "mona"}, {"login": "lisa"}]}}}}`))

					return &http.Client{Transport: r}, nil
				},

				Username: "octocat",
				Sponsors: []string{},
			},
			wants: []string{"mona", "lisa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			err := listRun(tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wants, tt.opts.Sponsors)
		})
	}
}

func Test_formatTTYOutput(t *testing.T) {
	tests := []struct {
		name  string
		opts  *ListOptions
		wants []string
	}{
		{
			name: "Simple table",
			opts: &ListOptions{
				Username: "octocat",
				Sponsors: []string{"mona", "lisa"},
			},
			wants: []string{
				"SPONSORS",
				"mona",
				"lisa",
			},
		},
		{
			name: "No Sponsors for octocat",
			opts: &ListOptions{
				Username: "octocat",
				Sponsors: []string{},
			},
			wants: []string{"No sponsors found for octocat"},
		},
		{
			name: "No Sponsors for monalisa",
			opts: &ListOptions{
				Username: "monalisa",
				Sponsors: []string{},
			},
			wants: []string{"No sponsors found for monalisa"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			tt.opts.IO = ios

			err := formatTTYOutput(tt.opts)
			assert.NoError(t, err)

			expected := fmt.Sprintf("%s\n", strings.Join(tt.wants, "\n"))
			assert.Equal(t, expected, stdout.String())
		})
	}
}
