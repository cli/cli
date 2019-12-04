package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/github/gh-cli/command"
	"github.com/mitchellh/go-homedir"
)

func main() {
	migrateConfig()

	if cmd, err := command.RootCmd.ExecuteC(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		_, isFlagError := err.(command.FlagError)
		if isFlagError || strings.HasPrefix(err.Error(), "unknown command ") {
			fmt.Fprintln(os.Stderr, cmd.UsageString())
		}
		os.Exit(1)
	}
}

// This is a temporary function that will migrate the config file. It can be removed
// in January.
//
// If ~/.config/gh is a file, convert it to a directory and place the file
// into ~/.config/gh/config.yml
func migrateConfig() {
	p, _ := homedir.Expand("~/.config/gh")
	fi, err := os.Stat(p)
	if err != nil { // This means the file doesn't exist, and that is fine.
		return
	}
	if fi.Mode().IsDir() {
		return
	}

	content, err := ioutil.ReadFile(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to read config at %s", p)
		return
	}

	err = os.Remove(p)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to remove %s", p)
		return
	}

	err = os.MkdirAll(p, 0771)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed to mkdir %s", p)
		return
	}

	newPath := path.Join(p, "config.yml")
	err = ioutil.WriteFile(newPath, []byte(content), 0771)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration error: failed write to new config path %s", newPath)
		return
	}
}
