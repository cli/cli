package cliutil

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJSONFiles(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		limit int64
		err   string
	}{
		{name: "object", input: "{\n\"value\":1\n}\n", limit: 100},
		{name: "too large", input: `{"value":1}`, limit: 3, err: "bounded regular"},
		{name: "invalid JSON", input: "{", limit: 100, err: "decode JSON"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "input.json")
			require.NoError(t, os.WriteFile(path, []byte(tc.input), 0o600))
			var value map[string]int
			raw, err := ReadJSON(path, tc.limit, &value)
			if tc.err != "" {
				require.ErrorContains(t, err, tc.err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.input, string(raw))
			output := filepath.Join(filepath.Dir(path), "output.json")
			require.NoError(t, WriteJSON(output, value))
			var actual map[string]int
			_, err = ReadJSON(output, 100, &actual)
			require.NoError(t, err)
			require.Equal(t, value, actual)
			info, err := os.Stat(output)
			require.NoError(t, err)
			require.Zero(t, info.Mode().Perm()&0o077)
		})
	}
}

func TestResponses(t *testing.T) {
	var output bytes.Buffer
	require.NoError(t, Error(&output, "blocked", "The requested operation is unavailable."))
	var response struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &response))
	require.False(t, response.OK)
	require.Equal(t, "blocked", response.Error.Code)
}
