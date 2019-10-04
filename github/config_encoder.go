package github

import (
	"io"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v2"
)

type configEncoder interface {
	Encode(w io.Writer, c *Config) error
}

type tomlConfigEncoder struct {
}

func (t *tomlConfigEncoder) Encode(w io.Writer, c *Config) error {
	enc := toml.NewEncoder(w)
	return enc.Encode(c)
}

type yamlConfigEncoder struct {
}

func (y *yamlConfigEncoder) Encode(w io.Writer, c *Config) error {
	yc := yaml.MapSlice{}
	for _, h := range c.Hosts {
		yc = append(yc, yaml.MapItem{
			Key: h.Host,
			Value: []yamlHost{
				{
					User:       h.User,
					OAuthToken: h.AccessToken,
					Protocol:   h.Protocol,
					UnixSocket: h.UnixSocket,
				},
			},
		})
	}

	d, err := yaml.Marshal(yc)
	if err != nil {
		return err
	}

	n, err := w.Write(d)
	if err == nil && n < len(d) {
		err = io.ErrShortWrite
	}

	return err
}
