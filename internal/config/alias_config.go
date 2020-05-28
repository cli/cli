package config

import (
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"
)

type AliasConfig struct {
	ConfigMap
	Parent Config
}

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
		// TODO awful hack bad type conversion i'm sorry
		entry, err := a.Parent.(*fileConfig).FindEntry("aliases")

		var notFound *NotFoundError

		if err != nil && !errors.As(err, &notFound) {
			return err
		}
		valueNode := &yaml.Node{
			Kind:  yaml.MappingNode,
			Value: "",
		}

		a.Root = valueNode
		if errors.As(err, &notFound) {
			// No aliases: key; just append new aliases key and empty map to end
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: "aliases",
			}
			a.Parent.Root().Content = append(a.Parent.Root().Content, keyNode, valueNode)
		} else {
			// Empty aliases; inject new value node after existing aliases key
			newContent := []*yaml.Node{}
			for i := 0; i < len(a.Parent.Root().Content); i++ {
				if i == entry.Index {
					newContent = append(newContent, entry.KeyNode, valueNode)
					i++
				} else {
					newContent = append(newContent, a.Parent.Root().Content[i])
				}
			}
			a.Parent.Root().Content = newContent
		}
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
