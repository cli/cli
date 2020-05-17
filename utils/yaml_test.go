package utils

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFlattenYamlNode(t *testing.T) {
	cases := []struct {
		config   string
		expected []*FlatYamlEntry
	}{
		{
			config: `foo: bar
baz: `,
			expected: []*FlatYamlEntry{
				{Key: "foo", Value: "bar"},
				{Key: "baz", Value: ""},
			},
		},
		{
			config: `foo: bar
baz:
  qux: quux
  abc: xyz
`,
			expected: []*FlatYamlEntry{
				{Key: "foo", Value: "bar"},
				{Key: "baz.qux", Value: "quux"},
				{Key: "baz.abc", Value: "xyz"},
			},
		},
	}

	for _, test := range cases {
		var root yaml.Node
		err := yaml.Unmarshal([]byte(test.config), &root)
		if err != nil {
			t.Errorf("Failed to parse test config: %v", err)
		}

		result := FlattenYamlNode(root.Content[0])
		for i, got := range result {
			if !reflect.DeepEqual(test.expected[i], got) {
				t.Errorf("expected: %v got %v", test.expected[i], got)
			}
		}
	}
}
