package shared

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/internal/run"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/ssh"
	"github.com/stretchr/testify/assert"
)

type tinyConfig map[string]string

func (c tinyConfig) Login(host, username, token, gitProtocol string, encrypt bool) error {
	c[fmt.Sprintf("%s:%s", host, "user")] = username
	c[fmt.Sprintf("%s:%s", host, "oauth_token")] = token
	c[fmt.Sprintf("%s:%s", host, "git_protocol")] = gitProtocol
	return nil
}

func TestLogin_ssh(t *testing.T) {
	dir := t.TempDir()
	ios, _, stdout, stderr := iostreams.Test()

	tr := httpmock.Registry{}
	defer tr.Verify(t)

	tr.Register(
		httpmock.REST("GET", "api/v3/"),
		httpmock.ScopesResponder("repo,read:org"))
	tr.Register(
		httpmock.GraphQL(`query UserCurrent\b`),
		httpmock.StringResponse(`{"data":{"viewer":{ "login": "monalisa" }}}`))
	tr.Register(
		httpmock.REST("GET", "api/v3/user/keys"),
		httpmock.StringResponse(`[]`))
	tr.Register(
		httpmock.REST("POST", "api/v3/user/keys"),
		httpmock.StringResponse(`{}`))

	pm := &prompter.PrompterMock{}
	pm.SelectFunc = func(prompt, _ string, opts []string) (int, error) {
		switch prompt {
		case "What is your preferred protocol for Git operations?":
			return prompter.IndexFor(opts, "SSH")
		case "How would you like to authenticate GitHub CLI?":
			return prompter.IndexFor(opts, "Paste an authentication token")
		}
		return -1, prompter.NoSuchPromptErr(prompt)
	}
	pm.PasswordFunc = func(_ string) (string, error) {
		return "monkey", nil
	}
	pm.ConfirmFunc = func(prompt string, _ bool) (bool, error) {
		return true, nil
	}
	pm.AuthTokenFunc = func() (string, error) {
		return "ATOKEN", nil
	}
	pm.InputFunc = func(_, _ string) (string, error) {
		return "Test Key", nil
	}

	rs, runRestore := run.Stub()
	defer runRestore(t)

	keyFile := filepath.Join(dir, "id_ed25519")
	rs.Register(`ssh-keygen`, 0, "", func(args []string) {
		expected := []string{
			"ssh-keygen", "-t", "ed25519",
			"-C", "",
			"-N", "monkey",
			"-f", keyFile,
		}
		assert.Equal(t, expected, args)
		// simulate that the public key file has been generated
		_ = os.WriteFile(keyFile+".pub", []byte("PUBKEY asdf"), 0600)
	})

	cfg := tinyConfig{}

	err := Login(&LoginOptions{
		IO:          ios,
		Config:      &cfg,
		Prompter:    pm,
		HTTPClient:  &http.Client{Transport: &tr},
		Hostname:    "example.com",
		Interactive: true,
		sshContext: ssh.Context{
			ConfigDir: dir,
			KeygenExe: "ssh-keygen",
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, heredoc.Docf(`
		Tip: you can generate a Personal Access Token here https://example.com/settings/tokens
		The minimum required scopes are 'repo', 'read:org', 'admin:public_key'.
		- gh config set -h example.com git_protocol ssh
		✓ Configured git protocol
		✓ Uploaded the SSH key to your GitHub account: %s.pub
		✓ Logged in as monalisa
	`, keyFile), stderr.String())

	assert.Equal(t, "monalisa", cfg["example.com:user"])
	assert.Equal(t, "ATOKEN", cfg["example.com:oauth_token"])
	assert.Equal(t, "ssh", cfg["example.com:git_protocol"])
}

func Test_scopesSentence(t *testing.T) {
	type args struct {
		scopes       []string
		isEnterprise bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic scopes",
			args: args{
				scopes:       []string{"repo", "read:org"},
				isEnterprise: false,
			},
			want: "'repo', 'read:org'",
		},
		{
			name: "empty",
			args: args{
				scopes:       []string(nil),
				isEnterprise: false,
			},
			want: "",
		},
		{
			name: "workflow scope for dotcom",
			args: args{
				scopes:       []string{"repo", "workflow"},
				isEnterprise: false,
			},
			want: "'repo', 'workflow'",
		},
		{
			name: "workflow scope for GHE",
			args: args{
				scopes:       []string{"repo", "workflow"},
				isEnterprise: true,
			},
			want: "'repo', 'workflow' (GHE 3.0+)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scopesSentence(tt.args.scopes, tt.args.isEnterprise); got != tt.want {
				t.Errorf("scopesSentence() = %q, want %q", got, tt.want)
			}
		})
	}
}
