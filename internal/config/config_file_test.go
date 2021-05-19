package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func Test_parseConfig(t *testing.T) {
	defer stubConfig(`---
hosts:
  github.com:
    user: monalisa
    oauth_token: OTOKEN
`, "")()
	config, err := parseConfig("config.yml")
	assert.NoError(t, err)
	user, err := config.Get("github.com", "user")
	assert.NoError(t, err)
	assert.Equal(t, "monalisa", user)
	token, err := config.Get("github.com", "oauth_token")
	assert.NoError(t, err)
	assert.Equal(t, "OTOKEN", token)
}

func Test_parseConfig_multipleHosts(t *testing.T) {
	defer stubConfig(`---
hosts:
  example.com:
    user: wronguser
    oauth_token: NOTTHIS
  github.com:
    user: monalisa
    oauth_token: OTOKEN
`, "")()
	config, err := parseConfig("config.yml")
	assert.NoError(t, err)
	user, err := config.Get("github.com", "user")
	assert.NoError(t, err)
	assert.Equal(t, "monalisa", user)
	token, err := config.Get("github.com", "oauth_token")
	assert.NoError(t, err)
	assert.Equal(t, "OTOKEN", token)
}

func Test_parseConfig_hostsFile(t *testing.T) {
	defer stubConfig("", `---
github.com:
  user: monalisa
  oauth_token: OTOKEN
`)()
	config, err := parseConfig("config.yml")
	assert.NoError(t, err)
	user, err := config.Get("github.com", "user")
	assert.NoError(t, err)
	assert.Equal(t, "monalisa", user)
	token, err := config.Get("github.com", "oauth_token")
	assert.NoError(t, err)
	assert.Equal(t, "OTOKEN", token)
}

func Test_parseConfig_hostFallback(t *testing.T) {
	defer stubConfig(`---
git_protocol: ssh
`, `---
github.com:
    user: monalisa
    oauth_token: OTOKEN
example.com:
    user: wronguser
    oauth_token: NOTTHIS
    git_protocol: https
`)()
	config, err := parseConfig("config.yml")
	assert.NoError(t, err)
	val, err := config.Get("example.com", "git_protocol")
	assert.NoError(t, err)
	assert.Equal(t, "https", val)
	val, err = config.Get("github.com", "git_protocol")
	assert.NoError(t, err)
	assert.Equal(t, "ssh", val)
	val, err = config.Get("nonexistent.io", "git_protocol")
	assert.NoError(t, err)
	assert.Equal(t, "ssh", val)
}

func Test_parseConfig_migrateConfig(t *testing.T) {
	defer stubConfig(`---
github.com:
  - user: keiyuri
    oauth_token: 123456
`, "")()

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer StubWriteConfig(&mainBuf, &hostsBuf)()
	defer StubBackupConfig()()

	_, err := parseConfig("config.yml")
	assert.NoError(t, err)

	expectedHosts := `github.com:
    user: keiyuri
    oauth_token: "123456"
`

	assert.Equal(t, expectedHosts, hostsBuf.String())
	assert.NotContains(t, mainBuf.String(), "github.com")
	assert.NotContains(t, mainBuf.String(), "oauth_token")
}

func Test_parseConfigFile(t *testing.T) {
	tests := []struct {
		contents string
		wantsErr bool
	}{
		{
			contents: "",
			wantsErr: true,
		},
		{
			contents: " ",
			wantsErr: false,
		},
		{
			contents: "\n",
			wantsErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("contents: %q", tt.contents), func(t *testing.T) {
			defer stubConfig(tt.contents, "")()
			_, yamlRoot, err := parseConfigFile("config.yml")
			if tt.wantsErr != (err != nil) {
				t.Fatalf("got error: %v", err)
			}
			if tt.wantsErr {
				return
			}
			assert.Equal(t, yaml.MappingNode, yamlRoot.Content[0].Kind)
			assert.Equal(t, 0, len(yamlRoot.Content[0].Content))
		})
	}
}

func Test_ConfigDir(t *testing.T) {
	tests := []struct {
		envVar string
		want   string
	}{
		{"/tmp/gh", ".tmp.gh"},
		{"", ".config.gh"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("envVar: %q", tt.envVar), func(t *testing.T) {
			if tt.envVar != "" {
				os.Setenv(GH_CONFIG_DIR, tt.envVar)
				defer os.Unsetenv(GH_CONFIG_DIR)
			}
			assert.Regexp(t, tt.want, ConfigDir())
		})
	}
}

func Test_configFile_Write_toDisk(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".config", "gh")
	os.Setenv(GH_CONFIG_DIR, configDir)
	defer os.Unsetenv(GH_CONFIG_DIR)

	cfg := NewFromString(`pager: less`)
	err := cfg.Write()
	if err != nil {
		t.Fatal(err)
	}

	expectedConfig := "pager: less\n"
	if configBytes, err := ioutil.ReadFile(filepath.Join(configDir, "config.yml")); err != nil {
		t.Error(err)
	} else if string(configBytes) != expectedConfig {
		t.Errorf("expected config.yml %q, got %q", expectedConfig, string(configBytes))
	}

	if configBytes, err := ioutil.ReadFile(filepath.Join(configDir, "hosts.yml")); err != nil {
		t.Error(err)
	} else if string(configBytes) != "" {
		t.Errorf("unexpected hosts.yml: %q", string(configBytes))
	}
}
