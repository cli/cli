package version

import (
	"fmt"
)

var Version = "0.0.0"

func FullVersion() (string, error) {
	return fmt.Sprintf("gh version %s", Version), nil
}
