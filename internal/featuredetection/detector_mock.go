package featuredetection

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
