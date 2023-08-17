package shared

import (
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/stretchr/testify/assert"
)

type metadataFetcher struct {
	metadataResult *api.RepoMetadataResult
}

func (mf *metadataFetcher) RepoMetadataFetch(input api.RepoMetadataInput) (*api.RepoMetadataResult, error) {
	return mf.metadataResult, nil
}

func TestMetadataSurvey_selectAll(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	repo := ghrepo.New("OWNER", "REPO")

	fetcher := &metadataFetcher{
		metadataResult: &api.RepoMetadataResult{
			AssignableUsers: []api.RepoAssignee{
				{Login: "hubot"},
				{Login: "monalisa"},
			},
			Labels: []api.RepoLabel{
				{Name: "help wanted"},
				{Name: "good first issue"},
			},
			Projects: []api.RepoProject{
				{Name: "Huge Refactoring"},
				{Name: "The road to 1.0"},
			},
			Milestones: []api.RepoMilestone{
				{Title: "1.2 patch release"},
			},
		},
	}

	pm := prompter.NewMockPrompter(t)
	pm.RegisterMultiSelect("What would you like to add?",
		[]string{}, []string{"Reviewers", "Assignees", "Labels", "Projects", "Milestone"}, func(_ string, _, _ []string) ([]int, error) {
			return []int{0, 1, 2, 3, 4}, nil
		})
	pm.RegisterMultiSelect("Reviewers", []string{}, []string{"hubot", "monalisa"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1}, nil
	})
	pm.RegisterMultiSelect("Assignees", []string{}, []string{"hubot", "monalisa"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{0}, nil
	})
	pm.RegisterMultiSelect("Labels", []string{}, []string{"help wanted", "good first issue"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1}, nil
	})
	pm.RegisterMultiSelect("Projects", []string{}, []string{"Huge Refactoring", "The road to 1.0"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1}, nil
	})
	pm.RegisterSelect("Milestone", []string{"(none)", "1.2 patch release"}, func(_, _ string, _ []string) (int, error) {
		return 0, nil
	})

	state := &IssueMetadataState{
		Assignees: []string{"hubot"},
		Type:      PRMetadata,
	}
	err := MetadataSurvey(pm, ios, repo, fetcher, state)
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())

	assert.Equal(t, []string{"hubot"}, state.Assignees)
	assert.Equal(t, []string{"monalisa"}, state.Reviewers)
	assert.Equal(t, []string{"good first issue"}, state.Labels)
	assert.Equal(t, []string{"The road to 1.0"}, state.Projects)
	assert.Equal(t, []string{}, state.Milestones)
}

func TestMetadataSurvey_keepExisting(t *testing.T) {
	ios, _, stdout, stderr := iostreams.Test()

	repo := ghrepo.New("OWNER", "REPO")

	fetcher := &metadataFetcher{
		metadataResult: &api.RepoMetadataResult{
			Labels: []api.RepoLabel{
				{Name: "help wanted"},
				{Name: "good first issue"},
			},
			Projects: []api.RepoProject{
				{Name: "Huge Refactoring"},
				{Name: "The road to 1.0"},
			},
		},
	}

	pm := prompter.NewMockPrompter(t)
	pm.RegisterMultiSelect("What would you like to add?", []string{}, []string{"Assignees", "Labels", "Projects", "Milestone"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1, 2}, nil
	})
	pm.RegisterMultiSelect("Labels", []string{}, []string{"help wanted", "good first issue"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1}, nil
	})
	pm.RegisterMultiSelect("Projects", []string{}, []string{"Huge Refactoring", "The road to 1.0"}, func(_ string, _, _ []string) ([]int, error) {
		return []int{1}, nil
	})

	state := &IssueMetadataState{
		Assignees: []string{"hubot"},
	}
	err := MetadataSurvey(pm, ios, repo, fetcher, state)
	assert.NoError(t, err)

	assert.Equal(t, "", stdout.String())
	assert.Equal(t, "", stderr.String())

	assert.Equal(t, []string{"hubot"}, state.Assignees)
	assert.Equal(t, []string{"good first issue"}, state.Labels)
	assert.Equal(t, []string{"The road to 1.0"}, state.Projects)
}
