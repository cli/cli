package view

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

// Maybe I should be testing the issueProjectList function instead...
func Test_printHumanIssuePreview(t *testing.T) {
	tests := map[string]struct {
		opts           *ViewOptions
		baseRepo       ghrepo.Interface
		issueFixture   string
		expectedOutput string
	}{
		"projectcards included (v1 projects are supported)": {
			opts:         &ViewOptions{},
			baseRepo:     ghrepo.New("OWNER", "REPO"),
			issueFixture: "./fixtures/issueView_v1ProjectsEnabled.json",
			// This is super fragile but it's working for now
			expectedOutput: "ix of coins OWNER/REPO#123\nOpen • marseilles opened about 1 day ago • 9 comments\nProjects: Project 1 (Column name)\nMilestone: \n\n\n  **bold story**                                                              \n\n\n———————— Not showing 9 comments ————————\n\n\nUse --comments to view the full conversation\nView this issue on GitHub: https://github.com/OWNER/REPO/issues/123\n",
		},
		"projectcards aren't included (v1 projects are supported)": {
			opts:         &ViewOptions{},
			baseRepo:     ghrepo.New("OWNER", "REPO"),
			issueFixture: "./fixtures/issueView_v1ProjectsDisabled.json",
			// This is super fragile but it's working for now
			expectedOutput: "ix of coins OWNER/REPO#123\nOpen • marseilles opened about 1 day ago • 9 comments\nMilestone: \n\n\n  **bold story**                                                              \n\n\n———————— Not showing 9 comments ————————\n\n\nUse --comments to view the full conversation\nView this issue on GitHub: https://github.com/OWNER/REPO/issues/123\n",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ios, _, stdout, _ := iostreams.Test()
			ios.SetStdoutTTY(true)
			ios.SetStdinTTY(true)
			ios.SetStderrTTY(true)
			tc.opts.IO = ios

			issue := &api.Issue{}

			f, err := os.Open(tc.issueFixture)
			if err != nil {
				t.Errorf("Error opening fixture at %s: %v", tc.issueFixture, err)
			}
			defer f.Close()

			dec := json.NewDecoder(f)
			err = dec.Decode(&issue)
			if err != nil {
				t.Errorf("Error decoding fixture at %s: %v", tc.issueFixture, err)
			}

			// This is a hack to fix the time for outputs
			tc.opts.Now = func() time.Time {
				return issue.CreatedAt.Add(24 * time.Hour)
			}

			issuePrint := &IssuePrint{
				issue:       issue,
				colorScheme: ios.ColorScheme(),
				IO:          ios,
				time:        tc.opts.Now(),
				baseRepo:    tc.baseRepo,
			}

			err = issuePrint.humanPreview(!tc.opts.Comments)
			if err != nil {
				assert.NoError(t, err)
			}
			assert.Equal(t, tc.expectedOutput, stdout.String())
		})
	}
}
