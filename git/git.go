package git

import (
	"context"
)

func GitCommand(args ...string) (*gitCommand, error) {
	c := &Client{}
	return c.Command(context.Background(), args...)
}
