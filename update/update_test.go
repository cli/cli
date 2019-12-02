package update

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/github/gh-cli/api"
	"github.com/github/gh-cli/command"
)

func TestRunWhileCheckingForUpdate(t *testing.T) {
	originalVersion := command.Version
	command.Version = "v0.0.0"
	defer func() {
		command.Version = originalVersion
	}()

	http := &api.FakeHTTP{}
	jsonFile, _ := os.Open("../test/fixtures/latestRelease.json")
	defer jsonFile.Close()
	http.StubResponse(200, jsonFile)

	client := api.NewClient(api.ReplaceTripper(http))
	alertMsg := *UpdateMessage(client)
	fmt.Printf("🌭 %+v\n", alertMsg)

	if !strings.Contains(alertMsg, command.Version) {
		t.Errorf("expected: \"%v\" to contain \"%v\"", alertMsg, command.Version)
	}

	if !strings.Contains(alertMsg, "v1.0.0") {
		t.Errorf("expected: \"%v\" to contain \"%v\"", alertMsg, "v1.0.0")
	}
}
