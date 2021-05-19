package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"

	"github.com/mitchellh/go-homedir"
	"gopkg.in/yaml.v3"
)

const (
	GH_CONFIG_DIR = "GH_CONFIG_DIR"
)

func ConfigDir() string {
	if v := os.Getenv(GH_CONFIG_DIR); v != "" {
		return v
	}

	homeDir, _ := homeDirAutoMigrate()
	return homeDir
}

func ConfigFile() string {
	return filepath.Join(ConfigDir(), "config.yml")
}

func HostsConfigFile() string {
	return filepath.Join(ConfigDir(), "hosts.yml")
}

func ParseDefaultConfig() (Config, error) {
	return parseConfig(ConfigFile())
}

func HomeDirPath(subdir string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// TODO: remove go-homedir fallback in GitHub CLI v2
		if legacyDir, err := homedir.Dir(); err == nil {
			return filepath.Join(legacyDir, subdir), nil
		}
		return "", err
	}

	newPath := filepath.Join(homeDir, subdir)
	if s, err := os.Stat(newPath); err == nil && s.IsDir() {
		return newPath, nil
	}

	// TODO: remove go-homedir fallback in GitHub CLI v2
	if legacyDir, err := homedir.Dir(); err == nil {
		legacyPath := filepath.Join(legacyDir, subdir)
		if s, err := os.Stat(legacyPath); err == nil && s.IsDir() {
			return legacyPath, nil
		}
	}

	return newPath, nil
}

// Looks up the `~/.config/gh` directory with backwards-compatibility with go-homedir and auto-migration
// when an old homedir location was found.
func homeDirAutoMigrate() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// TODO: remove go-homedir fallback in GitHub CLI v2
		if legacyDir, err := homedir.Dir(); err == nil {
			return filepath.Join(legacyDir, ".config", "gh"), nil
		}
		return "", err
	}

	newPath := filepath.Join(homeDir, ".config", "gh")
	_, newPathErr := os.Stat(newPath)
	if newPathErr == nil || !os.IsNotExist(err) {
		return newPath, newPathErr
	}

	// TODO: remove go-homedir fallback in GitHub CLI v2
	if legacyDir, err := homedir.Dir(); err == nil {
		legacyPath := filepath.Join(legacyDir, ".config", "gh")
		if s, err := os.Stat(legacyPath); err == nil && s.IsDir() {
			_ = os.MkdirAll(filepath.Dir(newPath), 0755)
			return newPath, os.Rename(legacyPath, newPath)
		}
	}

	return newPath, nil
}

var ReadConfigFile = func(filename string) ([]byte, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, pathError(err)
	}
	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	return data, nil
}

var WriteConfigFile = func(filename string, data []byte) error {
	err := os.MkdirAll(filepath.Dir(filename), 0771)
	if err != nil {
		return pathError(err)
	}

	cfgFile, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600) // cargo coded from setup
	if err != nil {
		return err
	}
	defer cfgFile.Close()

	_, err = cfgFile.Write(data)
	return err
}

var BackupConfigFile = func(filename string) error {
	return os.Rename(filename, filename+".bak")
}

func parseConfigFile(filename string) ([]byte, *yaml.Node, error) {
	data, err := ReadConfigFile(filename)
	if err != nil {
		return nil, nil, err
	}

	root, err := parseConfigData(data)
	if err != nil {
		return nil, nil, err
	}
	return data, root, err
}

func parseConfigData(data []byte) (*yaml.Node, error) {
	var root yaml.Node
	err := yaml.Unmarshal(data, &root)
	if err != nil {
		return nil, err
	}

	if len(root.Content) == 0 {
		return &yaml.Node{
			Kind:    yaml.DocumentNode,
			Content: []*yaml.Node{{Kind: yaml.MappingNode}},
		}, nil
	}
	if root.Content[0].Kind != yaml.MappingNode {
		return &root, fmt.Errorf("expected a top level map")
	}
	return &root, nil
}

func isLegacy(root *yaml.Node) bool {
	for _, v := range root.Content[0].Content {
		if v.Value == "github.com" {
			return true
		}
	}

	return false
}

func migrateConfig(filename string) error {
	b, err := ReadConfigFile(filename)
	if err != nil {
		return err
	}

	var hosts map[string][]yaml.Node
	err = yaml.Unmarshal(b, &hosts)
	if err != nil {
		return fmt.Errorf("error decoding legacy format: %w", err)
	}

	cfg := NewBlankConfig()
	for hostname, entries := range hosts {
		if len(entries) < 1 {
			continue
		}
		mapContent := entries[0].Content
		for i := 0; i < len(mapContent)-1; i += 2 {
			if err := cfg.Set(hostname, mapContent[i].Value, mapContent[i+1].Value); err != nil {
				return err
			}
		}
	}

	err = BackupConfigFile(filename)
	if err != nil {
		return fmt.Errorf("failed to back up existing config: %w", err)
	}

	return cfg.Write()
}

func parseConfig(filename string) (Config, error) {
	_, root, err := parseConfigFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			root = NewBlankRoot()
		} else {
			return nil, err
		}
	}

	if isLegacy(root) {
		err = migrateConfig(filename)
		if err != nil {
			return nil, fmt.Errorf("error migrating legacy config: %w", err)
		}

		_, root, err = parseConfigFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to reparse migrated config: %w", err)
		}
	} else {
		if _, hostsRoot, err := parseConfigFile(HostsConfigFile()); err == nil {
			if len(hostsRoot.Content[0].Content) > 0 {
				newContent := []*yaml.Node{
					{Value: "hosts"},
					hostsRoot.Content[0],
				}
				restContent := root.Content[0].Content
				root.Content[0].Content = append(newContent, restContent...)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	return NewConfig(root), nil
}

func pathError(err error) error {
	var pathError *os.PathError
	if errors.As(err, &pathError) && errors.Is(pathError.Err, syscall.ENOTDIR) {
		if p := findRegularFile(pathError.Path); p != "" {
			return fmt.Errorf("remove or rename regular file `%s` (must be a directory)", p)
		}

	}
	return err
}

func findRegularFile(p string) string {
	for {
		if s, err := os.Stat(p); err == nil && s.Mode().IsRegular() {
			return p
		}
		newPath := filepath.Dir(p)
		if newPath == p || newPath == "/" || newPath == "." {
			break
		}
		p = newPath
	}
	return ""
}
