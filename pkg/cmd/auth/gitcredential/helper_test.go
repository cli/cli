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
	token := c[fmt.Sprintf("%s:%s:oauth_token", hostname, user)]
	if token == "" {
		return "", "", fmt.Errorf("no token found for '%s'", user)
	}
	return token, c["_source"], nil
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
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":                 "monalisa",
						"example.com:oauth_token":          "OTOKEN",
						"example.com:monalisa:oauth_token": "OTOKEN",
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
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":                 "monalisa",
						"example.com:oauth_token":          "OTOKEN",
						"example.com:monalisa:oauth_token": "OTOKEN",
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
						"_source":                         "/Users/monalisa/.config/gh/hosts.yml",
						"github.com:user":                 "monalisa",
						"github.com:oauth_token":          "OTOKEN",
						"github.com:monalisa:oauth_token": "OTOKEN",
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
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":                 "monalisa",
						"example.com:oauth_token":          "OTOKEN",
						"example.com:monalisa:oauth_token": "OTOKEN",
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
			name: "user mismatch (requested user has no token)",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:user":                 "monalisa",
						"example.com:monalisa:oauth_token": "OTOKEN",
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
			name: "specific user requested, token found",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:monalisa:oauth_token": "OTOKEN_MONALISA",
						"example.com:hubot:oauth_token":    "OTOKEN_HUBOT",
						"example.com:user":                 "monalisa", // active user is monalisa
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
				username=hubot
				password=OTOKEN_HUBOT
			`),
			wantStderr: "",
		},
		{
			name: "specific user requested, token not found",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                          "/Users/monalisa/.config/gh/hosts.yml",
						"example.com:monalisa:oauth_token": "OTOKEN_MONALISA",
						"example.com:user":                 "monalisa", // active user is monalisa
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=example.com
				username=nonexistent
			`),
			wantErr:    true,
			wantStdout: "",
			wantStderr: "",
		},
		{
			name: "specific user for gist host",
			opts: CredentialOptions{
				Operation: "get",
				Config: func() (config, error) {
					return tinyConfig{
						"_source":                         "/Users/monalisa/.config/gh/hosts.yml",
						"github.com:monalisa:oauth_token": "OTOKEN_MONALISA",
						"github.com:hubot:oauth_token":    "OTOKEN_HUBOT",
						"github.com:user":                 "monalisa", // active user is monalisa
					}, nil
				},
			},
			input: heredoc.Doc(`
				protocol=https
				host=gist.github.com
				username=hubot
			`),
			wantErr: false,
			wantStdout: heredoc.Doc(`
				protocol=https
				host=gist.github.com
				username=hubot
				password=OTOKEN_HUBOT
			`),
			wantStderr: "",
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
