package root

import (
	"github.com/cli/cli/v2/internal/build"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"testing"
)

func TestReferencHelp(t *testing.T) {
	buildVersion := build.Version
	buildDate := build.Date
	cmdFactory := factory.New(buildVersion)
	rootCmd, err := NewCmdRoot(cmdFactory, buildVersion, buildDate)
	if err != nil {
		t.Fatalf("NewCmdRoot error %s", err.Error())
	}
	for _, cmd := range rootCmd.Commands() {
		name := cmd.Name()
		if name != "reference" {
			continue
		}
		err = cmd.Help()
		if err != nil {
			t.Errorf("cmd %s help err %s", name, err.Error())
		}
	}
}
