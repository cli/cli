package api

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/utils"
)

// TODO
func apiClientFromContext(token string) *http.Client {
	var opts []api.ClientOption
	if verbose := os.Getenv("DEBUG"); verbose != "" {
		opts = append(opts, apiVerboseLog())
	}
	opts = append(opts,
		api.AddHeader("Authorization", fmt.Sprintf("token %s", token)),
		// FIXME
		// api.AddHeader("User-Agent", fmt.Sprintf("GitHub CLI %s", command.Version)),
		// antiope-preview: Checks
		api.AddHeader("Accept", "application/vnd.github.antiope-preview+json"),
	)
	return api.NewHTTPClient(opts...)
}

// TODO
func apiVerboseLog() api.ClientOption {
	logTraffic := strings.Contains(os.Getenv("DEBUG"), "api")
	colorize := utils.IsTerminal(os.Stderr)
	return api.VerboseLog(utils.NewColorable(os.Stderr), logTraffic, colorize)
}
