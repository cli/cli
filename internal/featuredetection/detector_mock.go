package featuredetection

import "github.com/cli/cli/v2/internal/gh"

type DisabledDetectorMock struct{}

func (md *DisabledDetectorMock) IssueFeatures() (IssueFeatures, error) {
	return IssueFeatures{}, nil
}

func (md *DisabledDetectorMock) PullRequestFeatures() (PullRequestFeatures, error) {
	return PullRequestFeatures{}, nil
}

func (md *DisabledDetectorMock) RepositoryFeatures() (RepositoryFeatures, error) {
	return RepositoryFeatures{}, nil
}

func (md *DisabledDetectorMock) ProjectsV1() gh.ProjectsV1Support {
	return gh.ProjectsV1Unsupported
}

type EnabledDetectorMock struct{}

func (md *EnabledDetectorMock) IssueFeatures() (IssueFeatures, error) {
	return allIssueFeatures, nil
}

func (md *EnabledDetectorMock) PullRequestFeatures() (PullRequestFeatures, error) {
	return allPullRequestFeatures, nil
}

func (md *EnabledDetectorMock) RepositoryFeatures() (RepositoryFeatures, error) {
	return allRepositoryFeatures, nil
}

func (md *EnabledDetectorMock) ProjectsV1() gh.ProjectsV1Support {
	return gh.ProjectsV1Supported
}
