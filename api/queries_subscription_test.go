package api

import (
	"testing"

	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateSubscription(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`mutation UpdateSubscription\b`),
		httpmock.GraphQLMutation(`{
			"data": {
				"updateSubscription": {
					"subscribable": {"id": "ISSUE-ID"}
				}
			}
		}`, func(inputs map[string]interface{}) {
			assert.Equal(t, "ISSUE-ID", inputs["subscribableId"])
			assert.Equal(t, "SUBSCRIBED", inputs["state"])
		}),
	)

	err := UpdateSubscription(newTestClient(reg), "github.com", "ISSUE-ID", githubv4.SubscriptionStateSubscribed)
	require.NoError(t, err)
}

func TestUpdateSubscription_error(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.GraphQL(`mutation UpdateSubscription\b`),
		httpmock.GraphQLMutation(`{
			"errors": [{"type": "FORBIDDEN", "message": "subscription denied"}]
		}`, func(inputs map[string]interface{}) {}),
	)

	err := UpdateSubscription(newTestClient(reg), "github.com", "ISSUE-ID", githubv4.SubscriptionStateSubscribed)
	require.EqualError(t, err, "GraphQL: subscription denied")
}
