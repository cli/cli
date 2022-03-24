package base

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_defaultRun(t *testing.T) {
	tests := []struct {
		name         string
		opts         DefaultOptions
		wantedErr    error
		wantedStdOut string
	}{
		{
			opts: DefaultOptions{
				Remotes: func() (context.Remotes, error) {
					return []*context.Remote{
						{
							Remote: &git.Remote{
								Name:     "origin",
								Resolved: "base",
							},
							Repo: ghrepo.New("hubot", "Spoon-Knife"),
						},
					}, nil
				},
				ViewFlag: true,
			},
			wantedStdOut: "hubot/Spoon-Knife",
		},
		{
			opts: DefaultOptions{
				Remotes: func() (context.Remotes, error) {
					return []*context.Remote{
						{
							Remote: &git.Remote{
								Name: "origin",
							},
							Repo: ghrepo.New("hubot", "Spoon-Knife"),
						},
					}, nil
				},
				ViewFlag: true,
			},
			wantedErr: errors.New("a default repo has not been set, use `gh repo default` to set a default repo"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()

			reg := httpmock.Registry{}
			defer reg.Verify(t)

			opts := tt.opts
			opts.IO = io
			opts.HttpClient = func() (*http.Client, error) {
				return &http.Client{Transport: &reg}, nil
			}

			err := runDefault(&opts)
			if tt.wantedErr != nil {
				assert.EqualError(t, tt.wantedErr, err.Error())
			} else {
				assert.NoError(t, err)
				if opts.ViewFlag {
					assert.Equal(t, fmt.Sprintf("%s\n", tt.wantedStdOut), stdout.String())
				} else {
					assert.Equal(t, "", stdout.String())
					assert.Equal(t, "", stderr.String())
				}
			}
		})
	}
}
