package shared

import (
	"regexp"
	"strings"
)

var invalidCharactersRE = regexp.MustCompile(`[^\w._-]+`)

// NormalizeRepoName takes in the repo name the user inputted and normalizes it using the same logic as GitHub (GitHub.com/new)
func NormalizeRepoName(repoName string) string {
	newName := invalidCharactersRE.ReplaceAllString(repoName, "-")
	return strings.TrimSuffix(newName, ".git")
}
