package context

import (
	"fmt"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/utils"
)

func FormattedInfo(c Context, parentRepo string) (string, error) {
	baseRepo, err := c.BaseRepo()
	if err != nil {
		return "", err
	}
	authLogin, err := c.AuthLogin()
	if err != nil {
		return "", err
	}

	forkInfo := ""
	if parentRepo != "" {
		forkInfo = fmt.Sprintf("(fork of %s) ", parentRepo)
	}

	out := fmt.Sprintf("%s%s %s%s%s",
		utils.Gray("in "),
		utils.Magenta(ghrepo.FullName(baseRepo)),
		utils.Yellow(forkInfo),
		utils.Gray("as "),
		utils.Magenta(authLogin),
	)

	return out, nil
}
