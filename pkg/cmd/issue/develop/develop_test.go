package develop

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/stretchr/testify/assert"
)

func Test_developRun(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*DevelopOptions, *testing.T) func()
		cmdStubs       func(*run.CommandStubber)
		askStubs       func(*prompt.AskStubber) // TODO eventually migrate to PrompterMock
		httpStubs      func(*httpmock.Registry, *testing.T)
		expectedOut    string
		expectedErrOut string
		expectedBrowse string
		wantErr        string
		tty            bool
	}{
		{name: "develop new branch",
			setup: func(opts *DevelopOptions, t *testing.T) func() {
				opts.Name = "my-branch"
				opts.BaseBranch = "main"
				opts.IssueSelector = "123"
				return func() {}
			},
			httpStubs: func(reg *httpmock.Registry, t *testing.T) {
				reg.StubRepoResponse("OWNER", "REPO")
				reg.Register(
					httpmock.GraphQL(`query IssueByNumber\b`),
					httpmock.StringResponse(`{"data":{"repository":{
						"hasIssuesEnabled": true,
						"issue":{"id":1, "number":123, "title":"my issue"},
					}}}`))
				reg.Register(
					httpmock.GraphQL(`query BranchIssueReferenceFindBaseOid\b`),
					httpmock.StringResponse(`{"data":{"repository":{"ref":{"target":{"oid":"123"}}}}}`))

				reg.Register(
					httpmock.GraphQL(`mutation CreateLinkedBranch\b`),
					httpmock.GraphQLMutation(`
					{ "data": { "createLinkedBranch": { "linkedBranch": 1 } } }`,
						func(inputs map[string]interface{}) {
						}),
				)

			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)
			if tt.httpStubs != nil {
				tt.httpStubs(reg, t)
			}

			opts := DevelopOptions{}

			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			}
			opts.Config = func() (config.Config, error) {
				return config.NewBlankConfig(), nil
			}
			cleanSetup := func() {}
			if tt.setup != nil {
				cleanSetup = tt.setup(&opts, t)
			}
			defer cleanSetup()

			err := developRun(&opts)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
