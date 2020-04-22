package config

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

const defaultHostname = "github.com"
const defaultGitProtocol = "https"

// This interface describes interacting with some persistent configuration for gh.
type Config interface {
	Hosts() ([]*HostConfig, error)
	Get(string, string) (string, error)
	Set(string, string, string) error
	Write() error
}

type NotFoundError struct {
	error
}

type HostConfig struct {
	ConfigMap
	Host string
}

// This type implements a low-level get/set config that is backed by an in-memory tree of Yaml
// nodes. It allows us to interact with a yaml-based config programmatically, preserving any
// comments that were present when the yaml waas parsed.
type ConfigMap struct {
	Root *yaml.Node
}

func (cm *ConfigMap) GetStringValue(key string) (string, error) {
	_, valueNode, err := cm.FindEntry(key)
	if err != nil {
		return "", err
	}
	return valueNode.Value, nil
}

func (cm *ConfigMap) SetStringValue(key, value string) error {
	_, valueNode, err := cm.FindEntry(key)

	var notFound *NotFoundError

	if err != nil && errors.As(err, &notFound) {
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: key,
		}
		valueNode = &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "",
		}

		cm.Root.Content = append(cm.Root.Content, keyNode, valueNode)
	} else if err != nil {
		return err
	}

	valueNode.Value = value

	return nil
}

func (cm *ConfigMap) FindEntry(key string) (keyNode, valueNode *yaml.Node, err error) {
	err = nil

	topLevelKeys := cm.Root.Content
	for i, v := range topLevelKeys {
		if v.Value == key && i+1 < len(topLevelKeys) {
			keyNode = v
			valueNode = topLevelKeys[i+1]
			return
		}
	}

	return nil, nil, &NotFoundError{errors.New("not found")}
}

func NewConfig(root *yaml.Node) Config {
	return &fileConfig{
		ConfigMap:    ConfigMap{Root: root.Content[0]},
		documentRoot: root,
	}
}

// This type implements a Config interface and represents a config file on disk.
type fileConfig struct {
	ConfigMap
	documentRoot *yaml.Node
	hosts        []*HostConfig
}

func (c *fileConfig) Get(hostname, key string) (string, error) {
	if hostname != "" {
		hostCfg, err := c.configForHost(hostname)
		if err != nil {
			return "", err
		}

		hostValue, err := hostCfg.GetStringValue(key)
		var notFound *NotFoundError

		if err != nil && !errors.As(err, &notFound) {
			return "", err
		}

		if hostValue != "" {
			return hostValue, nil
		}
	}

	value, err := c.GetStringValue(key)

	var notFound *NotFoundError

	if err != nil && errors.As(err, &notFound) {
		return defaultFor(key), nil
	} else if err != nil {
		return "", err
	}

	if value == "" {
		return defaultFor(key), nil
	}

	return value, nil
}

func (c *fileConfig) Set(hostname, key, value string) error {
	if hostname == "" {
		return c.SetStringValue(key, value)
	} else {
		hostCfg, err := c.configForHost(hostname)
		if err != nil {
			return err
		}
		return hostCfg.SetStringValue(key, value)
	}
}

func (c *fileConfig) configForHost(hostname string) (*HostConfig, error) {
	hosts, err := c.Hosts()
	if err != nil {
		return nil, fmt.Errorf("failed to parse hosts config: %w", err)
	}

	for _, hc := range hosts {
		if hc.Host == hostname {
			return hc, nil
		}
	}
	return nil, fmt.Errorf("could not find config entry for %q", hostname)
}

func (c *fileConfig) Write() error {
	marshalled, err := yaml.Marshal(c.documentRoot)
	if err != nil {
		return err
	}

	return WriteConfigFile(ConfigFile(), marshalled)
}

func (c *fileConfig) Hosts() ([]*HostConfig, error) {
	if len(c.hosts) > 0 {
		return c.hosts, nil
	}

	_, hostsEntry, err := c.FindEntry("hosts")
	if err != nil {
		return nil, fmt.Errorf("could not find hosts config: %w", err)
	}

	hostConfigs, err := c.parseHosts(hostsEntry)
	if err != nil {
		return nil, fmt.Errorf("could not parse hosts config: %w", err)
	}

	c.hosts = hostConfigs

	return hostConfigs, nil
}

func (c *fileConfig) parseHosts(hostsEntry *yaml.Node) ([]*HostConfig, error) {
	hostConfigs := []*HostConfig{}

	for i := 0; i < len(hostsEntry.Content)-1; i = i + 2 {
		hostname := hostsEntry.Content[i].Value
		hostRoot := hostsEntry.Content[i+1]
		hostConfig := HostConfig{
			ConfigMap: ConfigMap{Root: hostRoot},
			Host:      hostname,
		}
		hostConfigs = append(hostConfigs, &hostConfig)
	}

	if len(hostConfigs) == 0 {
		return nil, errors.New("could not find any host configurations")
	}

	return hostConfigs, nil
}

func defaultFor(key string) string {
	// we only have a set default for one setting right now
	switch key {
	case "git_protocol":
		return defaultGitProtocol
	default:
		return ""
	}
}
