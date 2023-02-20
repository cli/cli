package shared

import (
	"strings"

	"github.com/cli/cli/v2/internal/config"
)

func AuthTokenWriteable(cfg config.Config, hostname string) (string, bool) {
	token, src := cfg.AuthToken(hostname)
	return src, (token == "" || !strings.HasSuffix(src, "_TOKEN"))
}
