package login

import (
	"fmt"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type tinyConfig map[string]string

func (c tinyConfig) ActiveTokenWithRefresh(host string) (gh.Credential, gh.RefreshStatus, error) {
	status := gh.RefreshStatusInapplicable
	switch c["_refresh_status"] {
	case "expired":
		status = gh.RefreshStatusExpired
	}
	return gh.Credential{
		Token:        c[fmt.Sprintf("%s:%s", host, "oauth_token")],
		Source:       c["_source"],
		RefreshToken: c["_refresh_token"],
	}, status, nil
}

func (c tinyConfig) ActiveUser(host string) (string, error) {
	return c[fmt.Sprintf("%s:%s", host, "user")], nil
}

func Test_helperRun(t *testing.T) {
	tests := []struct {
		name       string
		opts       CredentialOptions
		input      string
		wantStdout string
		wantStderr string
		wantErr    bool
	}{
		{
			name: "host only, credentials found",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "host plus user",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "gist host",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                "/Users/monalisa/.config/gh/hosts.yml",
						"github.com:user":        "monalisa",
						"github.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=gist.github.com
				username=monalisa
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=gist.github.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "url input",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				url=https://monalisa@example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "host only, no credentials found",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user": "monalisa",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr:    true,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "user mismatch",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=hubot
			`),
			wantErr:    true,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "no username configured",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=x-access-token
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "token from env",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "GITHUB_ENTERPRISE_TOKEN",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=hubot
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=x-access-token
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "refreshable token, authtype capability present",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"_refresh_token":          "RTOKEN",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				capability[]=authtype
				protocol=https
				host=example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				capability[]=authtype
				protocol=https
				host=example.com
				username=monalisa
				authtype=Basic
				credential=bW9uYWxpc2E6T1RPS0VO
				ephemeral=1
			`),
			wantStderr: "",
		},
		{
			name: "refreshable token, authtype capability absent",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"_refresh_token":          "RTOKEN",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "WARNING: gh: this git version cannot mark the short-lived token as non-cacheable; upgrade to git 2.46 or newer, or avoid a credential caching helper, to prevent a stale token being reused\n",
		},
		{
			name: "non-refreshable token, authtype capability present",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                 "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":        "monalisa",
						"example.com:oauth_token": "OTOKEN",
					}, nil
				},
			},
			input: heredoc.Doc(`
				capability[]=authtype
				protocol=https
				host=example.com
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=monalisa
				password=OTOKEN
			`),
			wantStderr: "",
		},
		{
			name: "expired refresh token",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":          "/Users/monalisa/.config/gh/hosts.yml",
						"_refresh_status":  "expired",
						"example.com:user": "monalisa",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr:    true,
			wantStdout: "",
			wantStderr: "the token for example.com has expired; please run 'gh auth login' to re-authenticate\n",
		},
		{
			name: "noop store operation",
			opts: CredentialOptions{
				Operation: "store",
			},
		},
		{
			name: "noop erase operation",
			opts: CredentialOptions{
				Operation: "erase",
			},
		},
		{
			name: "unknown operation",
			opts: CredentialOptions{
				Operation: "unknown",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, stdin, stdout, stderr := iostreams.Test()
			fmt.Fprint(stdin, tt.input)
			opts := &tt.opts
			opts.IO = ios
			if err := helperRun(opts); (err != nil) != tt.wantErr {
				t.Fatalf("helperRun() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantStdout != stdout.String() {
				t.Errorf("stdout: got %q, wants %q", stdout.String(), tt.wantStdout)
			}
			if tt.wantStderr != stderr.String() {
				t.Errorf("stderr: got %q, wants %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}
