package context

import (
	"bytes"
	"errors"
	"testing"

	"gopkg.in/yaml.v3"
)

func Test_parseConfig(t *testing.T) {
	defer StubConfig(`---
hosts:
  github.com:
    user: monalisa
    oauth_token: OTOKEN
`)()
	config, err := ParseConfig("filename")
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
`)()
	config, err := ParseConfig("filename")
	eq(t, err, nil)
	user, err := config.Get("github.com", "user")
	eq(t, err, nil)
	eq(t, user, "monalisa")
	token, err := config.Get("github.com", "oauth_token")
	eq(t, err, nil)
	eq(t, token, "OTOKEN")
}

func Test_parseConfig_notFound(t *testing.T) {
	defer StubConfig(`---
hosts:
  example.com:
    user: wronguser
    oauth_token: NOTTHIS
`)()
	config, err := ParseConfig("filename")
	eq(t, err, nil)
	_, err = config.configForHost("github.com")
	eq(t, err, errors.New(`could not find config entry for "github.com"`))
}

func Test_migrateConfig(t *testing.T) {
	oldStyle := `---
github.com:
  - user: keiyuri
    oauth_token: 123456`

	var root yaml.Node
	err := yaml.Unmarshal([]byte(oldStyle), &root)
	if err != nil {
		panic("failed to parse test yaml")
	}

	buf := bytes.NewBufferString("")
	defer StubWriteConfig(buf)()

	defer StubBackupConfig()()

	err = migrateConfig("boom.txt", &root)
	eq(t, err, nil)

	expected := `hosts:
    github.com:
        oauth_token: "123456"
        user: keiyuri
`

	eq(t, buf.String(), expected)
}
