package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestFindEntry(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		output  string
		wantErr bool
	}{
		{
			name:   "find key",
			key:    "valid",
			output: "present",
		},
		{
			name:    "find key that is not present",
			key:     "invalid",
			wantErr: true,
		},
		{
			name:   "find key with blank value",
			key:    "blank",
			output: "",
		},
		{
			name:   "find key that has same content as a value",
			key:    "same",
			output: "logical",
		},
	}

	for _, tt := range tests {
		cm := ConfigMap{Root: testYaml()}
		t.Run(tt.name, func(t *testing.T) {
			out, err := cm.FindEntry(tt.key)
			if tt.wantErr {
				assert.EqualError(t, err, "not found")
				return
			}
			assert.NoError(t, err)
			fmt.Println(out)
			assert.Equal(t, tt.output, out.ValueNode.Value)
		})
	}
}

func testYaml() *yaml.Node {
	var root yaml.Node
	var data = `
valid: present
erroneous: same
blank:
same: logical
`
	_ = yaml.Unmarshal([]byte(data), &root)
	return root.Content[0]
}
