package create

import (
	"bytes"
	"fmt"
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

func TestNewCmdCreate(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		isTTY        bool
		wantOpts     CreateOptions
		wantBaseRepo ghrepo.Interface
		wantErr      string
	}{
		{
			name:     "no flags",
			args:     "",
			isTTY:    true,
			wantOpts: CreateOptions{},
		},
		{
			name:  "all flags",
			args:  "--title 'My question' --body 'Details' --category 'Q&A' --label bug,enhancement",
			isTTY: true,
			wantOpts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "Q&A",
				Labels:   []string{"bug", "enhancement"},
			},
		},
		{
			name:    "extra args",
			args:    "extra",
			isTTY:   true,
			wantErr: "unknown argument",
		},
		{
			name:    "missing required flags non-interactively",
			args:    "--title 'My question'",
			isTTY:   false,
			wantErr: "--title, --body (or --body-file), and --category are required when not running interactively",
		},
		{
			name:    "blank title",
			args:    "--title '   '",
			isTTY:   true,
			wantErr: "title cannot be blank",
		},
		{
			name:    "blank category",
			args:    "--category '   '",
			isTTY:   true,
			wantErr: "category cannot be blank",
		},
		{
			name:    "blank body",
			args:    "--body '   '",
			isTTY:   true,
			wantErr: "body cannot be blank",
		},
		{
			name:    "body and body-file mutually exclusive",
			args:    "--body 'text' --body-file file.md --title 'T' --category 'Q&A'",
			isTTY:   true,
			wantErr: "specify only one of --body or --body-file",
		},
		{
			name:  "body-file flag",
			args:  "--title 'T' --body-file 'file.md' --category 'Q&A'",
			isTTY: true,
			wantOpts: CreateOptions{
				Title:    "T",
				BodyFile: "file.md",
				Category: "Q&A",
			},
		},
		{
			name:         "repo override",
			args:         "--title 'Test' --body 'Body' --category 'Q&A' -R OWNER/REPO",
			isTTY:        true,
			wantBaseRepo: ghrepo.New("OWNER", "REPO"),
			wantOpts: CreateOptions{
				Title:    "Test",
				Body:     "Body",
				Category: "Q&A",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, _, _ := iostreams.Test()
			ios.SetStdinTTY(tt.isTTY)
			ios.SetStdoutTTY(tt.isTTY)
			f := &cmdutil.Factory{IOStreams: ios}
			var gotOpts *CreateOptions
			cmd := NewCmdCreate(f, func(opts *CreateOptions) error {
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
			assert.Equal(t, tt.wantOpts.Title, gotOpts.Title)
			assert.Equal(t, tt.wantOpts.Body, gotOpts.Body)
			assert.Equal(t, tt.wantOpts.BodyFile, gotOpts.BodyFile)
			assert.Equal(t, tt.wantOpts.Category, gotOpts.Category)
			assert.Equal(t, tt.wantOpts.Labels, gotOpts.Labels)

			if tt.wantBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				assert.True(t, ghrepo.IsSame(tt.wantBaseRepo, baseRepo))
			}
		})
	}
}

