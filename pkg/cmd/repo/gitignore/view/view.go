package view

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/pkg/cmd/repo/shared/queries"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	HTTPClient func() (*http.Client, error)
	Config     func() (gh.Config, error)
	Template   string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		HTTPClient: f.HttpClient,
		Config:     f.Config,
		Template:   "",
	}

	cmd := &cobra.Command{
		Use:   "view <template>",
		Short: "View an available repository gitignore template",
		Long: heredoc.Docf(`
		View an available repository %[1]s.gitignore%[1]s template.
		
		%[1]s<template>%[1]s is a case-sensitive %[1]s.gitignore%[1]s template name.
		`, "`"),
		Args: cmdutil.ExactArgs(1, "gh repo gitignore view only takes a single template argument"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Template = args[0]
			if runF != nil {
				return runF(opts)
			}
			return viewRun(opts)
		},
	}
	return cmd
}

func viewRun(opts *ViewOptions) error {
	if opts.Template == "" {
		return errors.New("no template provided")
	}

	client, err := opts.HTTPClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err == nil {
		defer opts.IO.StopPager()
	} else {
		fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
	}

	hostname, _ := cfg.Authentication().DefaultHost()
	gitIgnore, err := queries.GitIgnoreTemplate(client, hostname, opts.Template)
	if err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return fmt.Errorf("'%s' is not a valid gitignore template. Run `gh repo gitignore list` for options", opts.Template)
		}
		return err
	}

	return renderGitIgnore(gitIgnore, opts)
}

func renderGitIgnore(licenseTemplate api.GitIgnore, opts *ViewOptions) error {
	// I wanted to render this in a markdown code block and benefit
	// from .gitignore syntax highlighting. But, the upstream syntax highlighter
	// does not currently support .gitignore.
	// So, I just add a newline and print the content as is instead.
	// Ref: https://github.com/alecthomas/chroma/pull/755
	var out string
	if opts.IO.IsStdoutTTY() {
		out = "\n"
	}
	out += licenseTemplate.Source
	_, err := opts.IO.Out.Write([]byte(out))
	if err != nil {
		return err
	}
	return nil
}
