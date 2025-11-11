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

func (md *DisabledDetectorMock) SearchFeatures() (SearchFeatures, error) {
	return advancedIssueSearchNotSupported, nil
}

func (md *DisabledDetectorMock) ReleaseFeatures() (ReleaseFeatures, error) {
	return ReleaseFeatures{}, nil
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

func (md *EnabledDetectorMock) SearchFeatures() (SearchFeatures, error) {
	return advancedIssueSearchNotSupported, nil
}

func (md *EnabledDetectorMock) ReleaseFeatures() (ReleaseFeatures, error) {
	return ReleaseFeatures{
		ImmutableReleases: true,
	}, nil
}

type AdvancedIssueSearchDetectorMock struct {
	EnabledDetectorMock
	searchFeatures SearchFeatures
}

func (md *AdvancedIssueSearchDetectorMock) SearchFeatures() (SearchFeatures, error) {
	return md.searchFeatures, nil
}

func AdvancedIssueSearchUnsupported() *AdvancedIssueSearchDetectorMock {
	return &AdvancedIssueSearchDetectorMock{
		searchFeatures: advancedIssueSearchNotSupported,
	}
}

func AdvancedIssueSearchSupportedAsOptIn() *AdvancedIssueSearchDetectorMock {
	return &AdvancedIssueSearchDetectorMock{
		searchFeatures: advancedIssueSearchSupportedAsOptIn,
	}
}

func AdvancedIssueSearchSupportedAsOnlyBackend() *AdvancedIssueSearchDetectorMock {
	return &AdvancedIssueSearchDetectorMock{
		searchFeatures: advancedIssueSearchSupportedAsOnlyBackend,
	}
}
