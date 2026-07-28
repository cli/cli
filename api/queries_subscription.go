package api

import "github.com/shurcooL/githubv4"

// UpdateSubscription changes the viewer's subscription state for a subscribable node.
func UpdateSubscription(client *Client, host, id string, state githubv4.SubscriptionState) error {
	var mutation struct {
		UpdateSubscription struct {
			Subscribable struct {
				ID githubv4.ID
			}
		} `graphql:"updateSubscription(input: $input)"`
	}

	variables := map[string]interface{}{
		"input": githubv4.UpdateSubscriptionInput{
			SubscribableID: githubv4.ID(id),
			State:          state,
		},
	}

	return client.Mutate(host, "UpdateSubscription", &mutation, variables)
}
