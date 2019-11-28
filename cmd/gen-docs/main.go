package main

import (
	"fmt"
	"os"

	"github.com/github/gh-cli/command"
	"github.com/spf13/cobra/doc"
)

func main() {
	if len(os.Args) < 2 {
		fatal("Usage: gen-docs <destination-dir>")
	}
	dir := os.Args[1]

	err := os.MkdirAll(dir, 0755)
	if err != nil {
		fatal(err)
	}

	err = doc.GenMarkdownTreeCustom(command.RootCmd, dir, filePrepender, linkHandler)
	if err != nil {
		fatal(err)
	}
}

func filePrepender(filename string) string {
	return `---
layout: page
---

`
}

func linkHandler(name string) string {
	return fmt.Sprintf("{{site.baseurl}}{%% link %s %%}", name)
}

func fatal(msg interface{}) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
