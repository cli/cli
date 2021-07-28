package config

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/cli/cli/internal/ghinstance"
	"gopkg.in/yaml.v3"
)

// This type implements a Config interface and represents a config file on disk.
type fileConfig struct {
	ConfigMap
	documentRoot *yaml.Node
}

type HostConfig struct {
	ConfigMap
	Host string
}

func (c *fileConfig) Root() *yaml.Node {
	return c.ConfigMap.Root
}

func (c *fileConfig) Get(hostname, key string) (string, error) {
	val, _, err := c.GetWithSource(hostname, key)
	return val, err
}

func (c *fileConfig) GetWithSource(hostname, key string) (string, string, error) {
	if hostname != "" {
		var notFound *NotFoundError

		hostCfg, err := c.configForHost(hostname)
		if err != nil && !errors.As(err, &notFound) {
			return "", "", err
		}

		var hostValue string
		if hostCfg != nil {
			hostValue, err = hostCfg.GetStringValue(key)
			if err != nil && !errors.As(err, &notFound) {
				return "", "", err
			}
		}

		if hostValue != "" {
			return hostValue, HostsConfigFile(), nil
		}
	}

	defaultSource := ConfigFile()

	value, err := c.GetStringValue(key)

	var notFound *NotFoundError

	if err != nil && errors.As(err, &notFound) {
		return defaultFor(key), defaultSource, nil
	} else if err != nil {
		return "", defaultSource, err
	}

	if value == "" {
		return defaultFor(key), defaultSource, nil
	}

	return value, defaultSource, nil
}

func (c *fileConfig) Set(hostname, key, value string) error {
	if hostname == "" {
		return c.SetStringValue(key, value)
	} else {
		hostCfg, err := c.configForHost(hostname)
		var notFound *NotFoundError
		if errors.As(err, &notFound) {
			hostCfg = c.makeConfigForHost(hostname)
		} else if err != nil {
			return err
		}
		return hostCfg.SetStringValue(key, value)
	}
}

func (c *fileConfig) UnsetHost(hostname string) {
	if hostname == "" {
		return
	}

	hostsEntry, err := c.FindEntry("hosts")
	if err != nil {
		return
	}

	cm := ConfigMap{hostsEntry.ValueNode}
	cm.RemoveEntry(hostname)
}

func (c *fileConfig) configForHost(hostname string) (*HostConfig, error) {
	hosts, err := c.hostEntries()
	if err != nil {
		return nil, err
	}

	for _, hc := range hosts {
		if strings.EqualFold(hc.Host, hostname) {
			return hc, nil
		}
	}
	return nil, &NotFoundError{fmt.Errorf("could not find config entry for %q", hostname)}
}

func (c *fileConfig) CheckWriteable(hostname, key string) error {
	// TODO: check filesystem permissions
	return nil
}

func (c *fileConfig) Write() error {
	mainData := yaml.Node{Kind: yaml.MappingNode}
	hostsData := yaml.Node{Kind: yaml.MappingNode}

	nodes := c.documentRoot.Content[0].Content
	for i := 0; i < len(nodes)-1; i += 2 {
		if nodes[i].Value == "hosts" {
			hostsData.Content = append(hostsData.Content, nodes[i+1].Content...)
		} else {
			mainData.Content = append(mainData.Content, nodes[i], nodes[i+1])
		}
	}

	mainBytes, err := yaml.Marshal(&mainData)
	if err != nil {
		return err
	}

	filename := ConfigFile()
	err = WriteConfigFile(filename, yamlNormalize(mainBytes))
	if err != nil {
		return err
	}

	hostsBytes, err := yaml.Marshal(&hostsData)
	if err != nil {
		return err
	}

	return WriteConfigFile(HostsConfigFile(), yamlNormalize(hostsBytes))
}

func (c *fileConfig) Aliases() (*AliasConfig, error) {
	// The complexity here is for dealing with either a missing or empty aliases key. It's something
	// we'll likely want for other config sections at some point.
	entry, err := c.FindEntry("aliases")
	var nfe *NotFoundError
	notFound := errors.As(err, &nfe)
	if err != nil && !notFound {
		return nil, err
	}

	toInsert := []*yaml.Node{}

	keyNode := entry.KeyNode
	valueNode := entry.ValueNode

	if keyNode == nil {
		keyNode = &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "aliases",
		}
		toInsert = append(toInsert, keyNode)
	}

	if valueNode == nil || valueNode.Kind != yaml.MappingNode {
		valueNode = &yaml.Node{
			Kind:  yaml.MappingNode,
			Value: "",
		}
		toInsert = append(toInsert, valueNode)
	}

	if len(toInsert) > 0 {
		newContent := []*yaml.Node{}
		if notFound {
			newContent = append(c.Root().Content, keyNode, valueNode)
		} else {
			for i := 0; i < len(c.Root().Content); i++ {
				if i == entry.Index {
					newContent = append(newContent, keyNode, valueNode)
					i++
				} else {
					newContent = append(newContent, c.Root().Content[i])
				}
			}
		}
		c.Root().Content = newContent
	}

	return &AliasConfig{
		Parent:    c,
		ConfigMap: ConfigMap{Root: valueNode},
	}, nil
}

func (c *fileConfig) hostEntries() ([]*HostConfig, error) {
	entry, err := c.FindEntry("hosts")
	if err != nil {
		return []*HostConfig{}, nil
	}

	hostConfigs, err := c.parseHosts(entry.ValueNode)
	if err != nil {
		return nil, fmt.Errorf("could not parse hosts config: %w", err)
	}

	return hostConfigs, nil
}

// Hosts returns a list of all known hostnames configured in hosts.yml
func (c *fileConfig) Hosts() ([]string, error) {
	entries, err := c.hostEntries()
	if err != nil {
		return nil, err
	}

	hostnames := []string{}
	for _, entry := range entries {
		hostnames = append(hostnames, entry.Host)
	}

	sort.SliceStable(hostnames, func(i, j int) bool { return hostnames[i] == ghinstance.Default() })

	return hostnames, nil
}

func (c *fileConfig) DefaultHost() (string, error) {
	val, _, err := c.DefaultHostWithSource()
	return val, err
}

func (c *fileConfig) DefaultHostWithSource() (string, string, error) {
	hosts, err := c.Hosts()
	if err == nil && len(hosts) == 1 {
		return hosts[0], HostsConfigFile(), nil
	}

	return ghinstance.Default(), "", nil
}

func (c *fileConfig) makeConfigForHost(hostname string) *HostConfig {
	hostRoot := &yaml.Node{Kind: yaml.MappingNode}
	hostCfg := &HostConfig{
		Host:      hostname,
		ConfigMap: ConfigMap{Root: hostRoot},
	}

	var notFound *NotFoundError
	hostsEntry, err := c.FindEntry("hosts")
	if errors.As(err, &notFound) {
		hostsEntry.KeyNode = &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "hosts",
		}
		hostsEntry.ValueNode = &yaml.Node{Kind: yaml.MappingNode}
		root := c.Root()
		root.Content = append(root.Content, hostsEntry.KeyNode, hostsEntry.ValueNode)
	} else if err != nil {
		panic(err)
	}

	hostsEntry.ValueNode.Content = append(hostsEntry.ValueNode.Content,
		&yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: hostname,
		}, hostRoot)

	return hostCfg
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

func yamlNormalize(b []byte) []byte {
	if bytes.Equal(b, []byte("{}\n")) {
		return []byte{}
	}
	return b
}

func defaultFor(key string) string {
	for _, co := range configOptions {
		if co.Key == key {
			return co.DefaultValue
		}
	}
	return ""
}
