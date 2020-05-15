package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/cli/cli/context"
)

func TestGistCreate(t *testing.T) {
	ctx := context.NewBlank()
	initContext = func() context.Context {
		return ctx
	}

	http := initFakeHTTP()

	http.StubResponse(200, bytes.NewBufferString(`
	{
		"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
	}
	`))

	output, err := RunCommand(`gist create -f "../test/fixtures/gistCreate.json" -d "Gist description" -p=false`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Description string
		Public      bool
		Files       map[string]struct {
			Content string
		}
	}{}

	json.Unmarshal(bodyBytes, &reqBody)

	eq(t, reqBody.Description, "Gist description")
	eq(t, reqBody.Public, false)
	eq(t, reqBody.Files["gistCreate.json"].Content, "{}")
	eq(t, output.String(), "https://gist.github.com/aa5a315d61ae9438b18d\n")
}
