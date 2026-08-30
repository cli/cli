package comment

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdComment(t *testing.T) {
	tests := []struct {
		name         string
		args         string
		isTTY        bool
		wantOpts     CommentOptions
		wantBaseRepo ghrepo.Interface
		wantErr      string
	}{
		{
			name:  "add comment with body",
			args:  "123 --body 'Hello world'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 123},
				Body:      "Hello world",
			},
		},
		{
			name:  "reply to comment by node ID",
			args:  "DC_abc --body 'Reply text'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_abc"},
				Body:      "Reply text",
			},
		},
		{
			name:  "reply to comment by comment URL",
			args:  "https://github.com/OWNER/REPO/discussions/5#discussioncomment-999 --body 'Reply text'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{
					Number:            5,
					CommentDatabaseID: 999,
				},
				Body: "Reply text",
			},
			wantBaseRepo: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
		},
		{
			name:  "edit comment by node ID",
			args:  "DC_abc --edit --body 'Updated'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_abc"},
				Edit:      true,
				Body:      "Updated",
			},
		},
		{
			name:  "edit comment by comment URL",
			args:  "https://github.com/OWNER/REPO/discussions/5#discussioncomment-999 --edit --body 'Updated'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{
					Number:            5,
					CommentDatabaseID: 999,
				},
				Edit: true,
				Body: "Updated",
			},
			wantBaseRepo: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
		},
		{
			name:  "delete comment by node ID",
			args:  "DC_abc --delete --yes",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_abc"},
				Delete:    true,
				Yes:       true,
			},
		},
		{
			name:  "delete comment by comment URL",
			args:  "https://github.com/OWNER/REPO/discussions/5#discussioncomment-999 --delete --yes",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{
					Number:            5,
					CommentDatabaseID: 999,
				},
				Delete: true,
				Yes:    true,
			},
			wantBaseRepo: ghrepo.NewWithHost("OWNER", "REPO", "github.com"),
		},
		{
			name:  "discussion URL as argument",
			args:  "https://github.com/OTHER/REPO2/discussions/42 --body 'comment'",
			isTTY: true,
			wantOpts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 42},
				Body:      "comment",
			},
			wantBaseRepo: ghrepo.NewWithHost("OTHER", "REPO2", "github.com"),
		},
		{
			name:    "mutual exclusion edit and delete",
			args:    "DC_abc --edit --delete",
			isTTY:   true,
			wantErr: "specify only one of --edit or --delete",
		},
		{
			name:    "mutual exclusion body and body-file",
			args:    "123 --body 'inline' --body-file body.md",
			isTTY:   true,
			wantErr: "specify only one of --body or --body-file",
		},
		{
			name:    "delete with body is invalid",
			args:    "DC_abc --delete --body 'text'",
			isTTY:   true,
			wantErr: "--delete cannot be combined with --body or --body-file",
		},
		{
			name:    "delete with body is invalid",
			args:    "DC_abc --delete --body-file /some/path",
			isTTY:   true,
			wantErr: "--delete cannot be combined with --body or --body-file",
		},
		{
			name:    "yes without delete is invalid",
			args:    "123 --yes --body 'text'",
			isTTY:   true,
			wantErr: "--yes can only be used with --delete",
		},
		{
			name:    "edit requires comment arg but given discussion number",
			args:    "123 --edit --body 'text'",
			isTTY:   true,
			wantErr: "--edit and --delete require a comment ID or comment URL",
		},
		{
			name:    "edit requires comment arg but given discussion URL",
			args:    "https://github.com/OWNER/REPO/discussions/123 --edit --body 'text'",
			isTTY:   true,
			wantErr: "--edit and --delete require a comment ID or comment URL",
		},
		{
			name:    "delete requires comment arg but given discussion number",
			args:    "123 --delete --yes",
			isTTY:   true,
			wantErr: "--edit and --delete require a comment ID or comment URL",
		},
		{
			name:    "delete requires comment arg but given discussion URL",
			args:    "https://github.com/OWNER/REPO/discussions/123 --delete --yes",
			isTTY:   true,
			wantErr: "--edit and --delete require a comment ID or comment URL",
		},
		{
			name:    "no body non-tty is error",
			args:    "123",
			isTTY:   false,
			wantErr: "--body or --body-file is required when not running interactively",
		},
		{
			name:    "delete without yes non-tty is error",
			args:    "DC_abc --delete",
			isTTY:   false,
			wantErr: "--yes is required when not running interactively with --delete",
		},
		{
			name:    "no args",
			args:    "",
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
			var gotOpts *CommentOptions
			cmd := NewCmdComment(f, func(opts *CommentOptions) error {
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
			require.NotNil(t, gotOpts.ParsedArg)
			if tt.wantOpts.ParsedArg != nil {
				assert.Equal(t, tt.wantOpts.ParsedArg.Number, gotOpts.ParsedArg.Number)
				assert.Equal(t, tt.wantOpts.ParsedArg.CommentNodeID, gotOpts.ParsedArg.CommentNodeID)
				assert.Equal(t, tt.wantOpts.ParsedArg.CommentDatabaseID, gotOpts.ParsedArg.CommentDatabaseID)
			}
			assert.Equal(t, tt.wantOpts.Body, gotOpts.Body)
			assert.Equal(t, tt.wantOpts.Edit, gotOpts.Edit)
			assert.Equal(t, tt.wantOpts.Delete, gotOpts.Delete)
			assert.Equal(t, tt.wantOpts.Yes, gotOpts.Yes)

			if tt.wantBaseRepo != nil {
				baseRepo, err := gotOpts.BaseRepo()
				require.NoError(t, err)
				assert.True(t, ghrepo.IsSame(tt.wantBaseRepo, baseRepo))
			}
		})
	}
}

