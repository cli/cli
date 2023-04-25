package fielddelete

import (
	"testing"

	"github.com/cli/cli/v2/internal/tableprinter"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestRunDeleteField(t *testing.T) {
	defer gock.Off()
	gock.Observe(gock.DumpRequest)

	// delete Field
	gock.New("https://api.github.com").
		Post("/graphql").
		BodyString(`{"query":"mutation DeleteField.*","variables":{"input":{"fieldId":"an ID"}}}`).
		Reply(200).
		JSON(map[string]interface{}{
			"data": map[string]interface{}{
				"deleteProjectV2Field": map[string]interface{}{
					"projectV2Field": map[string]interface{}{
						"id": "Field ID",
					},
				},
			},
		})

	client, err := api.NewGraphQLClient(api.ClientOptions{AuthToken: "token"})
	assert.NoError(t, err)

	ios, _, stdout, _ := iostreams.Test()
	config := deleteFieldConfig{
		tp: tableprinter.New(ios),
		opts: deleteFieldOpts{
			fieldID: "an ID",
		},
		client: client,
	}

	err = runDeleteField(config)
	assert.NoError(t, err)
	assert.Equal(
		t,
		"Deleted field\n",
		stdout.String())
}
