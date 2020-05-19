package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type AliasConfig struct {
	ConfigMap
	Parent Config
}

// TODO at what point should the alias top level be added? Lazily on first write, I think;
// but then we aren't initializing the default aliases that we want. also want to ensure that we
// don't re-add the default aliases if people delete them.
//
// I think I'm not going to worry about it for now; config setup will add the aliases and existing
// users can take some step later on to add them if they want them. we'll otherwise lazily create
// the aliases: section on first write.

func (a *AliasConfig) Exists(alias string) bool {
	if a.Empty() {
		return false
	}
	value, _ := a.GetStringValue(alias)

	return value != ""
}

func (a *AliasConfig) Get(alias string) string {
	value, _ := a.GetStringValue(alias)

	return value
}

func (a *AliasConfig) Add(alias, expansion string) error {
	if a.Root == nil {
		// then we don't actually have an aliases stanza in the config yet.
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "aliases",
		}
		valueNode := &yaml.Node{
			Kind:  yaml.MappingNode,
			Value: "",
		}

		a.Root = valueNode
		a.Parent.Root().Content = append(a.Parent.Root().Content, keyNode, valueNode)
	}

	err := a.SetStringValue(alias, expansion)
	if err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	err = a.Parent.Write()
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func (a *AliasConfig) Delete(alias string) error {
	// TODO when we get to gh alias delete
	return nil
}
