package view

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/githubrest"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ViewOptions struct {
	IO         *iostreams.IOStreams
	GitHubREST func(host string, opts ...githubrest.ClientOption) (*githubrest.Client, error)
	Config     func() (gh.Config, error)
	Template   string
}

func NewCmdView(f *cmdutil.Factory, runF func(*ViewOptions) error) *cobra.Command {
	opts := &ViewOptions{
		IO:         f.IOStreams,
		GitHubREST: f.GitHubREST,
		Config:     f.Config,
		Template:   "",
	}

	cmd := &cobra.Command{
		Use:   "view <template>",
		Short: "View an available repository gitignore template",
		Long: heredoc.Docf(`
			View an available repository %[1]s.gitignore%[1]s template.

			%[1]s<template>%[1]s is a case-sensitive %[1]s.gitignore%[1]s template name.

			For a list of available templates, run %[1]sgh repo gitignore list%[1]s.
		`, "`"),
		Example: heredoc.Doc(`
			# View the Go gitignore template
			$ gh repo gitignore view Go

			# View the Python gitignore template
			$ gh repo gitignore view Python

			# Create a new .gitignore file using the Go template
			$ gh repo gitignore view Go > .gitignore

			# Create a new .gitignore file using the Python template
			$ gh repo gitignore view Python > .gitignore
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Template = args[0]
			if runF != nil {
				return runF(opts)
			}
			return viewRun(cmd.Context(), opts)
		},
	}
	return cmd
}

func viewRun(ctx context.Context, opts *ViewOptions) error {
	if opts.Template == "" {
		return errors.New("no template provided")
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if err := opts.IO.StartPager(); err != nil {
		fmt.Fprintf(opts.IO.ErrOut, "starting pager failed: %v\n", err)
	}
	defer opts.IO.StopPager()

	hostname, _ := cfg.Authentication().DefaultHost()

	client, err := opts.GitHubREST(hostname)
	if err != nil {
		return err
	}

	gitIgnore, err := githubrest.RepoGitIgnoreTemplate(ctx, client, opts.Template)
	if err != nil {
		var httpErr *githubrest.ErrorResponse
		if errors.As(err, &httpErr) {
			if httpErr.StatusCode == 404 {
				return fmt.Errorf("'%s' is not a valid gitignore template. Run `gh repo gitignore list` for options", opts.Template)
			}
		}
		return err
	}

	return renderGitIgnore(gitIgnore, opts)
}

func renderGitIgnore(licenseTemplate *githubrest.GitIgnore, opts *ViewOptions) error {
	// I wanted to render this in a markdown code block and benefit
	// from .gitignore syntax highlighting. But, the upstream syntax highlighter
	// does not currently support .gitignore.
	// So, I just add a newline and print the content as is instead.
	// Ref: https://github.com/alecthomas/chroma/pull/755
	var out strings.Builder
	if opts.IO.IsStdoutTTY() {
		out.WriteString("\n")
	}
	out.WriteString(licenseTemplate.Source)
	_, err := opts.IO.Out.Write([]byte(out.String()))
	if err != nil {
		return err
	}
	return nil
}
