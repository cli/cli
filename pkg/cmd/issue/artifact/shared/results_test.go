package shared

import (
	"errors"
	"net/url"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/pkg/iostreams"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintSuccess(t *testing.T) {
	tests := []struct {
		name       string
		tty        bool
		status     string
		result     Result
		message    string
		wantStdout string
	}{
		{
			name:       "in a terminal, the icon and message go to stdout",
			tty:        true,
			status:     "deleted",
			result:     Result{Number: 2, Name: "OAuth callback plan"},
			message:    "Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142",
			wantStdout: "✓ Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142\n",
		},
		{
			name:       "piped, every column is printed, with an empty source and reason",
			status:     "deleted",
			result:     Result{Number: 2, Name: "OAuth callback plan"},
			message:    "Deleted artifact 2 (OAuth callback plan) from monalisa/monas-cafe#142",
			wantStdout: "deleted\t2\tOAuth callback plan\t\t\n",
		},
		{
			name:       "piped, with a source",
			status:     "created",
			result:     Result{Number: 6, Name: "signin-plan.md", Source: "docs/signin-plan.md"},
			message:    "Created artifact 6 (signin-plan.md) on monalisa/monas-cafe#142",
			wantStdout: "created\t6\tsignin-plan.md\tdocs/signin-plan.md\t\n",
		},
		{
			name:       "piped, the name is printed as given, since a path can hold any spaces",
			status:     "written",
			result:     Result{Number: 2, Name: "cafe  notes/2-OAuth-callback-plan.md"},
			message:    "cafe  notes/2-OAuth-callback-plan.md",
			wantStdout: "written\t2\tcafe  notes/2-OAuth-callback-plan.md\t\t\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)

			PrintSuccess(ios, tt.status, tt.result, "✓", tt.message)

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, "", stderr.String())
		})
	}
}

func TestPrintFailure(t *testing.T) {
	artifactURL := func(number string) *url.URL {
		u, err := url.Parse("https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/" + number)
		require.NoError(t, err)
		return u
	}
	apiError := func(status int, message string, number string) error {
		return api.HTTPError{HTTPError: &ghAPI.HTTPError{StatusCode: status, Message: message, RequestURL: artifactURL(number)}}
	}

	tests := []struct {
		name       string
		tty        bool
		color      bool
		result     Result
		action     string
		err        error
		wantStdout string
		wantStderr string
	}{
		{
			name:       "in a terminal, the server's message goes to stderr",
			tty:        true,
			result:     Result{Number: 9},
			action:     "delete artifact 9",
			err:        apiError(404, "Not Found", "9"),
			wantStderr: "X Failed to delete artifact 9: Not Found\n",
		},
		{
			name:       "colors in a terminal",
			tty:        true,
			color:      true,
			result:     Result{Number: 9},
			action:     "delete artifact 9",
			err:        apiError(404, "Not Found", "9"),
			wantStderr: "\x1b[0;31mX\x1b[0m Failed to delete artifact 9: Not Found\n",
		},
		{
			name:       "piped, a failed line goes to stdout",
			result:     Result{Number: 9},
			action:     "delete artifact 9",
			err:        apiError(404, "Not Found", "9"),
			wantStdout: "failed\t9\t\t\tNot Found\n",
		},
		{
			name:       "piped, with the artifact's name",
			result:     Result{Number: 3, Name: "Staging OAuth runbook"},
			action:     "delete artifact 3",
			err:        apiError(403, "Must have admin rights to Repository.", "3"),
			wantStdout: "failed\t3\tStaging OAuth runbook\t\tMust have admin rights to Repository.\n",
		},
		{
			name:       "piped, an artifact without a number leaves its column empty",
			result:     Result{Name: "oauth-research.md", Source: "oauth-research.md"},
			action:     "create artifact from oauth-research.md",
			err:        apiError(503, "Service Unavailable", "7"),
			wantStdout: "failed\t\toauth-research.md\toauth-research.md\tService Unavailable\n",
		},
		{
			name:       "a message on several lines stays on one line",
			result:     Result{Number: 2, Name: "OAuth callback plan"},
			action:     "delete artifact 2",
			err:        apiError(422, "Validation Failed\nname is invalid", "2"),
			wantStdout: "failed\t2\tOAuth callback plan\t\tValidation Failed name is invalid\n",
		},
		{
			name:       "an API error without a message gives the whole error",
			tty:        true,
			result:     Result{Number: 2},
			action:     "delete artifact 2",
			err:        apiError(502, "", "2"),
			wantStderr: "X Failed to delete artifact 2: HTTP 502 (https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/2)\n",
		},
		{
			name:       "an error that isn't from the API gives its own text",
			tty:        true,
			result:     Result{Number: 2},
			action:     "delete artifact 2",
			err:        errors.New(`Delete "https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/2": dial tcp: lookup api.github.com: no such host`),
			wantStderr: `X Failed to delete artifact 2: Delete "https://api.github.com/repos/monalisa/monas-cafe/issues/142/artifacts/2": dial tcp: lookup api.github.com: no such host` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ios, _, stdout, stderr := iostreams.Test()
			ios.SetStdinTTY(tt.tty)
			ios.SetStdoutTTY(tt.tty)
			ios.SetStderrTTY(tt.tty)
			ios.SetColorEnabled(tt.color)

			PrintFailure(ios, tt.result, tt.action, tt.err)

			assert.Equal(t, tt.wantStdout, stdout.String())
			assert.Equal(t, tt.wantStderr, stderr.String())
		})
	}
}