func TestCreateRun(t *testing.T) {
	tests := []struct {
		name         string
		opts         CreateOptions
		isTTY        bool
		stdinContent string
		setupMock    func(*client.DiscussionClientMock)
		prompter     *prompter.PrompterMock
		wantErr      string
		wantOut      string
	}{
		{
			name: "success non-tty",
			opts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "Q&A",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "CAT2", input.CategoryID)
					assert.Equal(t, "My question", input.Title)
					assert.Equal(t, "Details", input.Body)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "success non-tty with label",
			opts: CreateOptions{
				Title:    "Feature request",
				Body:     "Details",
				Category: "general",
				Labels:   []string{"enhancement", "bug"},
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.ListLabelsFunc = func(repo ghrepo.Interface) ([]client.DiscussionLabel, error) {
					return []client.DiscussionLabel{
						{ID: "L_bug", Name: "bug"},
						{ID: "L_enh", Name: "enhancement"},
					}, nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, []string{"L_enh", "L_bug"}, input.LabelIDs)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:         "success non-tty body-file from stdin",
			stdinContent: "Body from stdin",
			opts: CreateOptions{
				Title:    "My question",
				BodyFile: "-",
				Category: "Q&A",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "Body from stdin", input.Body)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name: "non-tty unknown category",
			opts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "nonexistent",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
			},
			wantErr: `unknown category: "nonexistent"`,
		},
		{
			name: "non-tty list categories query errors",
			opts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "General",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return nil, fmt.Errorf("network error")
				}
			},
			wantErr: "network error",
		},
		{
			name: "non-tty create mutation errors",
			opts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "General",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					return nil, fmt.Errorf("mutation failed")
				}
			},
			wantErr: "failed to create discussion: mutation failed",
		},
		{
			name:  "tty prompts for all fields",
			isTTY: true,
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "My question", input.Title)
					assert.Equal(t, "CAT1", input.CategoryID)
					assert.Equal(t, "Some body text", input.Body)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				InputFunc: func(prompt, defaultValue string) (string, error) {
					return "My question", nil
				},
				SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
					assert.Equal(t, []string{"General", "Q&A", "Show and tell"}, options)
					return 0, nil
				},
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					assert.False(t, blankAllowed, "body editor should not allow blank input")
					return "Some body text", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty does not prompt when all flags provided",
			isTTY: true,
			opts: CreateOptions{
				Title:    "My question",
				Body:     "Details",
				Category: "Q&A",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "CAT2", input.CategoryID)
					assert.Equal(t, "My question", input.Title)
					assert.Equal(t, "Details", input.Body)
					return sampleDiscussion(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty partial flags prompts only for missing category",
			isTTY: true,
			opts: CreateOptions{
				Title: "Pre-filled title",
				Body:  "Pre-filled body",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "Pre-filled title", input.Title)
					assert.Equal(t, "CAT2", input.CategoryID)
					assert.Equal(t, "Pre-filled body", input.Body)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				SelectFunc: func(prompt, defaultValue string, options []string) (int, error) {
					return 1, nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty partial flags prompts only for missing body",
			isTTY: true,
			opts: CreateOptions{
				Title:    "Pre-filled title",
				Category: "Q&A",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "Pre-filled title", input.Title)
					assert.Equal(t, "CAT2", input.CategoryID)
					assert.Equal(t, "Prompted body", input.Body)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					return "Prompted body", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty partial flags prompts only for missing title",
			isTTY: true,
			opts: CreateOptions{
				Body:     "Pre-filled body",
				Category: "General",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
				m.CreateFunc = func(repo ghrepo.Interface, input client.CreateDiscussionInput) (*client.Discussion, error) {
					assert.Equal(t, "Prompted title", input.Title)
					assert.Equal(t, "CAT1", input.CategoryID)
					assert.Equal(t, "Pre-filled body", input.Body)
					return sampleDiscussion(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				InputFunc: func(prompt, defaultValue string) (string, error) {
					return "Prompted title", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5\n",
		},
		{
			name:  "tty blank title returns error",
			isTTY: true,
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				InputFunc: func(prompt, defaultValue string) (string, error) {
					return "   ", nil
				},
			},
			wantErr: "title cannot be blank",
		},
		{
			name:  "tty blank body returns error",
			isTTY: true,
			opts: CreateOptions{
				Title:    "Valid title",
				Category: "General",
			},
			setupMock: func(m *client.DiscussionClientMock) {
				m.ListCategoriesFunc = func(repo ghrepo.Interface) ([]client.DiscussionCategory, error) {
					return sampleCategories(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					return "   ", nil
				},
			},
			wantErr: "body cannot be blank",
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
			tt.setupMock(mockClient)

			opts := tt.opts
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

			err := createRun(&opts)
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
		Number: 5,
		Title:  "My question",
		URL:    "https://github.com/OWNER/REPO/discussions/5",
	}
}
