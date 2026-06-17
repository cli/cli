package edit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdEdit(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		isTTY        bool
		wantOpts     EditOptions
		wantBaseRepo ghrepo.Interface
		wantErr      string
	}{
		{
			name:  "all flags",
			args:  "123 --title 'New title' --body 'New body' --category 'Ideas'",
			isTTY: true,
			wantOpts: EditOptions{
				DiscussionNumber: 123,
				TitleProvided:    true,
				Title:            "New title",
				BodyProvided:     true,
				Body:             "New body",
				CategoryProvided: true,
				Category:         "Ideas",
			},
		},
		{
			name:  "url arg overrides base repo",
			args:  "https://github.com/OWNER2/REPO2/discussions/42",
			isTTY: true,
			wantOpts: EditOptions{
				DiscussionNumber: 42,
				Interactive:      true,
			},
			wantBaseRepo: ghrepo.New("OWNER2", "REPO2"),
		},
		{
			name:  "interactive mode when no flags and tty",
			args:  "123",
			isTTY: true,
			wantOpts: EditOptions{
				DiscussionNumber: 123,
				Interactive:      true,
			},
		},
		{
			name:  "labels flags",
			args:  "123 --add-label 'bug,help wanted' --remove-label stale",
			isTTY: true,
			wantOpts: EditOptions{
				DiscussionNumber: 123,
				AddLabels:        []string{"bug", "help wanted"},
				RemoveLabels:     []string{"stale"},
				LabelsProvided:   true,
			},
		},
		{
			name:    "mutual exclusion --body and --body-file",
			args:    "123 --body 'inline' --body-file body.md",
			isTTY:   true,
			wantErr: "specify only one of --body or --body-file",
		},
		{
			name:    "no flags no TTY",
			args:    "123",
			isTTY:   false,
			wantErr: "specify at least one flag to update the discussion non-interactively",
		},
		{
			name:    "no args",
			args:    "",
			isTTY:   true,
			wantErr: "accepts 1 arg(s)",
		},
		{
			name:    "extra args",
			args:    "123 extra",
			isTTY:   true,
			wantErr: "accepts 1 arg(s)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			f := &cmdutil.Factory{IOStreams: ios}
			var gotOpts *EditOptions
			cmd := NewCmdEdit(f, func(opts *EditOptions) error {
				gotOpts = opts
				return nil
			})
			cmd.SetIn(&bytes.Buffer{})
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)
			cmd.SetArgs(argv)

			_, err = cmd.ExecuteC()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOpts.DiscussionNumber, gotOpts.DiscussionNumber)
			assert.Equal(t, tt.wantOpts.Interactive, gotOpts.Interactive)
			assert.Equal(t, tt.wantOpts.TitleProvided, gotOpts.TitleProvided)
			assert.Equal(t, tt.wantOpts.BodyProvided, gotOpts.BodyProvided)
			assert.Equal(t, tt.wantOpts.CategoryProvided, gotOpts.CategoryProvided)
			assert.Equal(t, tt.wantOpts.LabelsProvided, gotOpts.LabelsProvided)
			assert.Equal(t, tt.wantOpts.Title, gotOpts.Title)
			assert.Equal(t, tt.wantOpts.Body, gotOpts.Body)
			assert.Equal(t, tt.wantOpts.Category, gotOpts.Category)
			assert.Equal(t, tt.wantOpts.AddLabels, gotOpts.AddLabels)
			assert.Equal(t, tt.wantOpts.RemoveLabels, gotOpts.RemoveLabels)

			if tt.wantBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				assert.True(t, ghrepo.IsSame(tt.wantBaseRepo, baseRepo))
			}
		})
	}
}