func TestCommentRun(t *testing.T) {
	tests := []struct {
		name            string
		opts            CommentOptions
		bodyFileContent string
		stdinContent    string
		isTTY           bool
		setupMock       func(*testing.T, *client.DiscussionClientMock)
		prompter        *prompter.PrompterMock
		wantErr         string
		wantOut         string
	}{
		{
			name: "non-tty add comment with body",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
				Body:      "Hello world",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					assert.Equal(t, int32(5), number)
					return sampleDiscussion(), nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "D_1", discussionID)
					assert.Equal(t, "Hello world", body)
					assert.Equal(t, "", replyToID)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty add comment with body-file",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
			},
			bodyFileContent: "Body from file",
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "Body from file", body)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty add comment with body-file from stdin",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
				BodyFile:  "-",
			},
			stdinContent: "Body from stdin",
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "Body from stdin", body)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name:  "tty add comment interactive editor",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "Editor body", body)
					return sampleComment(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					assert.Equal(t, "", defaultValue)
					return "Editor body", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty reply to comment by node ID",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_parent"},
				Body:      "Reply text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return &client.DiscussionComment{
						ID:           "DC_parent",
						URL:          "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1",
						DiscussionID: "D_1",
						Body:         "Parent comment",
					}, nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "D_1", discussionID)
					assert.Equal(t, "Reply text", body)
					assert.Equal(t, "DC_parent", replyToID)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty reply via comment URL",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{
					Number:            5,
					CommentDatabaseID: 17196842,
				},
				Body: "Reply text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ResolveCommentNodeIDFunc = func(repo ghrepo.Interface, commentDatabaseID int64) (string, error) {
					assert.Equal(t, int64(17196842), commentDatabaseID)
					return "DC_resolved", nil
				}
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_resolved", commentID)
					return &client.DiscussionComment{
						ID:           "DC_resolved",
						DiscussionID: "D_1",
					}, nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "D_1", discussionID)
					assert.Equal(t, "DC_resolved", replyToID)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty edit comment via node ID with body",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Edit:      true,
				Body:      "Updated body",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_1", commentID)
					return sampleComment(), nil
				}
				m.UpdateCommentFunc = func(repo ghrepo.Interface, commentID, body string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_1", commentID)
					assert.Equal(t, "Updated body", body)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty edit comment via comment URL with body",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5, CommentDatabaseID: 999},
				Edit:      true,
				Body:      "Updated body",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ResolveCommentNodeIDFunc = func(repo ghrepo.Interface, commentDatabaseID int64) (string, error) {
					assert.Equal(t, int64(999), commentDatabaseID)
					return "DC_resolved", nil
				}
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_resolved", commentID)
					return sampleComment(), nil
				}
				m.UpdateCommentFunc = func(repo ghrepo.Interface, commentID, body string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_resolved", commentID)
					assert.Equal(t, "Updated body", body)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name: "non-tty edit comment with body-file from stdin",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Edit:      true,
				BodyFile:  "-",
			},
			stdinContent: "Edited from stdin",
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.UpdateCommentFunc = func(repo ghrepo.Interface, commentID, body string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_1", commentID)
					assert.Equal(t, "Edited from stdin", body)
					return sampleComment(), nil
				}
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name:  "tty edit comment via node ID interactive editor pre-populates",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Edit:      true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.UpdateCommentFunc = func(repo ghrepo.Interface, commentID, body string) (*client.DiscussionComment, error) {
					assert.Equal(t, "Edited in editor", body)
					return sampleComment(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				MarkdownEditorFunc: func(prompt, defaultValue string, blankAllowed bool) (string, error) {
					assert.Equal(t, "Original comment body", defaultValue)
					return "Edited in editor", nil
				},
			},
			wantOut: "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1\n",
		},
		{
			name:  "tty delete comment via node ID with confirmation",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Delete:    true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.DeleteCommentFunc = func(repo ghrepo.Interface, commentID string) error {
					assert.Equal(t, "DC_1", commentID)
					return nil
				}
			},
			prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
					assert.False(t, defaultValue)
					return true, nil
				},
			},
			wantOut: "",
		},
		{
			name:  "tty delete comment via comment URL with confirmation",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5, CommentDatabaseID: 999},
				Delete:    true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.ResolveCommentNodeIDFunc = func(repo ghrepo.Interface, commentDatabaseID int64) (string, error) {
					return "DC_resolved", nil
				}
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					assert.Equal(t, "DC_resolved", commentID)
					return sampleComment(), nil
				}
				m.DeleteCommentFunc = func(repo ghrepo.Interface, commentID string) error {
					assert.Equal(t, "DC_resolved", commentID)
					return nil
				}
			},
			prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
					return true, nil
				},
			},
			wantOut: "",
		},
		{
			name:  "tty delete comment with --yes skips prompt",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Delete:    true,
				Yes:       true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.DeleteCommentFunc = func(repo ghrepo.Interface, commentID string) error {
					return nil
				}
			},
			wantOut: "",
		},
		{
			name:  "tty delete comment declined",
			isTTY: true,
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Delete:    true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
			},
			prompter: &prompter.PrompterMock{
				ConfirmFunc: func(prompt string, defaultValue bool) (bool, error) {
					return false, nil
				},
			},
			wantErr: "CancelError",
		},
		{
			name: "GetByNumber error",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
				Body:      "text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return nil, fmt.Errorf("not found")
				}
			},
			wantErr: "not found",
		},
		{
			name: "AddComment error",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{Number: 5},
				Body:      "text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetByNumberFunc = func(repo ghrepo.Interface, number int32) (*client.Discussion, error) {
					return sampleDiscussion(), nil
				}
				m.AddCommentFunc = func(repo ghrepo.Interface, discussionID, body, replyToID string) (*client.DiscussionComment, error) {
					return nil, fmt.Errorf("mutation failed")
				}
			},
			wantErr: "mutation failed",
		},
		{
			name: "UpdateComment mutation error",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Edit:      true,
				Body:      "text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.UpdateCommentFunc = func(repo ghrepo.Interface, commentID, body string) (*client.DiscussionComment, error) {
					return nil, fmt.Errorf("update mutation failed")
				}
			},
			wantErr: "update mutation failed",
		},
		{
			name: "DeleteComment mutation error",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_1"},
				Delete:    true,
				Yes:       true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return sampleComment(), nil
				}
				m.DeleteCommentFunc = func(repo ghrepo.Interface, commentID string) error {
					return fmt.Errorf("delete mutation failed")
				}
			},
			wantErr: "delete mutation failed",
		},
		{
			name: "GetComment error on edit",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_bad"},
				Edit:      true,
				Body:      "text",
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return nil, fmt.Errorf("comment not found")
				}
			},
			wantErr: "comment not found",
		},
		{
			name: "GetComment error on delete",
			opts: CommentOptions{
				ParsedArg: &shared.ParsedDiscussionOrCommentArg{CommentNodeID: "DC_bad"},
				Delete:    true,
				Yes:       true,
			},
			setupMock: func(t *testing.T, m *client.DiscussionClientMock) {
				m.GetCommentFunc = func(host string, commentID string) (*client.DiscussionComment, error) {
					return nil, fmt.Errorf("comment not found")
				}
			},
			wantErr: "comment not found",
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
				tt.setupMock(t, mockClient)
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

			err := commentRun(&opts)
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

func sampleDiscussion() *client.Discussion {
	return &client.Discussion{
		ID:     "D_1",
		Number: 5,
		Title:  "Sample discussion",
		URL:    "https://github.com/OWNER/REPO/discussions/5",
	}
}

func sampleComment() *client.DiscussionComment {
	return &client.DiscussionComment{
		ID:   "DC_1",
		URL:  "https://github.com/OWNER/REPO/discussions/5#discussioncomment-1",
		Body: "Original comment body",
	}
}
