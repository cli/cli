package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSkillMetadata(t *testing.T) {
	root, err := filepath.Abs("..")
	require.NoError(t, err)
	internal, err := filepath.Glob(filepath.Join(root, "internal", "*", "SKILL.md"))
	require.NoError(t, err)
	files := append([]string{filepath.Join(root, "SKILL.md")}, internal...)
	require.Len(t, files, 6)
	for _, file := range files {
		t.Run(filepath.Base(filepath.Dir(file)), func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			text := string(data)
			require.True(t, strings.HasPrefix(text, "---\n"))
			parts := strings.SplitN(text, "---", 3)
			require.Len(t, parts, 3)
			fields := map[string]string{}
			for line := range strings.SplitSeq(strings.TrimSpace(parts[1]), "\n") {
				key, value, found := strings.Cut(line, ":")
				require.True(t, found)
				fields[key] = strings.TrimSpace(value)
			}
			require.Equal(t, filepath.Base(filepath.Dir(file)), fields["name"])
			require.Regexp(t, `^[a-z0-9]+(?:-[a-z0-9]+)*$`, fields["name"])
			require.LessOrEqual(t, len(fields["name"]), 64)
			require.NotEmpty(t, fields["description"])
			require.LessOrEqual(t, len(fields["description"]), 1024)
			require.LessOrEqual(t, len(fields["compatibility"]), 500)
			require.NotContains(t, fields, "allowed-tools")
			require.NotContains(t, fields, "disable-model-invocation")
			require.Less(t, len(strings.Split(parts[2], "\n")), 500)
			require.NotContains(t, text, "\u2014")
		})
	}
}

func TestSkillResources(t *testing.T) {
	root := ".."
	links := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	entry, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	require.NoError(t, err)
	indexed := map[string]bool{}
	for _, match := range links.FindAllStringSubmatch(string(entry), -1) {
		indexed[strings.Split(match[1], "#")[0]] = true
	}
	err = filepath.WalkDir(root, func(path string, item os.DirEntry, walkErr error) error {
		require.NoError(t, walkErr)
		if item.IsDir() {
			if slices.Contains([]string{"node_modules", ".git", "__pycache__"}, item.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		require.NoError(t, err)
		relative = filepath.ToSlash(relative)
		if slices.Contains([]string{".js", ".mjs", ".cjs"}, filepath.Ext(path)) {
			require.Equal(t, "scripts/terminal.mjs", relative,
				"Test code and fixtures must remain Go; JavaScript is limited to the Tuistory boundary.")
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		if strings.HasPrefix(relative, "references/") || strings.HasPrefix(relative, "internal/") {
			require.True(t, indexed[relative], "Resource is not directly indexed: %s", relative)
		}
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotContains(t, string(data), "\u2014")
		for _, match := range links.FindAllStringSubmatch(string(data), -1) {
			target := match[1]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") {
				continue
			}
			target = strings.Split(target, "#")[0]
			_, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.FromSlash(target)))
			require.NoError(t, err, "Broken reference in %s", path)
		}
		return nil
	})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, "tests", "skill-evaluations.json"))
	require.NoError(t, err)
	var evaluations struct {
		Cases []struct {
			ID       string   `json:"id"`
			Input    string   `json:"input"`
			Setup    string   `json:"setup"`
			Expected []string `json:"expected"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(data, &evaluations))
	require.GreaterOrEqual(t, len(evaluations.Cases), 3)
	seen := map[string]bool{}
	for _, entry := range evaluations.Cases {
		require.False(t, seen[entry.ID])
		seen[entry.ID] = true
		require.NotEmpty(t, entry.Input)
		require.NotEmpty(t, entry.Setup)
		require.NotEmpty(t, entry.Expected)
	}
}