func TestEditRun(t *testing.T) {
	tests := []struct {
		name            string
		opts            EditOptions
		bodyFileContent string // if non-empty, creates a temp file and sets opts.BodyFile
		stdinContent    string // if non-empty, writes to stdin buffer
		isTTY           bool
		setupMock       func(*client.DiscussionClientMock)
		prompter        *prompter.PrompterMock
		wantErr         string
		wantOut         string
	}{
		{
			name: "success non-tty title only",
			opts: EditOptions{
				Title:         "Updated title",
				TitleProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "D_1", input.DiscussionID)
					require.NotNil(t, input.Title)
					assert.Equal(t, "Updated title", *input.Title)
					assert.Nil(t, input.Body)
					assert.Nil(t, input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty body only",
			opts: EditOptions{
				Body:         "Updated body",
				BodyProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					require.NotNil(t, input.Body)
					assert.Equal(t, "Updated body", *input.Body)
					assert.Nil(t, input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty category change",
			opts: EditOptions{
				Category:         "Q&A",
				CategoryProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					assert.Nil(t, input.Body)
					require.NotNil(t, input.CategoryID)
					assert.Equal(t, "CAT2", *input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty add/remove labels only",
			opts: EditOptions{
				AddLabels:      []string{"bug", "enhancement"},
				RemoveLabels:   []string{"stale"},
				LabelsProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_bug", Name: "bug"},
						{ID: "L_enh", Name: "enhancement"},
						{ID: "L_stale", Name: "stale"},
					}, nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					assert.Nil(t, input.Body)
					assert.Nil(t, input.CategoryID)
					assert.Equal(t, []string{"L_bug", "L_enh"}, input.AddLabelIDs)
					assert.Equal(t, []string{"L_stale"}, input.RemoveLabelIDs)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty add labels only",
			opts: EditOptions{
				AddLabels:      []string{"bug", "enhancement"},
				LabelsProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_bug", Name: "bug"},
						{ID: "L_enh", Name: "enhancement"},
					}, nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, []string{"L_bug", "L_enh"}, input.AddLabelIDs)
					assert.Nil(t, input.RemoveLabelIDs)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty remove labels only",
			opts: EditOptions{
				RemoveLabels:   []string{"stale"},
				LabelsProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_stale", Name: "stale"},
					}, nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.AddLabelIDs)
					assert.Equal(t, []string{"L_stale"}, input.RemoveLabelIDs)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty all flags",
			opts: EditOptions{
				Title:            "New title",
				Body:             "New body",
				Category:         "General",
				AddLabels:        []string{"bug"},
				RemoveLabels:     []string{"stale"},
				TitleProvided:    true,
				BodyProvided:     true,
				CategoryProvided: true,
				LabelsProvided:   true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_bug", Name: "bug"},
						{ID: "L_stale", Name: "stale"},
					}, nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					require.NotNil(t, input.Title)
					assert.Equal(t, "New title", *input.Title)
					require.NotNil(t, input.Body)
					assert.Equal(t, "New body", *input.Body)
					require.NotNil(t, input.CategoryID)
					assert.Equal(t, "CAT1", *input.CategoryID)
					assert.Equal(t, []string{"L_bug"}, input.AddLabelIDs)
					assert.Equal(t, []string{"L_stale"}, input.RemoveLabelIDs)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "non-tty blank title returns error",
			opts: EditOptions{
				Title:         "   ",
				TitleProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
			},
			wantErr: "title cannot be blank",
		},
		{
			name: "non-tty unknown category",
			opts: EditOptions{
				Category:         "nonexistent",
				CategoryProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
			},
			wantErr: `unknown category: "nonexistent"`,
		},
		{
			name: "non-tty list categories error",
			opts: EditOptions{
				Category:         "General",
				CategoryProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return nil, fmt.Errorf("network error")
				}
			},
			wantErr: "network error",
		},
		{
			name: "non-tty unresolvable label returns error",
			opts: EditOptions{
				AddLabels:      []string{"bug", "nonexistent", "also-missing"},
				LabelsProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_bug", Name: "bug"},
					}, nil
				}
			},
			wantErr: "labels not found: nonexistent, also-missing",
		},
		{
			name: "GetByNumber error",
			opts: EditOptions{
				Title:         "whatever",
				TitleProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return nil, fmt.Errorf("not found")
				}
			},
			wantErr: "not found",
		},
		{
			name: "Update error",
			opts: EditOptions{
				Title:         "Updated title",
				TitleProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					return nil, fmt.Errorf("mutation failed")
				}
			},
			wantErr: "mutation failed",
		},
		{
			name:  "tty interactive select title",
			isTTY: true,
			opts:  EditOptions{Interactive: true},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					require.NotNil(t, input.Title)
					assert.Equal(t, "New title", *input.Title)
					assert.Nil(t, input.Body)
					assert.Nil(t, input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					assert.Equal(t, []string{"Title", "Body", "Category"}, options)
					return []int{0}, nil
				},
				InputFunc: func(prompt, defaultValue string) (string, error) {
					assert.Equal(t, "Original title", defaultValue)
					return "New title", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty interactive select body",
			isTTY: true,
			opts:  EditOptions{Interactive: true},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					require.NotNil(t, input.Body)
					assert.Equal(t, "New body text", *input.Body)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return []int{1}, nil // body is index 1
				},
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					assert.Equal(t, "Original body", defaultValue)
					return "New body text", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty interactive select category",
			isTTY: true,
			opts:  EditOptions{Interactive: true},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					assert.Nil(t, input.Body)
					require.NotNil(t, input.CategoryID)
					assert.Equal(t, "CAT2", *input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return []int{2}, nil // category is index 2
				},
				SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
					assert.Equal(t, "General", defaultValue)
					assert.Equal(t, []string{"General", "Q&A", "Show and tell"}, options)
					return 1, nil // select "Q&A"
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty interactive nothing selected is a no-op",
			isTTY: true,
			opts:  EditOptions{Interactive: true},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return []int{}, nil
				},
			},
			wantErr: "CancelError",
		},
		{
			name:            "success non-tty body-file",
			bodyFileContent: "Body from file",
			opts: EditOptions{
				BodyProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					require.NotNil(t, input.Body)
					assert.Equal(t, "Body from file", *input.Body)
					assert.Nil(t, input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty interactive blank title returns error",
			isTTY: true,
			opts:  EditOptions{Interactive: true},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MultiSelectFunc: func(prompt string, defaults []string, options []string) ([]int, error) {
					return []int{0}, nil
				},
				InputFunc: func(prompt, defaultValue string) (string, error) {
					return "   ", nil
				},
			},
			wantErr: "title cannot be blank",
		},
		{
			name:         "success non-tty body-file from stdin",
			stdinContent: "Body from stdin",
			opts: EditOptions{
				BodyFile:     "-",
				BodyProvided: true,
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.UpdateFunc = func(repo ghrepo.Interface, input client.UpdateDiscussionInput) (*client.Discussion, error) {
					assert.Nil(t, input.Title)
					require.NotNil(t, input.Body)
					assert.Equal(t, "Body from stdin", *input.Body)
					assert.Nil(t, input.CategoryID)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(tt.isTTY)
			ios.SetStdinTTY(tt.isTTY)

			if tt.stdinContent != "" {
				stdin.WriteString(tt.stdinContent)
			}

			mockClient := &client.DiscussionClientMock{}
			if tt.setupMock != nil {
				tt.setupMock(mockClient)
			}

			opts := tt.opts
			if tt.bodyFileContent != "" {
				dir := t.TempDir()
				f := filepath.Join(dir, "body.md")
				require.NoError(t, os.WriteFile(f, []byte(tt.bodyFileContent), 0600))
				opts.BodyFile = f
			}
			opts.IO = ios
			opts.BaseRepo = func() (ghrepo.Interface, error) {
				return ghrepo.New("OWNER", "REPO"), nil
			}
			opts.Client = func() (client.DiscussionClient, error) {
				return mockClient, nil
			}
			if tt.prompter != nil {
				opts.Prompter = tt.prompter
			}

			err := editRun(&opts)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, stdout.String())
		})
	}
}

func sampleCategories() []client.DiscussionCategory {
	return []client.DiscussionCategory{
		{ID: "CAT1", Name: "General", Slug: "general"},
		{ID: "CAT2", Name: "Q&A", Slug: "q-a"},
		{ID: "CAT3", Name: "Show and tell", Slug: "show-and-tell"},
	}
}

func sampleDiscussion() *client.Discussion {
	return &client.Discussion{
		ID:     "D_1",
		Number: 5,
		Title:  "Original title",
		Body:   "Original body",
		URL:    "https://github.com/OWNER/REPO/discussions/5",
		Category: client.DiscussionCategory{
			ID:   "CAT1",
			Name: "General",
			Slug: "general",
		},
	}
}
