package format

import (
	"testing"

	"github.com/cli/cli/v2/pkg/cmd/project/shared/queries"
	"github.com/stretchr/testify/assert"
)

func TestProjectState(t *testing.T) {
	assert.Equal(t, "open", ProjectState(queries.Project{}))
	assert.Equal(t, "closed", ProjectState(queries.Project{Closed: true}))
}

func TestColorForProjectState(t *testing.T) {
	assert.Equal(t, "green", ColorForProjectState(queries.Project{}))
	assert.Equal(t, "gray", ColorForProjectState(queries.Project{Closed: true}))
}
