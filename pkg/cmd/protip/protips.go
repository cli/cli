package protip

import (
	"math/rand"
	"time"
)

func getProtip() []string {
	rand.Seed(time.Now().UnixNano())
	tips := [][]string{
		{
			"To merge a pull request, review it",
			"until it is approved",
		},
		{
			"To have gh use a different editor you can run:",
			"gh config set editor <name of editor>",
		},
		{
			"If you prefer to use gh over ssh you can run:",
			"gh config set git_protocol ssh",
		},
		{
			"You can search through issues using grep:",
			"gh issue list -L200 | grep 'search term'",
		},
	}

	return tips[rand.Intn(len(tips))]
}
