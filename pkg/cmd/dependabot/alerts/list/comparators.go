package list

import (
	dependabot "github.com/cli/cli/v2/pkg/cmd/dependabot/shared"
)

type byCreatedAt []dependabot.VulnerabilityAlert

func (a byCreatedAt) Len() int           { return len(a) }
func (a byCreatedAt) Less(i, j int) bool { return a[i].CreatedAt.Before(a[j].CreatedAt) }
func (a byCreatedAt) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }

type bySeverity []dependabot.VulnerabilityAlert

func (a bySeverity) Len() int { return len(a) }
func (a bySeverity) Less(i, j int) bool {
	return a[i].SecurityVulnerability.Severity < a[j].SecurityVulnerability.Severity
}
func (a bySeverity) Swap(i, j int) { a[i], a[j] = a[j], a[i] }

type byNumber []dependabot.VulnerabilityAlert

func (a byNumber) Len() int           { return len(a) }
func (a byNumber) Less(i, j int) bool { return a[i].Number < a[j].Number }
func (a byNumber) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
