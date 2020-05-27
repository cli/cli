package command

import (
	"testing"
)

func TestSearchRepo(t *testing.T) {
	_, err := RunCommand(`search repo "docker"`)
	eq(t, err, nil)
}
