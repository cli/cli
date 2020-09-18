package view

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/httpmock"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/test"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name     string
		cli      string
		wants    ViewOptions
		wantsErr bool
	}{
		{
			name: "no args",
			cli:  "",
			wants: ViewOptions{
				RepoArg: "",
				Web:     false,
			},
		},
		{
			name: "sets repo arg",
			cli:  "some/repo",
			wants: ViewOptions{
				RepoArg: "some/repo",
				Web:     false,
			},
		},
		{
			name: "sets web",
			cli:  "-w",
			wants: ViewOptions{
				RepoArg: "",
				Web:     true,
			},
		},
		{
			name: "sets branch",
			cli:  "-b feat/awesome",
			wants: ViewOptions{
				RepoArg: "",
				Branch:  "feat/awesome",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, _, _ := iostreams.Test()

			f := &cmdutil.Factory{
				IOStreams: io,
			}

			// THOUGHT: this seems ripe for cmdutil. It's almost identical to the set up for the same test
			// in gist create.
			argv, err := shlex.Split(tt.cli)
			assert.NoError(t, err)

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()
			if tt.wantsErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)

			assert.Equal(t, tt.wants.Web, gotOpts.Web)
			assert.Equal(t, tt.wants.Branch, gotOpts.Branch)
			assert.Equal(t, tt.wants.RepoArg, gotOpts.RepoArg)
		})
	}
}

func Test_RepoView_Web(t *testing.T) {
	tests := []struct {
		name       string
		stdoutTTY  bool
		wantStderr string
	}{
		{
			name:       "tty",
			stdoutTTY:  true,
			wantStderr: "Opening github.com/OWNER/REPO in your browser.\n",
		},
		{
			name:       "nontty",
			wantStderr: "",
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.StubRepoInfoResponse("OWNER", "REPO", "main")

		opts := &ViewOptions{
			Web: true,
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
		}

		io, _, stdout, stderr := iostreams.Test()

		opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			cs, teardown := test.InitCmdStubber()
			defer teardown()

			cs.Stub("") // browser open

			if err := viewRun(opts); err != nil {
				t.Errorf("viewRun() error = %v", err)
			}
			assert.Equal(t, "", stdout.String())
			assert.Equal(t, 1, len(cs.Calls))
			call := cs.Calls[0]
			assert.Equal(t, "https://github.com/OWNER/REPO", call.Args[len(call.Args)-1])
			assert.Equal(t, tt.wantStderr, stderr.String())
			reg.Verify(t)
		})
	}
}

func Test_ViewRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ViewOptions
		repoName   string
		stdoutTTY  bool
		wantOut    string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "nontty",
			wantOut: heredoc.Doc(`
				name:	OWNER/REPO
				description:	social distancing
				--
				# truly cool readme check it out
				`),
		},
		{
			name:     "url arg",
			repoName: "jill/valentine",
			opts: &ViewOptions{
				RepoArg: "https://github.com/jill/valentine",
			},
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				jill/valentine
				social distancing


				  # truly cool readme check it out                                            



				View this repository on GitHub: https://github.com/jill/valentine
			`),
		},
		{
			name:     "name arg",
			repoName: "jill/valentine",
			opts: &ViewOptions{
				RepoArg: "jill/valentine",
			},
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				jill/valentine
				social distancing


				  # truly cool readme check it out                                            



				View this repository on GitHub: https://github.com/jill/valentine
			`),
		},
		{
			name: "branch arg",
			opts: &ViewOptions{
				Branch: "feat/awesome",
			},
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				OWNER/REPO
				social distancing


				  # truly cool readme check it out                                            



				View this repository on GitHub: https://github.com/OWNER/REPO
			`),
		},
		{
			name:      "no args",
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				OWNER/REPO
				social distancing


				  # truly cool readme check it out                                            



				View this repository on GitHub: https://github.com/OWNER/REPO
			`),
		},
	}
	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &ViewOptions{}
		}

		if tt.repoName == "" {
			tt.repoName = "OWNER/REPO"
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			repo, _ := ghrepo.FromFullName(tt.repoName)
			return repo, nil
		}

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(`
		{ "data": {
			"repository": {
			"description": "social distancing"
		} } }`))
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("repos/%s/readme", tt.repoName)),
			httpmock.StringResponse(`
		{ "name": "readme.md",
		"content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}`))

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, stderr := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := viewRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("viewRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}

func Test_ViewRun_NonMarkdownReadme(t *testing.T) {
	tests := []struct {
		name      string
		stdoutTTY bool
		wantOut   string
	}{
		{
			name: "tty",
			wantOut: heredoc.Doc(`
			OWNER/REPO
			social distancing

			# truly cool readme check it out

			View this repository on GitHub: https://github.com/OWNER/REPO
			`),
			stdoutTTY: true,
		},
		{
			name: "nontty",
			wantOut: heredoc.Doc(`
			name:	OWNER/REPO
			description:	social distancing
			--
			# truly cool readme check it out
			`),
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(`
		{ "data": {
				"repository": {
				"description": "social distancing"
		} } }`))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/readme"),
			httpmock.StringResponse(`
		{ "name": "readme.org",
		"content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}`))

		opts := &ViewOptions{
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
		}

		io, _, stdout, stderr := iostreams.Test()

		opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := viewRun(opts); err != nil {
				t.Errorf("viewRun() error = %v", err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, "", stderr.String())
			reg.Verify(t)
		})
	}
}

