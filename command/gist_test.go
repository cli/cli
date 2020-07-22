package command

import (
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/cli/cli/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestGistCreate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")

	http := initFakeHTTP()
	http.Register(httpmock.REST("POST", "gists"), httpmock.StringResponse(`
	{
		"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
	}
	`))

	output, err := RunCommand(`gist create "../test/fixtures/gistCreate.json" -d "Gist description" --public`)
	assert.NoError(t, err)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := make(map[string]interface{})
	err = json.Unmarshal(bodyBytes, &reqBody)
	if err != nil {
		t.Fatalf("error decoding JSON: %v", err)
	}

	expectParams := map[string]interface{}{
		"description": "Gist description",
		"files": map[string]interface{}{
			"gistCreate.json": map[string]interface{}{
				"content": "{}",
			},
		},
		"public": true,
	}

	assert.Equal(t, expectParams, reqBody)
	assert.Equal(t, "https://gist.github.com/aa5a315d61ae9438b18d\n", output.String())
}

func TestGistCreate_stdin(t *testing.T) {
	fakeStdin := strings.NewReader("hey cool how is it going")
	files, err := processFiles(ioutil.NopCloser(fakeStdin), []string{"-"})
	if err != nil {
		t.Fatalf("unexpected error processing files: %s", err)
	}

	assert.Equal(t, 1, len(files))
	assert.Equal(t, "hey cool how is it going", files["gistfile0.txt"])
}
