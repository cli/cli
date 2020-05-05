package utils

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// FlatYamlEntry represents a flattened yaml entry
type FlatYamlEntry struct {
	Key   string // e.g. topkey.childkey1.childkey2
	Value string
}

// FlattenYamlNode flattens nested yaml.Node
func FlattenYamlNode(node *yaml.Node) []*FlatYamlEntry {
	if node == nil || node.Kind != yaml.MappingNode {
		return make([]*FlatYamlEntry, 0)
	}

	var entries []*FlatYamlEntry
	if node.Kind == yaml.MappingNode {
		entries = make([]*FlatYamlEntry, 0)
		nodes := node.Content

		for i := 0; i < len(nodes)-1; i += 2 {
			key := nodes[i]
			value := nodes[i+1]
			if key.Kind == yaml.ScalarNode && value.Kind == yaml.MappingNode {
				keyStr := key.Value
				children := FlattenYamlNode(value)
				for _, c := range children {
					c.Key = fmt.Sprintf("%s.%s", keyStr, c.Key)
				}
				entries = append(entries, children...)
			} else if value.Kind == yaml.ScalarNode {
				keyStr := key.Value
				valueStr := value.Value
				entries = append(entries, &FlatYamlEntry{
					Key:   keyStr,
					Value: valueStr,
				})
			}
		}
	}

	return entries
}