func Test_ViewRun_NoReadme(t *testing.T) {
	tests := []struct {
		name      string
		stdoutTTY bool
		wantOut   string
	}{
		{
			name: "tty",
			wantOut: heredoc.Doc(`
			OWNER/REPO
			social distancing

			This repository does not have a README

			View this repository on GitHub: https://github.com/OWNER/REPO
			`),
			stdoutTTY: true,
		},
		{
			name: "nontty",
			wantOut: heredoc.Doc(`
			name:	OWNER/REPO
			description:	social distancing
			`),
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(`
		{ "data": {
				"repository": {
				"description": "social distancing"
		} } }`))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/readme"),
			httpmock.StatusStringResponse(404, `{}`))

		opts := &ViewOptions{
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
		}

		io, _, stdout, stderr := iostreams.Test()

		opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := viewRun(opts); err != nil {
				t.Errorf("viewRun() error = %v", err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, "", stderr.String())
			reg.Verify(t)
		})
	}
}

func Test_ViewRun_NoDescription(t *testing.T) {
	tests := []struct {
		name      string
		stdoutTTY bool
		wantOut   string
	}{
		{
			name: "tty",
			wantOut: heredoc.Doc(`
			OWNER/REPO
			No description provided

			# truly cool readme check it out

			View this repository on GitHub: https://github.com/OWNER/REPO
			`),
			stdoutTTY: true,
		},
		{
			name: "nontty",
			wantOut: heredoc.Doc(`
			name:	OWNER/REPO
			description:	
			--
			# truly cool readme check it out
			`),
		},
	}

	for _, tt := range tests {
		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(`
		{ "data": {
				"repository": {
				"description": ""
		} } }`))
		reg.Register(
			httpmock.REST("GET", "repos/OWNER/REPO/readme"),
			httpmock.StringResponse(`
		{ "name": "readme.org",
		"content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}`))

		opts := &ViewOptions{
			HttpClient: func() (*http.Client, error) {
				return &http.Client{Transport: reg}, nil
			},
			BaseRepo: func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			},
		}

		io, _, stdout, stderr := iostreams.Test()

		opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := viewRun(opts); err != nil {
				t.Errorf("viewRun() error = %v", err)
			}
			assert.Equal(t, tt.wantOut, stdout.String())
			assert.Equal(t, "", stderr.String())
			reg.Verify(t)
		})
	}
}

func Test_ViewRun_WithoutUsername(t *testing.T) {
	reg := &httpmock.Registry{}
	reg.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`
		{ "data": { "viewer": {
			"login": "OWNER"
		}}}`))
	reg.Register(
		httpmock.GraphQL(`query RepositoryInfo\b`),
		httpmock.StringResponse(`
	{ "data": {
		"repository": {
		"description": "social distancing"
	} } }`))
	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/readme"),
		httpmock.StringResponse(`
	{ "name": "readme.md",
	"content": "IyB0cnVseSBjb29sIHJlYWRtZSBjaGVjayBpdCBvdXQ="}`))

	io, _, stdout, stderr := iostreams.Test()
	io.SetStdoutTTY(false)

	opts := &ViewOptions{
		RepoArg: "REPO",
		HttpClient: func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		},
		IO: io,
	}

	if err := viewRun(opts); err != nil {
		t.Errorf("viewRun() error = %v", err)
	}

	assert.Equal(t, heredoc.Doc(`
			name:	OWNER/REPO
			description:	social distancing
			--
			# truly cool readme check it out
			`), stdout.String())
	assert.Equal(t, "", stderr.String())
	reg.Verify(t)
}

func Test_ViewRun_HandlesSpecialCharacters(t *testing.T) {
	tests := []struct {
		name       string
		opts       *ViewOptions
		repoName   string
		stdoutTTY  bool
		wantOut    string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "nontty",
			wantOut: heredoc.Doc(`
				name:	OWNER/REPO
				description:	Some basic special characters " & / < > '
				--
				# < is always > than & ' and "
				`),
		},
		{
			name:      "no args",
			stdoutTTY: true,
			wantOut: heredoc.Doc(`
				OWNER/REPO
				Some basic special characters " & / < > '


				  # < is always > than & ' and "                                              



				View this repository on GitHub: https://github.com/OWNER/REPO
			`),
		},
	}
	for _, tt := range tests {
		if tt.opts == nil {
			tt.opts = &ViewOptions{}
		}

		if tt.repoName == "" {
			tt.repoName = "OWNER/REPO"
		}

		tt.opts.BaseRepo = func() (ghrepo.Interface, error) {
			repo, _ := ghrepo.FromFullName(tt.repoName)
			return repo, nil
		}

		reg := &httpmock.Registry{}
		reg.Register(
			httpmock.GraphQL(`query RepositoryInfo\b`),
			httpmock.StringResponse(`
		{ "data": {
			"repository": {
			"description": "Some basic special characters \" & / < > '"
		} } }`))
		reg.Register(
			httpmock.REST("GET", fmt.Sprintf("repos/%s/readme", tt.repoName)),
			httpmock.StringResponse(`
		{ "name": "readme.md",
		"content": "IyA8IGlzIGFsd2F5cyA+IHRoYW4gJiAnIGFuZCAi"}`))

		tt.opts.HttpClient = func() (*http.Client, error) {
			return &http.Client{Transport: reg}, nil
		}

		io, _, stdout, stderr := iostreams.Test()
		tt.opts.IO = io

		t.Run(tt.name, func(t *testing.T) {
			io.SetStdoutTTY(tt.stdoutTTY)

			if err := viewRun(tt.opts); (err != nil) != tt.wantErr {
				t.Errorf("viewRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			assert.Equal(t, tt.wantStderr, stderr.String())
			assert.Equal(t, tt.wantOut, stdout.String())
			reg.Verify(t)
		})
	}
}
