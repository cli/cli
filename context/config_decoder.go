package context

import (
	"io"
	"io/ioutil"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

type configDecoder interface {
	Decode(r io.Reader, c *Config) error
}

type tomlConfigDecoder struct {
}

func (t *tomlConfigDecoder) Decode(r io.Reader, c *Config) error {
	_, err := toml.DecodeReader(r, c)
	return err
}

type yamlConfigDecoder struct {
}

func (y *yamlConfigDecoder) Decode(r io.Reader, c *Config) error {
	d, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}

	yc := yaml.MapSlice{}
	err = yaml.Unmarshal(d, &yc)

	if err != nil {
		return err
	}

	for _, hostEntry := range yc {
		v := hostEntry.Value.([]interface{})
		if len(v) < 1 {
			continue
		}
		host := &Host{Host: hostEntry.Key.(string)}
		for _, prop := range v[0].(yaml.MapSlice) {
			switch prop.Key.(string) {
			case "user":
				host.User = prop.Value.(string)
			case "oauth_token":
				host.AccessToken = prop.Value.(string)
			case "protocol":
				host.Protocol = prop.Value.(string)
			case "unix_socket":
				host.UnixSocket = prop.Value.(string)
			}
		}
		c.Hosts = append(c.Hosts, host)
	}

	return nil
}
