package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// This interface describes interacting with some persistent configuration for gh.
type Config interface {
	Get(string, string) (string, error)
	GetOrDefault(string, string) (string, error)
	GetWithSource(string, string) (string, string, error)
	Set(string, string, string) error
	UnsetHost(string)
	Hosts() ([]string, error)
	DefaultHost() (string, error)
	DefaultHostWithSource() (string, string, error)
	Aliases() (*AliasConfig, error)
	CheckWriteable(string, string) error
	Write() error
}

type ConfigOption struct {
	Key           string
	Description   string
	DefaultValue  string
	AllowedValues []string
}

var configOptions = []ConfigOption{
	{
		Key:           "git_protocol",
		Description:   "the protocol to use for git clone and push operations",
		DefaultValue:  "https",
		AllowedValues: []string{"https", "ssh"},
	},
	{
		Key:          "editor",
		Description:  "the text editor program to use for authoring text",
		DefaultValue: "",
	},
	{
		Key:           "prompt",
		Description:   "toggle interactive prompting in the terminal",
		DefaultValue:  "enabled",
		AllowedValues: []string{"enabled", "disabled"},
	},
	{
		Key:          "pager",
		Description:  "the terminal pager program to send standard output to",
		DefaultValue: "",
	},
}

func DefaultFor(key string) string {
	for _, co := range configOptions {
		if co.Key == key {
			return co.DefaultValue
		}
	}
	return ""
}

func ConfigOptions() []ConfigOption {
	return configOptions
}

func ValidateKey(key string) error {
	for _, configKey := range configOptions {
		if key == configKey.Key {
			return nil
		}
	}

	return fmt.Errorf("invalid key")
}

type InvalidValueError struct {
	ValidValues []string
}

func (e InvalidValueError) Error() string {
	return "invalid value"
}

func ValidateValue(key, value string) error {
	var validValues []string

	for _, v := range configOptions {
		if v.Key == key {
			validValues = v.AllowedValues
			break
		}
	}

	if validValues == nil {
		return nil
	}

	for _, v := range validValues {
		if v == value {
			return nil
		}
	}

	return &InvalidValueError{ValidValues: validValues}
}

func NewConfig(root *yaml.Node) Config {
	return &fileConfig{
		ConfigMap:    ConfigMap{Root: root.Content[0]},
		documentRoot: root,
	}
}

// NewFromString initializes a Config from a yaml string
func NewFromString(str string) Config {
	root, err := parseConfigData([]byte(str))
	if err != nil {
		panic(err)
	}
	return NewConfig(root)
}

// NewBlankConfig initializes a config file pre-populated with comments and default values
func NewBlankConfig() Config {
	return NewConfig(NewBlankRoot())
}

func NewBlankRoot() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.DocumentNode,
		Content: []*yaml.Node{
			{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{
						HeadComment: "What protocol to use when performing git operations. Supported values: ssh, https",
						Kind:        yaml.ScalarNode,
						Value:       "git_protocol",
					},
					{
						Kind:  yaml.ScalarNode,
						Value: "https",
					},
					{
						HeadComment: "What editor gh should run when creating issues, pull requests, etc. If blank, will refer to environment.",
						Kind:        yaml.ScalarNode,
						Value:       "editor",
					},
					{
						Kind:  yaml.ScalarNode,
						Value: "",
					},
					{
						HeadComment: "When to interactively prompt. This is a global config that cannot be overridden by hostname. Supported values: enabled, disabled",
						Kind:        yaml.ScalarNode,
						Value:       "prompt",
					},
					{
						Kind:  yaml.ScalarNode,
						Value: "enabled",
					},
					{
						HeadComment: "A pager program to send command output to, e.g. \"less\". Set the value to \"cat\" to disable the pager.",
						Kind:        yaml.ScalarNode,
						Value:       "pager",
					},
					{
						Kind:  yaml.ScalarNode,
						Value: "",
					},
					{
						HeadComment: "Aliases allow you to create nicknames for gh commands",
						Kind:        yaml.ScalarNode,
						Value:       "aliases",
					},
					{
						Kind: yaml.MappingNode,
						Content: []*yaml.Node{
							{
								Kind:  yaml.ScalarNode,
								Value: "co",
							},
							{
								Kind:  yaml.ScalarNode,
								Value: "pr checkout",
							},
						},
					},
				},
			},
		},
	}
}
