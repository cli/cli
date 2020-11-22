package config

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func eq(t *testing.T, got interface{}, expected interface{}) {
	t.Helper()
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("expected: %v, got: %v", expected, got)
	}
}

func Test_parseConfig(t *testing.T) {
	defer StubConfig(`---
hosts:
  github.com:
    user: monalisa
    oauth_token: OTOKEN
`, "")()
	config, err := ParseConfig("config.yml")
	eq(t, err, nil)
	user, err := config.Get("github.com", "user")
	eq(t, err, nil)
	eq(t, user, "monalisa")
	token, err := config.Get("github.com", "oauth_token")
	eq(t, err, nil)
	eq(t, token, "OTOKEN")
}

func Test_parseConfig_multipleHosts(t *testing.T) {
	defer StubConfig(`---
hosts:
  example.com:
    user: wronguser
    oauth_token: NOTTHIS
  github.com:
    user: monalisa
    oauth_token: OTOKEN
`, "")()
	config, err := ParseConfig("config.yml")
	eq(t, err, nil)
	user, err := config.Get("github.com", "user")
	eq(t, err, nil)
	eq(t, user, "monalisa")
	token, err := config.Get("github.com", "oauth_token")
	eq(t, err, nil)
	eq(t, token, "OTOKEN")
}

func Test_parseConfig_hostsFile(t *testing.T) {
	defer StubConfig("", `---
github.com:
  user: monalisa
  oauth_token: OTOKEN
`)()
	config, err := ParseConfig("config.yml")
	eq(t, err, nil)
	user, err := config.Get("github.com", "user")
	eq(t, err, nil)
	eq(t, user, "monalisa")
	token, err := config.Get("github.com", "oauth_token")
	eq(t, err, nil)
	eq(t, token, "OTOKEN")
}

func Test_parseConfig_hostFallback(t *testing.T) {
	defer StubConfig(`---
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
	config, err := ParseConfig("config.yml")
	eq(t, err, nil)
	val, err := config.Get("example.com", "git_protocol")
	eq(t, err, nil)
	eq(t, val, "https")
	val, err = config.Get("github.com", "git_protocol")
	eq(t, err, nil)
	eq(t, val, "ssh")
	val, err = config.Get("nonexistent.io", "git_protocol")
	eq(t, err, nil)
	eq(t, val, "ssh")
}

func Test_ParseConfig_migrateConfig(t *testing.T) {
	defer StubConfig(`---
github.com:
  - user: keiyuri
    oauth_token: 123456
`, "")()

	mainBuf := bytes.Buffer{}
	hostsBuf := bytes.Buffer{}
	defer StubWriteConfig(&mainBuf, &hostsBuf)()
	defer StubBackupConfig()()

	_, err := ParseConfig("config.yml")
	assert.Nil(t, err)

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
			defer StubConfig(tt.contents, "")()
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
