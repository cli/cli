package context

import (
	"io"
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v3"
)

type configEntry struct {
	User  string
	Token string `yaml:"oauth_token"`
}

func parseConfigFile(fn string) (*configEntry, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseConfig(f)
}

func parseConfig(r io.Reader) (*configEntry, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var config yaml.Node
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}
	var entries []configEntry
	// TODO: this will panic if the config is malformed
	err = config.Content[0].Content[1].Decode(&entries)
	if err != nil {
		return nil, err
	}
	return &entries[0], nil
}
