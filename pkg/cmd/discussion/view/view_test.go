package view

import (
	"bytes"
	"testing"
	"time"

	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testDiscussion() *client.Discussion {
	return &client.Discussion{
		ID:     "D_123",
		Number: 123,
		Title:  "How to authenticate with SSO?",
		Body:   "I need help with SSO authentication.",
		URL:    "https://github.com/OWNER/REPO/discussions/123",
		State:  "OPEN",
		Author: client.DiscussionAuthor{Login: "monalisa"},
		Category: client.DiscussionCategory{
			Name: "Q&A", Slug: "q-a", IsAnswerable: true,
		},
		Labels:   []client.DiscussionLabel{{Name: "help-wanted", Color: "0075ca"}},
		Answered: false,
		Comments: client.DiscussionCommentList{TotalCount: 3},
		ReactionGroups: []client.ReactionGroup{
			{Content: "THUMBS_UP", TotalCount: 5},
			{Content: "ROCKET", TotalCount: 2},
		},
		CreatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestNewCmdView(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantNum int
		wantErr string
	}{
		{
			name:    "number argument",
			args:    []string{"123"},
			wantNum: 123,
		},
		{
			name:    "hash number argument",
			args:    []string{"#456"},
			wantNum: 456,
		},
		{
			name:    "URL argument",
			args:    []string{"https://github.com/OWNER/REPO/discussions/789"},
			wantNum: 789,
		},
		{
			name:    "invalid argument",
			args:    []string{"not-a-number"},
			wantErr: "invalid discussion argument",
		},
		{
			name:    "no arguments",
			args:    []string{},
			wantErr: "accepts 1 arg(s), received 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{}
			ios, _, _, _ := iostreams.Test()
			f.IOStreams = ios
			f.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			f.Browser = &browser.Stub{}

			var gotOpts *ViewOptions
			cmd := NewCmdView(f, func(opts *ViewOptions) error {
				gotOpts = opts
				return nil
			})

			cmd.SetArgs(tt.args)
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			err := cmd.Execute()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNum, gotOpts.DiscussionNumber)
		})
	}
}

func TestViewRun_tty(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	d := testDiscussion()
	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			return d, nil
		},
	}

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Client: func() (client.DiscussionClient, error) {
			return mock, nil
		},
		DiscussionNumber: 123,
		Now:              func() time.Time { return time.Date(2025, 3, 1, 1, 0, 0, 0, time.UTC) },
	}

	err := viewRun(opts)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "How to authenticate with SSO?")
	assert.Contains(t, out, "#123")
	assert.Contains(t, out, "Q&A")
	assert.Contains(t, out, "Asked by")
	assert.Contains(t, out, "monalisa")
	assert.Contains(t, out, "3 comments")
	assert.Contains(t, out, "help-wanted")
	assert.Contains(t, out, "View this discussion on GitHub")
}

func TestViewRun_nontty(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(false)

	d := testDiscussion()
	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			return d, nil
		},
	}

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Client: func() (client.DiscussionClient, error) {
			return mock, nil
		},
		DiscussionNumber: 123,
		Now:              time.Now,
	}

	err := viewRun(opts)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "title:\tHow to authenticate with SSO?")
	assert.Contains(t, out, "state:\tOPEN")
	assert.Contains(t, out, "category:\tQ&A")
	assert.Contains(t, out, "author:\tmonalisa")
	assert.Contains(t, out, "labels:\thelp-wanted")
	assert.Contains(t, out, "number:\t123")
	assert.Contains(t, out, "--")
	assert.Contains(t, out, "I need help with SSO authentication.")
}

func TestViewRun_json(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(false)

	d := testDiscussion()
	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			return d, nil
		},
	}

	exporter := cmdutil.NewJSONExporter()
	exporter.SetFields(shared.DiscussionFields)

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Client: func() (client.DiscussionClient, error) {
			return mock, nil
		},
		DiscussionNumber: 123,
		Exporter:         exporter,
		Now:              time.Now,
	}

	err := viewRun(opts)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, `"title"`)
	assert.Contains(t, out, `"number"`)
	assert.Contains(t, out, "How to authenticate with SSO?")
}

func TestViewRun_web(t *testing.T) {
	ios, _, _, stderr := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	b := &browser.Stub{}

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Browser:          b,
		DiscussionNumber: 123,
		WebMode:          true,
		Now:              time.Now,
	}

	err := viewRun(opts)
	require.NoError(t, err)

	b.Verify(t, "https://github.com/OWNER/REPO/discussions/123")
	assert.Contains(t, stderr.String(), "Opening")
}

func TestViewRun_urlArg(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(false)

	d := testDiscussion()
	d.URL = "https://github.com/OTHER/REPO/discussions/42"
	d.Number = 42

	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			assert.Equal(t, "OTHER", repo.RepoOwner())
			assert.Equal(t, "REPO", repo.RepoName())
			assert.Equal(t, 42, number)
			return d, nil
		},
	}

	f := &cmdutil.Factory{}
	f.IOStreams = ios
	f.BaseRepo = func() (ghrepo.Interface, error) {
		return ghrepo.New("OWNER", "REPO"), nil
	}
	f.Browser = &browser.Stub{}

	var gotOpts *ViewOptions
	cmd := NewCmdView(f, func(opts *ViewOptions) error {
		gotOpts = opts
		opts.Client = func() (client.DiscussionClient, error) {
			return mock, nil
		}
		return viewRun(opts)
	})

	cmd.SetArgs([]string{"https://github.com/OTHER/REPO/discussions/42"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	err := cmd.Execute()
	require.NoError(t, err)
	assert.Equal(t, 42, gotOpts.DiscussionNumber)

	out := stdout.String()
	assert.Contains(t, out, "number:\t42")
}

func TestViewRun_answerable(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	d := testDiscussion()
	d.Category.IsAnswerable = true

	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			return d, nil
		},
	}

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Client: func() (client.DiscussionClient, error) {
			return mock, nil
		},
		DiscussionNumber: 123,
		Now:              func() time.Time { return time.Date(2025, 3, 1, 1, 0, 0, 0, time.UTC) },
	}

	err := viewRun(opts)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Asked by")
}

func TestViewRun_notAnswerable(t *testing.T) {
	ios, _, stdout, _ := iostreams.Test()
	ios.SetStdoutTTY(true)
	ios.SetStderrTTY(true)

	d := testDiscussion()
	d.Category.Name = "General"
	d.Category.IsAnswerable = false

	mock := &client.DiscussionClientMock{
		GetByNumberFunc: func(repo ghrepo.Interface, number int) (*client.Discussion, error) {
			return d, nil
		},
	}

	opts := &ViewOptions{
		IO: ios,
		BaseRepo: func() (ghrepo.Interface, error) {
			return ghrepo.New("OWNER", "REPO"), nil
		},
		Client: func() (client.DiscussionClient, error) {
			return mock, nil
		},
		DiscussionNumber: 123,
		Now:              func() time.Time { return time.Date(2025, 3, 1, 1, 0, 0, 0, time.UTC) },
	}

	err := viewRun(opts)
	require.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "Started by")
	assert.NotContains(t, out, "Asked by")
}
