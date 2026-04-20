// Package ci provides helpers for detecting CI/CD execution environments.
package ci

import "os"

// IsCI determines if the current execution context is within a known CI/CD system.
// This is based on https://github.com/watson/ci-info/blob/HEAD/index.js.
func IsCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}

// IsGitHubActions determines if the current execution context is within GitHub Actions.
// GitHub Actions sets the GITHUB_ACTIONS environment variable to "true" for all steps.
// See https://docs.github.com/en/actions/learn-github-actions/variables#default-environment-variables.
func IsGitHubActions() bool {
	return os.Getenv("GITHUB_ACTIONS") == "true"
}
