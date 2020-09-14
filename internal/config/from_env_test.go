package config

import (
	"os"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/stretchr/testify/assert"
)

func TestInheritEnv(t *testing.T) {
	orig_GITHUB_TOKEN := os.Getenv("GITHUB_TOKEN")
	orig_GITHUB_ENTERPRISE_TOKEN := os.Getenv("GITHUB_ENTERPRISE_TOKEN")
	t.Cleanup(func() {
		os.Setenv("GITHUB_TOKEN", orig_GITHUB_TOKEN)
		os.Setenv("GITHUB_ENTERPRISE_TOKEN", orig_GITHUB_ENTERPRISE_TOKEN)
	})

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
		hostname                string
		wants                   wants
	}{
		{
			name:                    "blank",
			baseConfig:              ``,
			GITHUB_TOKEN:            "",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "github.com",
			wants: wants{
				hosts:     []string(nil),
				token:     "",
				source:    "~/.config/gh/config.yml",
				writeable: true,
			},
		},
		{
			name:                    "GITHUB_TOKEN over blank config",
			baseConfig:              ``,
			GITHUB_TOKEN:            "OTOKEN",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "OTOKEN",
				source:    "GITHUB_TOKEN",
				writeable: false,
			},
		},
		{
			name:                    "GITHUB_TOKEN not applicable to GHE",
			baseConfig:              ``,
			GITHUB_TOKEN:            "OTOKEN",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "example.org",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "",
				source:    "~/.config/gh/config.yml",
				writeable: true,
			},
		},
		{
			name:                    "GITHUB_ENTERPRISE_TOKEN over blank config",
			baseConfig:              ``,
			GITHUB_TOKEN:            "",
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
			name: "token from file",
			baseConfig: heredoc.Doc(`
			hosts:
			  github.com:
			    oauth_token: OTOKEN
			`),
			GITHUB_TOKEN:            "",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "OTOKEN",
				source:    "~/.config/gh/hosts.yml",
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
			GITHUB_TOKEN:            "ENVTOKEN",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "github.com",
			wants: wants{
				hosts:     []string{"github.com"},
				token:     "ENVTOKEN",
				source:    "GITHUB_TOKEN",
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
			GITHUB_TOKEN:            "ENVTOKEN",
			GITHUB_ENTERPRISE_TOKEN: "",
			hostname:                "github.com",
			wants: wants{
				hosts:     []string{"github.com", "example.org"},
				token:     "ENVTOKEN",
				source:    "GITHUB_TOKEN",
				writeable: false,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("GITHUB_TOKEN", tt.GITHUB_TOKEN)
			os.Setenv("GITHUB_ENTERPRISE_TOKEN", tt.GITHUB_ENTERPRISE_TOKEN)

			baseCfg := NewFromString(tt.baseConfig)
			cfg := InheritEnv(baseCfg)

			hosts, _ := cfg.Hosts()
			assert.Equal(t, tt.wants.hosts, hosts)

			val, source, _ := cfg.GetWithSource(tt.hostname, "oauth_token")
			assert.Equal(t, tt.wants.token, val)
			assert.Equal(t, tt.wants.source, source)

			val, _ = cfg.Get(tt.hostname, "oauth_token")
			assert.Equal(t, tt.wants.token, val)

			err := cfg.CheckWriteable(tt.hostname, "oauth_token")
			assert.Equal(t, tt.wants.writeable, err == nil)
		})
	}
}
