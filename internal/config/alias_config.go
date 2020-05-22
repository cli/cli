package config

import (
	"fmt"
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

func (a *AliasConfig) All() map[string]string {
	out := map[string]string{}

	if a.Empty() {
		return out
	}

	for i := 0; i < len(a.Root.Content); i += 2 {
		if i+1 == len(a.Root.Content) {
			break
		}
		key := a.Root.Content[i].Value
		value := a.Root.Content[i+1].Value
		out[key] = value
	}

	return out
}
