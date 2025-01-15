package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/docs"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/root"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/pflag"
)

func main() {
	if err := run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := pflag.NewFlagSet("", pflag.ContinueOnError)
	manPage := flags.BoolP("man-page", "", false, "Generate manual pages")
	website := flags.BoolP("website", "", false, "Generate website pages")
	dir := flags.StringP("doc-path", "", "", "Path directory where you want generate doc files")
	help := flags.BoolP("help", "h", false, "Help about any command")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Fprintf(os.Stderr, "Usage of %s:\n\n%s", filepath.Base(args[0]), flags.FlagUsages())
		return nil
	}

	if *dir == "" {
		return fmt.Errorf("error: --doc-path not set")
	}

	ios, _, _, _ := iostreams.Test()
	rootCmd, _ := root.NewCmdRoot(&cmdutil.Factory{
		IOStreams: ios,
		Browser:   &browser{},
		Config: func() (gh.Config, error) {
			return config.NewFromString(""), nil
		},
		ExtensionManager: &em{},
	}, "", "")
	rootCmd.InitDefaultHelpCmd()

	if err := os.MkdirAll(*dir, 0755); err != nil {
		return err
	}

	if *website {
		if err := docs.GenMarkdownTreeCustom(rootCmd, *dir, filePrepender, linkHandler); err != nil {
			return err
		}
	}

	if *manPage {
		if err := docs.GenManTree(rootCmd, *dir); err != nil {
			return err
		}
	}

	return nil
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

// Implements browser.Browser interface.
type browser struct{}

func (b *browser) Browse(_ string) error {
	return nil
}

// Implements extensions.ExtensionManager interface.
type em struct{}

func (e *em) List() []extensions.Extension {
	return nil
}

func (e *em) Install(_ ghrepo.Interface, _ string) error {
	return nil
}

func (e *em) InstallLocal(_ string) error {
	return nil
}

func (e *em) Upgrade(_ string, _ bool) error {
	return nil
}

func (e *em) Remove(_ string) error {
	return nil
}

func (e *em) Dispatch(_ []string, _ io.Reader, _, _ io.Writer) (bool, error) {
	return false, nil
}

func (e *em) Create(_ string, _ extensions.ExtTemplateType) error {
	return nil
}

func (e *em) EnableDryRunMode() {}

func (e *em) UpdateDir(_ string) string {
	return ""
}
