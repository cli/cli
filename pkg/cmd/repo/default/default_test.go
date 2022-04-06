package base

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/cli/cli/v2/context"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

func Test_defaultRun(t *testing.T) {
	setGitDir(t, "../../../../git/fixtures/simple.git")
	tests := []struct {
		name               string
		opts               DefaultOptions
		wantedErr          error
		wantedStdOut       string
		wantedResolvedName string
	}{
		{
			name: "Base repo set with view option",
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
			name: "Base repo not set with view option",
			opts: DefaultOptions{
				Remotes: func() (context.Remotes, error) {
					return []*context.Remote{
						{
							Remote: &git.Remote{
								Name: "origin",
							},
						},
					}, nil
				},
				ViewFlag: true,
			},
			wantedErr: errors.New("a default repo has not been set, use `gh repo default` to set a default repo"),
		},
		{
			name: "Base repo not set, assign non-interactively",
			opts: DefaultOptions{
				Remotes: func() (context.Remotes, error) {
					return []*context.Remote{
						{
							Remote: &git.Remote{
								Name: "origin",
							},
						},
						{
							Remote: &git.Remote{
								Name: "upstream",
							},
						},
					}, nil
				},
			},
			wantedResolvedName: "upstream",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			io, _, stdout, stderr := iostreams.Test()

			opts := tt.opts
			opts.IO = io
			opts.HttpClient = func() (*http.Client, error) { return nil, nil }

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
			if tt.wantedResolvedName != "" {
				resolvedAmount := 0
				remotes, err := git.Remotes()
				if err != nil {
					panic(err)
				}
				for _, r := range remotes {
					if r.Resolved == "base" {
						assert.Equal(t, r.Name, tt.wantedResolvedName)
						resolvedAmount++
					}
				}
				assert.Equal(t, 1, resolvedAmount)
			}
		})
	}
}

func setGitDir(t *testing.T, dir string) {
	old_GIT_DIR := os.Getenv("GIT_DIR")
	os.Setenv("GIT_DIR", dir)
	t.Cleanup(func() {
		_ = git.UnsetRemoteResolution("upstream") 
		os.Setenv("GIT_DIR", old_GIT_DIR)
	})
}
