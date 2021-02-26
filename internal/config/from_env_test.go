package config

import (
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/pkg/env"
	"github.com/stretchr/testify/assert"
)

func TestInheritEnv(t *testing.T) {
	type wants struct {
		hosts     []string
		token     string
		source    string
		writeable bool
	}

	tests := []struct {
		name                    string
		baseConfig              string
		GITHUB_TOKEN            string
		GITHUB_ENTERPRISE_TOKEN string
		GH_TOKEN                string
		GH_ENTERPRISE_TOKEN     string
		hostname                string
		wants                   wants
	}{
		{
			name:       "blank",
			baseConfig: ``,
			hostname:   "github.com",
			wants: wants{
				hosts:     []string(nil),
				token:     "",
				source:    ".config.gh.config.yml",
				writeable: true,
			},
		},
		{
			name:         "GITHUB_TOKEN over blank config",
			baseConfig:   ``,
			GITHUB_TOKEN: "OTOKEN",
			hostname:     "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "OTOKEN",
				source:    "GITHUB_TOKEN",
				writeable: false,
			},
		},
		{
			name:       "GH_TOKEN over blank config",
			baseConfig: ``,
			GH_TOKEN:   "OTOKEN",
			hostname:   "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "OTOKEN",
				source:    "GH_TOKEN",
				writeable: false,
			},
		},
		{
			name:         "GITHUB_TOKEN not applicable to GHE",
			baseConfig:   ``,
			GITHUB_TOKEN: "OTOKEN",
			hostname:     "example.org",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "",
				source:    ".config.gh.config.yml",
				writeable: true,
			},
		},
		{
			name:       "GH_TOKEN not applicable to GHE",
			baseConfig: ``,
			GH_TOKEN:   "OTOKEN",
			hostname:   "example.org",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "",
				source:    ".config.gh.config.yml",
				writeable: true,
			},
		},
		{
			name:                    "GITHUB_ENTERPRISE_TOKEN over blank config",
			baseConfig:              ``,
			GITHUB_ENTERPRISE_TOKEN: "ENTOKEN",
			hostname:                "example.org",
			wants: wants{
				hosts:     []string(nil),
				token:     "ENTOKEN",
				source:    "GITHUB_ENTERPRISE_TOKEN",
				writeable: false,
			},
		},
		{
			name:                "GH_ENTERPRISE_TOKEN over blank config",
			baseConfig:          ``,
			GH_ENTERPRISE_TOKEN: "ENTOKEN",
			hostname:            "example.org",
			wants: wants{
				hosts:     []string(nil),
				token:     "ENTOKEN",
				source:    "GH_ENTERPRISE_TOKEN",
				writeable: false,
			},
		},
		{
			name: "token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  github.com:
			    oauth_token: OTOKEN
			`),
			hostname: "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "OTOKEN",
				source:    ".config.gh.hosts.yml",
				writeable: true,
			},
		},
		{
			name: "GITHUB_TOKEN shadows token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  github.com:
			    oauth_token: OTOKEN
			`),
			GITHUB_TOKEN: "ENVTOKEN",
			hostname:     "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "ENVTOKEN",
				source:    "GITHUB_TOKEN",
				writeable: false,
			},
		},
		{
			name: "GH_TOKEN shadows token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  github.com:
			    oauth_token: OTOKEN
			`),
			GH_TOKEN: "ENVTOKEN",
			hostname: "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "ENVTOKEN",
				source:    "GH_TOKEN",
				writeable: false,
			},
		},
		{
			name: "GITHUB_ENTERPRISE_TOKEN shadows token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  example.org:
			    oauth_token: OTOKEN
			`),
			GITHUB_ENTERPRISE_TOKEN: "ENVTOKEN",
			hostname:                "example.org",
			wants: wants{
				hosts:     []string{"example.org"},
				token:     "ENVTOKEN",
				source:    "GITHUB_ENTERPRISE_TOKEN",
				writeable: false,
			},
		},
		{
			name: "GH_ENTERPRISE_TOKEN shadows token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  example.org:
			    oauth_token: OTOKEN
			`),
			GH_ENTERPRISE_TOKEN: "ENVTOKEN",
			hostname:            "example.org",
			wants: wants{
				hosts:     []string{"example.org"},
				token:     "ENVTOKEN",
				source:    "GH_ENTERPRISE_TOKEN",
				writeable: false,
			},
		},
		{
			name:         "GH_TOKEN shadows token from GITHUB_TOKEN",
			baseConfig:   ``,
			GH_TOKEN:     "GHTOKEN",
			GITHUB_TOKEN: "GITHUBTOKEN",
			hostname:     "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "GHTOKEN",
				source:    "GH_TOKEN",
				writeable: false,
			},
		},
		{
			name:                    "GH_ENTERPRISE_TOKEN shadows token from GITHUB_ENTERPRISE_TOKEN",
			baseConfig:              ``,
			GH_ENTERPRISE_TOKEN:     "GHTOKEN",
			GITHUB_ENTERPRISE_TOKEN: "GITHUBTOKEN",
			hostname:                "example.org",
			wants: wants{
				hosts:     []string(nil),
				token:     "GHTOKEN",
				source:    "GH_ENTERPRISE_TOKEN",
				writeable: false,
			},
		},
		{
			name: "GITHUB_TOKEN adds host entry",
			baseConfig: heredoc.Doc(`
			hosts:
			  example.org:
			    oauth_token: OTOKEN
			`),
			GITHUB_TOKEN: "ENVTOKEN",
			hostname:     "github.com",
			wants: wants{
				hosts:     []string{"github.com", "example.org"},
				token:     "ENVTOKEN",
				source:    "GITHUB_TOKEN",
				writeable: false,
			},
		},
		{
			name: "GH_TOKEN adds host entry",
			baseConfig: heredoc.Doc(`
			hosts:
			  example.org:
			    oauth_token: OTOKEN
			`),
			GH_TOKEN: "ENVTOKEN",
			hostname: "github.com",
			wants: wants{
				hosts:     []string{"github.com", "example.org"},
				token:     "ENVTOKEN",
				source:    "GH_TOKEN",
				writeable: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(env.WithEnv(map[string]string{
				"GITHUB_TOKEN":            tt.GITHUB_TOKEN,
				"GITHUB_ENTERPRISE_TOKEN": tt.GITHUB_ENTERPRISE_TOKEN,
				"GH_TOKEN":                tt.GH_TOKEN,
				"GH_ENTERPRISE_TOKEN":     tt.GH_ENTERPRISE_TOKEN,
			}))

			baseCfg := NewFromString(tt.baseConfig)
			cfg := InheritEnv(baseCfg)

			hosts, _ := cfg.Hosts()
			assert.Equal(t, tt.wants.hosts, hosts)

			val, source, _ := cfg.GetWithSource(tt.hostname, "oauth_token")
			assert.Equal(t, tt.wants.token, val)
			assert.Regexp(t, tt.wants.source, source)

			val, _ = cfg.Get(tt.hostname, "oauth_token")
			assert.Equal(t, tt.wants.token, val)

			err := cfg.CheckWriteable(tt.hostname, "oauth_token")
			if tt.wants.writeable != (err == nil) {
				t.Errorf("CheckWriteable() = %v, wants %v", err, tt.wants.writeable)
			}
		})
	}
}

func TestAuthTokenProvidedFromEnv(t *testing.T) {
	tests := []struct {
		name                    string
		GITHUB_TOKEN            string
		GITHUB_ENTERPRISE_TOKEN string
		GH_TOKEN                string
		GH_ENTERPRISE_TOKEN     string
		provided                bool
	}{
		{
			name:     "no env tokens",
			provided: false,
		},
		{
			name:     "GH_TOKEN",
			GH_TOKEN: "TOKEN",
			provided: true,
		},
		{
			name:         "GITHUB_TOKEN",
			GITHUB_TOKEN: "TOKEN",
			provided:     true,
		},
		{
			name:                "GH_ENTERPRISE_TOKEN",
			GH_ENTERPRISE_TOKEN: "TOKEN",
			provided:            true,
		},
		{
			name:                    "GITHUB_ENTERPRISE_TOKEN",
			GITHUB_ENTERPRISE_TOKEN: "TOKEN",
			provided:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(env.WithEnv(map[string]string{
				"GITHUB_TOKEN":            tt.GITHUB_TOKEN,
				"GITHUB_ENTERPRISE_TOKEN": tt.GITHUB_ENTERPRISE_TOKEN,
				"GH_TOKEN":                tt.GH_TOKEN,
				"GH_ENTERPRISE_TOKEN":     tt.GH_ENTERPRISE_TOKEN,
			}))
			assert.Equal(t, tt.provided, AuthTokenProvidedFromEnv())
		})
	}
}
