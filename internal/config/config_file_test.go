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
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		onlyWindows bool
		env         map[string]string
		output      string
	}{
		{
			name: "HOME/USERPROFILE specified",
			env: map[string]string{
				"GH_CONFIG_DIR":   "",
				"XDG_CONFIG_HOME": "",
				"AppData":         "",
				"USERPROFILE":     tempDir,
				"HOME":            tempDir,
			},
			output: filepath.Join(tempDir, ".config", "gh"),
		},
		{
			name: "GH_CONFIG_DIR specified",
			env: map[string]string{
				"GH_CONFIG_DIR": filepath.Join(tempDir, "gh_config_dir"),
			},
			output: filepath.Join(tempDir, "gh_config_dir"),
		},
		{
			name: "XDG_CONFIG_HOME specified",
			env: map[string]string{
				"XDG_CONFIG_HOME": tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
		},
		{
			name: "GH_CONFIG_DIR and XDG_CONFIG_HOME specified",
			env: map[string]string{
				"GH_CONFIG_DIR":   filepath.Join(tempDir, "gh_config_dir"),
				"XDG_CONFIG_HOME": tempDir,
			},
			output: filepath.Join(tempDir, "gh_config_dir"),
		},
		{
			name:        "AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"AppData": tempDir,
			},
			output: filepath.Join(tempDir, "GitHub CLI"),
		},
		{
			name:        "GH_CONFIG_DIR and AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"GH_CONFIG_DIR": filepath.Join(tempDir, "gh_config_dir"),
				"AppData":       tempDir,
			},
			output: filepath.Join(tempDir, "gh_config_dir"),
		},
		{
			name:        "XDG_CONFIG_HOME and AppData specified",
			onlyWindows: true,
			env: map[string]string{
				"XDG_CONFIG_HOME": tempDir,
				"AppData":         tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
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
					os.Setenv(k, v)
					defer os.Setenv(k, old)
				}
			}

			// Create directory to skip auto migration code
			// which gets run when target directory does not exist
			_ = os.MkdirAll(tt.output, 0755)

			assert.Equal(t, tt.output, ConfigDir())
		})
	}
}

func Test_configFile_Write_toDisk(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), ".config", "gh")
	_ = os.MkdirAll(configDir, 0755)
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

func Test_autoMigrateConfigDir_noMigration_notExist(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := t.TempDir()

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err := autoMigrateConfigDir(migrateDir)
	assert.Equal(t, errNotExist, err)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateConfigDir_noMigration_samePath(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := filepath.Join(homeDir, ".config", "gh")
	err := os.MkdirAll(migrateDir, 0755)
	assert.NoError(t, err)

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err = autoMigrateConfigDir(migrateDir)
	assert.Equal(t, errSamePath, err)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateConfigDir_migration(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := t.TempDir()
	homeConfigDir := filepath.Join(homeDir, ".config", "gh")
	migrateConfigDir := filepath.Join(migrateDir, ".config", "gh")

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err := os.MkdirAll(homeConfigDir, 0755)
	assert.NoError(t, err)
	f, err := ioutil.TempFile(homeConfigDir, "")
	assert.NoError(t, err)
	f.Close()

	err = autoMigrateConfigDir(migrateConfigDir)
	assert.NoError(t, err)

	_, err = ioutil.ReadDir(homeConfigDir)
	assert.True(t, os.IsNotExist(err))

	files, err := ioutil.ReadDir(migrateConfigDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files))
}

func Test_StateDir(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		onlyWindows bool
		env         map[string]string
		output      string
	}{
		{
			name: "HOME/USERPROFILE specified",
			env: map[string]string{
				"XDG_STATE_HOME":  "",
				"GH_CONFIG_DIR":   "",
				"XDG_CONFIG_HOME": "",
				"LocalAppData":    "",
				"USERPROFILE":     tempDir,
				"HOME":            tempDir,
			},
			output: filepath.Join(tempDir, ".local", "state", "gh"),
		},
		{
			name: "XDG_STATE_HOME specified",
			env: map[string]string{
				"XDG_STATE_HOME": tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
		},
		{
			name:        "LocalAppData specified",
			onlyWindows: true,
			env: map[string]string{
				"LocalAppData": tempDir,
			},
			output: filepath.Join(tempDir, "GitHub CLI"),
		},
		{
			name:        "XDG_STATE_HOME and LocalAppData specified",
			onlyWindows: true,
			env: map[string]string{
				"XDG_STATE_HOME": tempDir,
				"LocalAppData":   tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
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
					os.Setenv(k, v)
					defer os.Setenv(k, old)
				}
			}

			// Create directory to skip auto migration code
			// which gets run when target directory does not exist
			_ = os.MkdirAll(tt.output, 0755)

			assert.Equal(t, tt.output, StateDir())
		})
	}
}

func Test_autoMigrateStateDir_noMigration_notExist(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := t.TempDir()

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err := autoMigrateStateDir(migrateDir)
	assert.Equal(t, errNotExist, err)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateStateDir_noMigration_samePath(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := filepath.Join(homeDir, ".config", "gh")
	err := os.MkdirAll(migrateDir, 0755)
	assert.NoError(t, err)

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err = autoMigrateStateDir(migrateDir)
	assert.Equal(t, errSamePath, err)

	files, err := ioutil.ReadDir(migrateDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))
}

func Test_autoMigrateStateDir_migration(t *testing.T) {
	homeDir := t.TempDir()
	migrateDir := t.TempDir()
	homeConfigDir := filepath.Join(homeDir, ".config", "gh")
	migrateStateDir := filepath.Join(migrateDir, ".local", "state", "gh")

	homeEnvVar := "HOME"
	if runtime.GOOS == "windows" {
		homeEnvVar = "USERPROFILE"
	}
	old := os.Getenv(homeEnvVar)
	os.Setenv(homeEnvVar, homeDir)
	defer os.Setenv(homeEnvVar, old)

	err := os.MkdirAll(homeConfigDir, 0755)
	assert.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(homeConfigDir, "state.yml"), nil, 0755)
	assert.NoError(t, err)

	err = autoMigrateStateDir(migrateStateDir)
	assert.NoError(t, err)

	files, err := ioutil.ReadDir(homeConfigDir)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(files))

	files, err = ioutil.ReadDir(migrateStateDir)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(files))
	assert.Equal(t, "state.yml", files[0].Name())
}

func Test_DataDir(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		onlyWindows bool
		env         map[string]string
		output      string
	}{
		{
			name: "HOME/USERPROFILE specified",
			env: map[string]string{
				"XDG_DATA_HOME":   "",
				"GH_CONFIG_DIR":   "",
				"XDG_CONFIG_HOME": "",
				"LocalAppData":    "",
				"USERPROFILE":     tempDir,
				"HOME":            tempDir,
			},
			output: filepath.Join(tempDir, ".local", "share", "gh"),
		},
		{
			name: "XDG_DATA_HOME specified",
			env: map[string]string{
				"XDG_DATA_HOME": tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
		},
		{
			name:        "LocalAppData specified",
			onlyWindows: true,
			env: map[string]string{
				"LocalAppData": tempDir,
			},
			output: filepath.Join(tempDir, "GitHub CLI"),
		},
		{
			name:        "XDG_DATA_HOME and LocalAppData specified",
			onlyWindows: true,
			env: map[string]string{
				"XDG_DATA_HOME": tempDir,
				"LocalAppData":  tempDir,
			},
			output: filepath.Join(tempDir, "gh"),
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
					os.Setenv(k, v)
					defer os.Setenv(k, old)
				}
			}

			assert.Equal(t, tt.output, DataDir())
		})
	}
}
