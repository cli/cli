package shared

import (
	"context"
	"net/http"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	graphql "github.com/cli/shurcooL-graphql"
	"github.com/shurcooL/githubv4"
	"golang.org/x/sync/errgroup"
)

func UpdateIssue(httpClient *http.Client, repo ghrepo.Interface, id string, isPR bool, options Editable) error {
	var wg errgroup.Group

	// Labels are updated through discrete mutations to avoid having to replace the entire list of labels
	// and risking race conditions.
	if options.Labels.Edited {
		if len(options.Labels.Add) > 0 {
			wg.Go(func() error {
				addedLabelIds, err := options.Metadata.LabelsToIDs(options.Labels.Add)
				if err != nil {
					return err
				}
				return addLabels(httpClient, id, repo, addedLabelIds)
			})
		}
		if len(options.Labels.Remove) > 0 {
			wg.Go(func() error {
				removeLabelIds, err := options.Metadata.LabelsToIDs(options.Labels.Remove)
				if err != nil {
					return err
				}
				return removeLabels(httpClient, id, repo, removeLabelIds)
			})
		}
	}

	if dirtyExcludingLabels(options) {
		wg.Go(func() error {
			return replaceIssueFields(httpClient, repo, id, isPR, options)
		})
	}

	return wg.Wait()
}

func replaceIssueFields(httpClient *http.Client, repo ghrepo.Interface, id string, isPR bool, options Editable) error {
	apiClient := api.NewClientFromHTTP(httpClient)
	assigneeIds, err := options.AssigneeIds(apiClient, repo)
	if err != nil {
		return err
	}

	projectIds, err := options.ProjectIds()
	if err != nil {
		return err
	}

	milestoneId, err := options.MilestoneId()
	if err != nil {
		return err
	}

	if isPR {
		params := githubv4.UpdatePullRequestInput{
			PullRequestID: id,
			Title:         ghString(options.TitleValue()),
			Body:          ghString(options.BodyValue()),
			AssigneeIDs:   ghIds(assigneeIds),
			ProjectIDs:    ghIds(projectIds),
			MilestoneID:   ghId(milestoneId),
		}
		if options.Base.Edited {
			params.BaseRefName = ghString(&options.Base.Value)
		}
		return updatePullRequest(httpClient, repo, params)
	}

	params := githubv4.UpdateIssueInput{
		ID:          id,
		Title:       ghString(options.TitleValue()),
		Body:        ghString(options.BodyValue()),
		AssigneeIDs: ghIds(assigneeIds),
		ProjectIDs:  ghIds(projectIds),
		MilestoneID: ghId(milestoneId),
	}
	return updateIssue(httpClient, repo, params)
}

func dirtyExcludingLabels(e Editable) bool {
	return e.Title.Edited ||
		e.Body.Edited ||
		e.Base.Edited ||
		e.Reviewers.Edited ||
		e.Assignees.Edited ||
		e.Projects.Edited ||
		e.Milestone.Edited
}

func addLabels(httpClient *http.Client, id string, repo ghrepo.Interface, labels []string) error {
	params := githubv4.AddLabelsToLabelableInput{
		LabelableID: id,
		LabelIDs:    *ghIds(&labels),
	}

	var mutation struct {
		AddLabelsToLabelable struct {
			Typename string `graphql:"__typename"`
		} `graphql:"addLabelsToLabelable(input: $input)"`
	}

	variables := map[string]interface{}{"input": params}
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	return gql.MutateNamed(context.Background(), "LabelAdd", &mutation, variables)
}

func removeLabels(httpClient *http.Client, id string, repo ghrepo.Interface, labels []string) error {
	params := githubv4.RemoveLabelsFromLabelableInput{
		LabelableID: id,
		LabelIDs:    *ghIds(&labels),
	}

	var mutation struct {
		RemoveLabelsFromLabelable struct {
			Typename string `graphql:"__typename"`
		} `graphql:"removeLabelsFromLabelable(input: $input)"`
	}

	variables := map[string]interface{}{"input": params}
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	return gql.MutateNamed(context.Background(), "LabelRemove", &mutation, variables)
}

func updateIssue(httpClient *http.Client, repo ghrepo.Interface, params githubv4.UpdateIssueInput) error {
	var mutation struct {
		UpdateIssue struct {
			Typename string `graphql:"__typename"`
		} `graphql:"updateIssue(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	return gql.MutateNamed(context.Background(), "IssueUpdate", &mutation, variables)
}

func updatePullRequest(httpClient *http.Client, repo ghrepo.Interface, params githubv4.UpdatePullRequestInput) error {
	var mutation struct {
		UpdatePullRequest struct {
			Typename string `graphql:"__typename"`
		} `graphql:"updatePullRequest(input: $input)"`
	}
	variables := map[string]interface{}{"input": params}
	gql := graphql.NewClient(ghinstance.GraphQLEndpoint(repo.RepoHost()), httpClient)
	err := gql.MutateNamed(context.Background(), "PullRequestUpdate", &mutation, variables)
	return err
}

func ghIds(s *[]string) *[]githubv4.ID {
	if s == nil {
		return nil
	}
	ids := make([]githubv4.ID, len(*s))
	for i, v := range *s {
		ids[i] = v
	}
	return &ids
}

func ghId(s *string) *githubv4.ID {
	if s == nil {
		return nil
	}
	if *s == "" {
		r := githubv4.ID(nil)
		return &r
	}
	r := githubv4.ID(*s)
	return &r
}

func ghString(s *string) *githubv4.String {
	if s == nil {
		return nil
	}
	r := githubv4.String(*s)
	return &r
}
