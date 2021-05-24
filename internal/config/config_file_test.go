package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
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
		name        string
		onlyWindows bool
		env         map[string]string
		output      string
	}{
		{
			name: "no envVars",
			env: map[string]string{
				"GH_CONFIG_DIR":   "",
				"XDG_CONFIG_HOME": "",
				"AppData":         "",
				"USERPROFILE":     "",
				"HOME":            "",
			},
			output: ".config/gh",
		},
		{
			name: "GH_CONFIG_DIR specified",
			env: map[string]string{
				"GH_CONFIG_DIR": "/tmp/gh_config_dir",
			},
			output: "/tmp/gh_config_dir",
		},
		{
			name: "XDG_CONFIG_HOME specified",
			env: map[string]string{
				"XDG_CONFIG_HOME": "/tmp",
			},
			output: "/tmp/gh",
		},
		{
			name: "GH_CONFIG_DIR and XDG_CONFIG_HOME specified",
			env: map[string]string{
				"GH_CONFIG_DIR":   "/tmp/gh_config_dir",
				"XDG_CONFIG_HOME": "/tmp",
			},
			output: "/tmp/gh_config_dir",
		},
		{
			name:        "AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"AppData": "/tmp/",
			},
			output: "/tmp/GitHub CLI",
		},
		{
			name:        "GH_CONFIG_DIR and AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"GH_CONFIG_DIR": "/tmp/gh_config_dir",
				"AppData":       "/tmp",
			},
			output: "/tmp/gh_config_dir",
		},
		{
			name:        "XDG_CONFIG_HOME and AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"XDG_CONFIG_HOME": "/tmp",
				"AppData":         "/tmp",
			},
			output: "/tmp/gh",
		},
	}

	for _, tt := range tests {
		if tt.onlyWindows && runtime.GOOS != "windows" {
			continue
		}
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != nil {
				for k, v := range tt.env {
					old := os.Getenv(k)
					os.Setenv(k, filepath.FromSlash(v))
					defer os.Setenv(k, old)
				}
			}

			defer stubMigrateConfigDir()()
			assert.Equal(t, filepath.FromSlash(tt.output), ConfigDir())
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

func Test_autoMigrateConfigDir_noMigration(t *testing.T) {
	migrateDir := t.TempDir()

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, "/nonexistent-dir")
	defer os.Setenv(homeEnvVar, old)

	autoMigrateConfigDir(migrateDir)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateConfigDir_noMigration_samePath(t *testing.T) {
	migrateDir := t.TempDir()

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, migrateDir)
	defer os.Setenv(homeEnvVar, old)

	autoMigrateConfigDir(migrateDir)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateConfigDir_migration(t *testing.T) {
	defaultDir := t.TempDir()
	dd := filepath.Join(defaultDir, ".config", "gh")
	migrateDir := t.TempDir()
	md := filepath.Join(migrateDir, ".config", "gh")

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, defaultDir)
	defer os.Setenv(homeEnvVar, old)

	err := os.MkdirAll(dd, 0777)
	assert.NoError(t, err)
	f, err := ioutil.TempFile(dd, "")
	assert.NoError(t, err)
	f.Close()

	autoMigrateConfigDir(md)

	_, err = ioutil.ReadDir(dd)
	assert.True(t, os.IsNotExist(err))

	files, err := ioutil.ReadDir(md)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files))
}
