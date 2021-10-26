package rename

import (
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/prompt"
	"github.com/stretchr/testify/assert"
)

func TestRenameRun(t *testing.T) {
	testCases := []struct {
		name      string
		opts      RenameOptions
		httpStubs func(*httpmock.Registry)
		execStubs func(*run.CommandStubber)
		askStubs  func(*prompt.AskStubber)
		wantOut   string
		tty       bool
		prompt    bool
	}{
		{
			name:    "none argument",
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote \n",
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("NEW_REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
			tty: true,
		},
		{
			name: "owner repo change name prompt",
			opts: RenameOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote \n",
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("NEW_REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
			tty: true,
		},
		{
			name: "owner repo change name prompt no tty",
			opts: RenameOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
			},
			askStubs: func(q *prompt.AskStubber) {
				q.StubOne("NEW_REPO")
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
		},
		{
			name: "owner repo change name argument tty",
			opts: RenameOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				newRepoSelector: "NEW_REPO",
			},
			wantOut: "✓ Renamed repository OWNER/NEW_REPO\n✓ Updated the \"origin\" remote \n",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
			tty: true,
		},
		{
			name: "owner repo change name argument no tty",
			opts: RenameOptions{
				BaseRepo: func() (ghrepo.Interface, error) {
					return ghrepo.New("OWNER", "REPO"), nil
				},
				newRepoSelector: "NEW_REPO",
			},
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("PATCH", "repos/OWNER/REPO"),
					httpmock.StatusStringResponse(204, "{}"))
			},
			execStubs: func(cs *run.CommandStubber) {
				cs.Register(`git remote set-url origin https://github.com/OWNER/NEW_REPO.git`, 0, "")
			},
		},
	}

	for _, tt := range testCases {
		q, teardown := prompt.InitAskStubber()
		defer teardown()
		if tt.askStubs != nil {
			tt.askStubs(q)
		}

		repo, _ := ghrepo.FromFullName("OWNER/REPO")
		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			return repo, nil
		}

		tt.opts.Config = func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		}

		tt.opts.Remotes = func() (context.Remotes, error) {
			return []*context.Remote{
				{
					Remote: &git.Remote{Name: "origin"},
					Repo:   repo,
				},
			}, nil
		}

		cs, restoreRun := run.Stub()
		defer restoreRun(t)
		if tt.execStubs != nil {
			tt.execStubs(cs)
		}

		reg := &httpmock.Registry{}
		if tt.httpStubs != nil {
			tt.httpStubs(reg)
		}
		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, _ := iostreams.Test()
		io.SetStdinTTY(tt.tty)
		io.SetStdoutTTY(tt.tty)
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			defer reg.Verify(t)
			err := renameRun(&tt.opts)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}
