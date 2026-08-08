package frontmatter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/cli/cli/v2/internal/skills/source"
	"gopkg.in/yaml.v3"
)

const delimiter = "---"

// Metadata represents the parsed YAML frontmatter of a SKILL.md file.
type Metadata struct {
	Name        string                 `yaml:"name"`
	Description string                 `yaml:"description"`
	License     string                 `yaml:"license,omitempty"`
	Meta        map[string]interface{} `yaml:"metadata,omitempty"`
}

// ParseResult contains the parsed frontmatter and remaining body.
type ParseResult struct {
	Metadata Metadata
	Body     string
	RawYAML  map[string]interface{}
}

// Parse extracts YAML frontmatter from a SKILL.md file.
// Frontmatter is delimited by --- on its own lines.
func Parse(content string) (*ParseResult, error) {
	trimmed := strings.TrimLeft(content, "\r\n")
	if !strings.HasPrefix(trimmed, delimiter) {
		return &ParseResult{Body: content}, nil
	}

	rest := trimmed[len(delimiter):]
	rest = strings.TrimLeft(rest, "\r\n")
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return &ParseResult{Body: content}, nil
	}

	yamlContent := rest[:endIdx]
	body := rest[endIdx+len("\n"+delimiter):]
	body = strings.TrimLeft(body, "\r\n")

	var rawYAML map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &rawYAML); err != nil {
		return nil, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	var meta Metadata
	if err := yaml.Unmarshal([]byte(yamlContent), &meta); err != nil {
		return nil, fmt.Errorf("invalid frontmatter YAML: %w", err)
	}

	return &ParseResult{
		Metadata: meta,
		Body:     body,
		RawYAML:  rawYAML,
	}, nil
}

// InjectGitHubMetadata adds GitHub tracking metadata to the spec-defined
// "metadata" map in frontmatter. Keys are prefixed with "github-" to avoid
// collisions with other tools' metadata.
// pinnedRef is the user's explicit --pin value; empty string means unpinned.
// skillPath is the skill's source path in the repo (e.g. "skills/author/my-skill").
func InjectGitHubMetadata(content string, host, owner, repo, ref, treeSHA, pinnedRef, skillPath string) (string, error) {
	result, err := Parse(content)
	if err != nil {
		return "", err
	}

	if result.RawYAML == nil {
		result.RawYAML = make(map[string]interface{})
	}

	meta, _ := result.RawYAML["metadata"].(map[string]interface{})
	if meta == nil {
		meta = make(map[string]interface{})
	}
	delete(meta, "github-owner")
	meta["github-repo"] = source.BuildRepoURL(host, owner, repo)
	meta["github-ref"] = ref
	delete(meta, "github-sha")
	meta["github-tree-sha"] = treeSHA
	meta["github-path"] = skillPath
	if pinnedRef != "" {
		meta["github-pinned"] = pinnedRef
	} else {
		delete(meta, "github-pinned")
	}
	result.RawYAML["metadata"] = meta

	return Serialize(result.RawYAML, result.Body)
}

// InjectLocalMetadata adds local-source tracking metadata to frontmatter.
// sourcePath is the absolute path to the source skill directory.
func InjectLocalMetadata(content string, sourcePath string) (string, error) {
	result, err := Parse(content)
	if err != nil {
		return "", err
	}

	if result.RawYAML == nil {
		result.RawYAML = make(map[string]interface{})
	}

	meta, _ := result.RawYAML["metadata"].(map[string]interface{})
	if meta == nil {
		meta = make(map[string]interface{})
	}
	delete(meta, "github-owner")
	delete(meta, "github-repo")
	delete(meta, "github-ref")
	delete(meta, "github-sha")
	delete(meta, "github-tree-sha")
	delete(meta, "github-pinned")
	delete(meta, "github-path")
	meta["local-path"] = sourcePath
	result.RawYAML["metadata"] = meta

	return Serialize(result.RawYAML, result.Body)
}

// Serialize writes a frontmatter map and body back to a SKILL.md string.
func Serialize(frontmatter map[string]interface{}, body string) (string, error) {
	var buf bytes.Buffer

	yamlBytes, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("failed to serialize frontmatter: %w", err)
	}

	buf.WriteString(delimiter + "\n")
	buf.Write(yamlBytes)
	buf.WriteString(delimiter + "\n")
	if body != "" {
		buf.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			buf.WriteString("\n")
		}
	}

	return buf.String(), nil
}
