package context

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
)

type configEntry struct {
	User  string
	Token string `yaml:"oauth_token"`
}

func parseConfigFile(fn string) (*configEntry, error) {
	migrateConfigFile()

	f, err := os.Open(fn)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return setupConfigFile(fn)
		}
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
	if len(config.Content) < 1 {
		return nil, fmt.Errorf("malformed config")
	}
	for i := 0; i < len(config.Content[0].Content)-1; i = i + 2 {
		if config.Content[0].Content[i].Value == defaultHostname {
			var entries []configEntry
			err = config.Content[0].Content[i+1].Decode(&entries)
			if err != nil {
				return nil, err
			}
			return &entries[0], nil
		}
	}
	return nil, fmt.Errorf("could not find config entry for %q", defaultHostname)
}

// This is a temporary function that will migrate the config file. It can be removed
// in January.
//
// If ~/.config/gh is a file, convert it to a directory and place the file
// into ~/.config/gh/config.yml
func migrateConfigFile() {
	p, _ := homedir.Expand("~/.config/gh")
	fi, err := os.Stat(p)
	if err != nil { // This means the file doesn't exist, and that is fine.
		return
	}
	if fi.Mode().IsDir() {
		return
	}

	content, err := ioutil.ReadFile(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to read config at %s", p)
		return
	}

	err = os.Remove(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to remove %s", p)
		return
	}

	err = os.MkdirAll(p, 0771)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to mkdir %s", p)
		return
	}

	newPath := path.Join(p, "config.yml")
	err = ioutil.WriteFile(newPath, []byte(content), 0771)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed write to new config path %s", newPath)
		return
	}
}
