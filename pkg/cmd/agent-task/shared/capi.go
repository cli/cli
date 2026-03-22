package shared

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/cmd/agent-task/capi"
	prShared "github.com/cli/cli/v2/pkg/cmd/pr/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
)

const uuidPattern = `[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}`

var sessionIDRegexp = regexp.MustCompile(fmt.Sprintf("^%s$", uuidPattern))
var agentSessionURLRegexp = regexp.MustCompile(fmt.Sprintf("^/agent-sessions/(%s)$", uuidPattern))

func CapiClientFunc(f *cmdutil.Factory) func() (capi.CapiClient, error) {
	return func() (capi.CapiClient, error) {
		cfg, err := f.Config()
		if err != nil {
			return nil, err
		}

		httpClient, err := f.HttpClient()
		if err != nil {
			return nil, err
		}

		authCfg := cfg.Authentication()
		host, _ := authCfg.DefaultHost()
		token, _ := authCfg.ActiveToken(host)

		cachedClient := api.NewCachedHTTPClient(httpClient, time.Minute*10)
		capiBaseURL, err := resolveCapiURL(cachedClient, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve Copilot API URL: %w", err)
		}

		return capi.NewCAPIClient(httpClient, token, host, capiBaseURL), nil
	}
}

// resolveCapiURL queries the GitHub API for the Copilot API endpoint URL.
func resolveCapiURL(httpClient *http.Client, host string) (string, error) {
	apiClient := api.NewClientFromHTTP(httpClient)

	var resp struct {
		Viewer struct {
			CopilotEndpoints struct {
				Api string `graphql:"api"`
			} `graphql:"copilotEndpoints"`
		} `graphql:"viewer"`
	}

	if err := apiClient.Query(host, "CopilotEndpoints", &resp, nil); err != nil {
		return "", err
	}

	if resp.Viewer.CopilotEndpoints.Api == "" {
		return "", errors.New("empty Copilot API URL returned")
	}

	return resp.Viewer.CopilotEndpoints.Api, nil
}

func IsSessionID(s string) bool {
	return sessionIDRegexp.MatchString(s)
}

// ParseSessionIDFromURL parses session ID from a pull request's agent session
// URL, which is of the form:
//
//	`https://github.com/OWNER/REPO/pull/NUMBER/agent-sessions/SESSION-ID`
func ParseSessionIDFromURL(u string) (string, error) {
	_, _, rest, err := prShared.ParseURL(u)
	if err != nil {
		return "", err
	}

	match := agentSessionURLRegexp.FindStringSubmatch(rest)
	if match == nil {
		return "", errors.New("not a valid agent session URL")
	}
	return match[1], nil
}
