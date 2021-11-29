package config

import (
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
			assert.Equal(t, tt.output, out.ValueNode.Value)
		})
	}
}

func TestEmpty(t *testing.T) {
	cm := ConfigMap{}
	assert.Equal(t, true, cm.Empty())
	cm.Root = &yaml.Node{
		Content: []*yaml.Node{
			{
				Value: "test",
			},
		},
	}
	assert.Equal(t, false, cm.Empty())
}

func TestGetStringValue(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		wantValue string
		wantErr   bool
	}{
		{
			name:      "get key",
			key:       "valid",
			wantValue: "present",
		},
		{
			name:    "get key that is not present",
			key:     "invalid",
			wantErr: true,
		},
		{
			name:      "get key that has same content as a value",
			key:       "same",
			wantValue: "logical",
		},
	}

	for _, tt := range tests {
		cm := ConfigMap{Root: testYaml()}
		t.Run(tt.name, func(t *testing.T) {
			val, err := cm.GetStringValue(tt.key)
			if tt.wantErr {
				assert.EqualError(t, err, "not found")
				return
			}
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

func TestSetStringValue(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "set key that is not present",
			key:   "notPresent",
			value: "test1",
		},
		{
			name:  "set key that is present",
			key:   "erroneous",
			value: "test2",
		},
		{
			name:  "set key that is blank",
			key:   "blank",
			value: "test3",
		},
		{
			name:  "set key that has same content as a value",
			key:   "present",
			value: "test4",
		},
	}

	for _, tt := range tests {
		cm := ConfigMap{Root: testYaml()}
		t.Run(tt.name, func(t *testing.T) {
			err := cm.SetStringValue(tt.key, tt.value)
			assert.NoError(t, err)
			val, err := cm.GetStringValue(tt.key)
			assert.NoError(t, err)
			assert.Equal(t, tt.value, val)
		})
	}
}

func TestRemoveEntry(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		wantLength int
	}{
		{
			name:       "remove key",
			key:        "erroneous",
			wantLength: 6,
		},
		{
			name:       "remove key that is not present",
			key:        "invalid",
			wantLength: 8,
		},
		{
			name:       "remove key that has same content as a value",
			key:        "same",
			wantLength: 6,
		},
	}

	for _, tt := range tests {
		cm := ConfigMap{Root: testYaml()}
		t.Run(tt.name, func(t *testing.T) {
			cm.RemoveEntry(tt.key)
			assert.Equal(t, tt.wantLength, len(cm.Root.Content))
			_, err := cm.FindEntry(tt.key)
			assert.EqualError(t, err, "not found")
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
