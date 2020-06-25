package newlygit

/*

Ideas
-----

- What PR is this commit from?
- How many commits does X have?
	- How lines has X added removed?
- Who is the author of this commit/PR/Issue?
- Has this PR been merged yet?
- From what release is this PR from?
- How many comments are on this issue?
- How long was this PR open?
- When was this PR opened?
- When was X’s first commit?

*/

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

func NewCmd(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "newlygit",
		Short: "It's like the newlywed game, but with your repo",
		Args:  cobra.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			client, err := f.HttpClient()
			if err != nil {
				panic(err)
			}
			baseRepo, err := f.BaseRepo()
			if err != nil {
				panic(err)
			}
			err = newlygit(client, baseRepo)
			if err != nil {
				panic(err)
			}
			return nil
		},
	}

	return cmd
}

func newlygit(client *http.Client, baseRepo ghrepo.Interface) error {
	rand.Seed(time.Now().UnixNano())
	right := 0
	wrong := 0
	isCorrect, err := commit_pr(client, baseRepo)
	if err != nil {
		return err
	}

	if isCorrect {
		right++
	} else {
		wrong++
	}

	fmt.Printf("\nGame over\n")
	fmt.Printf("You got %d/%d\n", right, right+wrong)

	return nil
}
