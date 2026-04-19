package contributions

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/v2/api"
)

// ContributionDay represents a single day's contributions in the calendar.
type ContributionDay struct {
	Date              string `json:"date"`
	ContributionCount int    `json:"contributionCount"`
	ContributionLevel string `json:"contributionLevel"`
}

// ContributionWeek is a single week (up to 7 days) in the calendar.
type ContributionWeek struct {
	ContributionDays []ContributionDay `json:"contributionDays"`
}

// ContributionCalendar holds the full calendar data returned by the API.
type ContributionCalendar struct {
	TotalContributions int                `json:"totalContributions"`
	Weeks              []ContributionWeek `json:"weeks"`
}

// ContributionsResult bundles the calendar with the resolved login.
type ContributionsResult struct {
	Login    string               `json:"login"`
	From     time.Time            `json:"from"`
	To       time.Time            `json:"to"`
	Calendar ContributionCalendar `json:"calendar"`
}

const calendarFragment = `
fragment Calendar on ContributionsCollection {
	contributionCalendar {
		totalContributions
		weeks {
			contributionDays {
				date
				contributionCount
				contributionLevel
			}
		}
	}
}
`

// fetchContributions retrieves the contribution calendar for the given user
// (or the authenticated viewer if login is empty). from/to may be zero values.
func fetchContributions(client *http.Client, hostname, login string, from, to time.Time) (*ContributionsResult, error) {
	c := api.NewClientFromHTTP(client)

	variables := map[string]interface{}{}
	if !from.IsZero() {
		variables["from"] = from.Format(time.RFC3339)
	}
	if !to.IsZero() {
		variables["to"] = to.Format(time.RFC3339)
	}

	if login == "" {
		query := calendarFragment + `
		query ViewerContributions($from: DateTime, $to: DateTime) {
			viewer {
				login
				contributionsCollection(from: $from, to: $to) {
					...Calendar
				}
			}
		}`
		var resp struct {
			Viewer struct {
				Login                   string
				ContributionsCollection struct {
					ContributionCalendar ContributionCalendar
				}
			}
		}
		if err := c.GraphQL(hostname, query, variables, &resp); err != nil {
			return nil, err
		}
		return &ContributionsResult{
			Login:    resp.Viewer.Login,
			From:     from,
			To:       to,
			Calendar: resp.Viewer.ContributionsCollection.ContributionCalendar,
		}, nil
	}

	query := calendarFragment + `
	query UserContributions($login: String!, $from: DateTime, $to: DateTime) {
		user(login: $login) {
			login
			contributionsCollection(from: $from, to: $to) {
				...Calendar
			}
		}
	}`
	variables["login"] = login

	var resp struct {
		User *struct {
			Login                   string
			ContributionsCollection struct {
				ContributionCalendar ContributionCalendar
			}
		}
	}
	if err := c.GraphQL(hostname, query, variables, &resp); err != nil {
		return nil, err
	}
	if resp.User == nil {
		return nil, fmt.Errorf("could not find user %q", login)
	}
	return &ContributionsResult{
		Login:    resp.User.Login,
		From:     from,
		To:       to,
		Calendar: resp.User.ContributionsCollection.ContributionCalendar,
	}, nil
}
