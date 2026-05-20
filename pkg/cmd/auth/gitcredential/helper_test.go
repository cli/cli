package login

import (
	"fmt"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/pkg/iostreams"
)

type tinyConfig map[string]string

func (c tinyConfig) ActiveToken(host string) (string, string) {
	return c[fmt.Sprintf("%s:%s", host, "oauth_token")], c["_source"]
}

func (c tinyConfig) ActiveUser(host string) (string, error) {
	return c[fmt.Sprintf("%s:%s", host, "user")], nil
}

func (c tinyConfig) TokenForUser(hostname, user string) (string, string, error) {
	key := fmt.Sprintf("%s:users:%s:oauth_token", hostname, user)
	if token, ok := c[key]; ok {
		return token, "oauth_token", nil
	}
	return "", "", fmt.Errorf("no token for %s on %s", user, hostname)
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
		{
			name: "account with existing token",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{
						"example.com:users:user2:oauth_token": "TOKEN2",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=user2
				password=TOKEN2
			`),
			wantErr: false,
		},
		{
			name: "account token not found",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
			`),
			wantErr: true,
		},
		{
			name: "account matches requested username",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{
						"example.com:users:user2:oauth_token": "TOKEN2",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=user2
			`),
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=user2
				password=TOKEN2
			`),
			wantErr: false,
		},
		{
			name: "account conflicts with requested username",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{
						"example.com:users:user2:oauth_token": "TOKEN2",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=user1
			`),
			wantErr: true,
		},
		{
			name: "account with gist host",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{
						"github.com:users:user2:oauth_token": "TOKEN2",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=gist.github.com
			`),
			wantStdout: heredoc.Doc(`
				protocol=https
				host=gist.github.com
				username=user2
				password=TOKEN2
			`),
			wantErr: false,
		},
		{
			name: "account with x-access-token username is not a conflict",
			opts: CredentialOptions{
				Operation: "get",
				Account:   "user2",
				Config: func() (config, error) {
					return tinyConfig{
						"example.com:users:user2:oauth_token": "TOKEN2",
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=x-access-token
			`),
			wantStdout: heredoc.Doc(`
				protocol=https
				host=example.com
				username=user2
				password=TOKEN2
			`),
			wantErr: false,
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
