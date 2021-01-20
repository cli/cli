package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cli/cli/internal/docs"
	"github.com/cli/cli/pkg/cmd/root"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/pflag"
)

func main() {
	var flagError pflag.ErrorHandling
	docCmd := pflag.NewFlagSet("", flagError)
	manPage := docCmd.BoolP("man-page", "", false, "Generate manual pages")
	website := docCmd.BoolP("website", "", false, "Generate website pages")
	dir := docCmd.StringP("doc-path", "", "", "Path directory where you want generate doc files")
	help := docCmd.BoolP("help", "h", false, "Help about any command")

	if err := docCmd.Parse(os.Args); err != nil {
		os.Exit(1)
	}

	if *help {
		_, err := fmt.Fprintf(os.Stderr, "Usage of %s:\n\n%s", os.Args[0], docCmd.FlagUsages())
		if err != nil {
			fatal(err)
		}
		os.Exit(1)
	}

	if *dir == "" {
		fatal("no dir set")
	}

	io, _, _, _ := iostreams.Test()
	rootCmd := root.NewCmdRoot(&cmdutil.Factory{IOStreams: io}, "", "")
	rootCmd.InitDefaultHelpCmd()

	err := os.MkdirAll(*dir, 0755)
	if err != nil {
		fatal(err)
	}

	if *website {
		err = docs.GenMarkdownTreeCustom(rootCmd, *dir, filePrepender, linkHandler)
		if err != nil {
			fatal(err)
		}
	}

	if *manPage {
		header := &docs.GenManHeader{
			Title:   "gh",
			Section: "1",
			Source:  "",
			Manual:  "",
		}
		err = docs.GenManTree(rootCmd, header, *dir)
		if err != nil {
			fatal(err)
		}
	}
}

func filePrepender(filename string) string {
	return `---
layout: manual
permalink: /:path/:basename
---

`
}

func linkHandler(name string) string {
	return fmt.Sprintf("./%s", strings.TrimSuffix(name, ".md"))
}

func fatal(msg interface{}) {
	fmt.Fprintln(os.Stderr, msg)
	os.Exit(1)
}
