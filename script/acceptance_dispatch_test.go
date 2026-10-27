package main_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNoUpstreamAcceptanceDispatch(t *testing.T) {
	root := repositoryRoot(t)
	var hits []string
	for _, rel := range []string{"script/run-acceptance", "Makefile"} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		require.NoError(t, err)
		for i, line := range strings.Split(string(body), "\n") {
			if acceptanceDispatchTarget(rel, line) {
				hits = append(hits, rel+":"+strconv.Itoa(i+1)+":"+strings.TrimSpace(line))
			}
		}
	}
	require.Empty(t, hits)
}

func acceptanceDispatchTarget(rel, line string) bool {
	if strings.Contains(line, "acceptance.yml") {
		return true
	}
	if !strings.Contains(line, "cli/cli") {
		return false
	}
	if rel == "Makefile" && strings.HasPrefix(strings.TrimSpace(line), "#") {
		return false
	}
	return true
}
