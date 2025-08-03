package strictyaml

import (
	"io"
	"testing"

	"github.com/letsencrypt/boulder/test"
)

var (
	emptyConfig = []byte(``)
	validConfig = []byte(`
a: c
d: c
`)
	invalidConfig1 = []byte(`
x: y
`)

	invalidConfig2 = []byte(`
a: c
d: c
x:
  - hey
`)
)

func TestStrictYAMLUnmarshal(t *testing.T) {
	var config struct {
		A string `yaml:"a"`
		D string `yaml:"d"`
	}

	err := Unmarshal(validConfig, &config)
	test.AssertNotError(t, err, "yaml: unmarshal errors")
	test.AssertNotError(t, err, "EOF")

	err = Unmarshal(invalidConfig1, &config)
	test.AssertError(t, err, "yaml: unmarshal errors")

	err = Unmarshal(invalidConfig2, &config)
	test.AssertError(t, err, "yaml: unmarshal errors")

	// Test an empty buffer (config file)
	err = Unmarshal(emptyConfig, &config)
	test.AssertErrorIs(t, err, io.EOF)
}
