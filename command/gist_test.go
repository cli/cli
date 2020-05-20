package command

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"
)

func TestGistCreate(t *testing.T) {
	initBlankContext("", "OWNER/REPO", "trunk")

	http := initFakeHTTP()
	http.StubResponse(200, bytes.NewBufferString(`
	{
		"html_url": "https://gist.github.com/aa5a315d61ae9438b18d"
	}
	`))

	output, err := RunCommand(`gist create "../test/fixtures/gistCreate.json" -d "Gist description" --public`)
	eq(t, err, nil)

	bodyBytes, _ := ioutil.ReadAll(http.Requests[0].Body)
	reqBody := struct {
		Description string
		Public      bool
		Files       map[string]struct {
			Content string
		}
	}{}

	err = json.Unmarshal(bodyBytes, &reqBody)
	if err != nil {
		t.Fatalf("unexpected json error: %s", err)
	}

	eq(t, reqBody.Description, "Gist description")
	eq(t, reqBody.Public, true)
	eq(t, reqBody.Files["gistCreate.json"].Content, "{}")
	eq(t, output.String(), "https://gist.github.com/aa5a315d61ae9438b18d\n")
}

func TestGistCreate_stdin(t *testing.T) {
	fakeStdin := strings.NewReader("hey cool how is it going")
	files, err := processFiles(fakeStdin, []string{"-"})
	if err != nil {
		t.Fatalf("unexpected error processing files: %s", err)
	}

	eq(t, files["gistfile0.txt"], "hey cool how is it going")
}
