package shared

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/iostreams"
)

type metadataStateType int

const (
	IssueMetadata metadataStateType = iota
	PRMetadata
)

type IssueMetadataState struct {
	Type metadataStateType

	Draft bool

	Body  string
	Title string

	Metadata   []string
	Reviewers  []string
	Assignees  []string
	Labels     []string
	Projects   []string
	Milestones []string

	MetadataResult *api.RepoMetadataResult

	dirty bool // whether user i/o has modified this
}

func (tb *IssueMetadataState) MarkDirty() {
	tb.dirty = true
}

func (tb *IssueMetadataState) IsDirty() bool {
	return tb.dirty || tb.HasMetadata()
}

func (tb *IssueMetadataState) HasMetadata() bool {
	return len(tb.Reviewers) > 0 ||
		len(tb.Assignees) > 0 ||
		len(tb.Labels) > 0 ||
		len(tb.Projects) > 0 ||
		len(tb.Milestones) > 0
}

func FillFromJSON(io *iostreams.IOStreams, JSONInput string, state *IssueMetadataState) error {
	var data []byte
	var err error
	if strings.HasPrefix(JSONInput, "@") {
		data, err = io.ReadUserFile(JSONInput[1:])
		if err != nil {
			return fmt.Errorf("failed to read file %s: %w", JSONInput[1:], err)
		}
	} else {
		data = []byte(JSONInput)
	}

	err = json.Unmarshal(data, state)
	if err != nil {
		return fmt.Errorf("JSON parsing failure: %w", err)
	}

	return nil
}
