package iostreams

import "os"

func hasAlternateScreenBuffer(_ bool) bool {
	return os.Getenv("TERM") != "dumb"
}
