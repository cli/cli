package version

import (
	"fmt"

	"github.com/github/gh/git"
)

var Version = "2.12.7"

func FullVersion() (string, error) {
	gitVersion, err := git.Version()
	if err != nil {
		gitVersion = "git version (unavailable)"
	}
	return fmt.Sprintf("%s\nhub version %s", gitVersion, Version), err
}
