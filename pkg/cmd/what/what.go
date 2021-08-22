package what

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/cli/cli/pkg/resolver"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	HttpClient         func() (*http.Client, error)
	Config             func() (config.Config, error)
	IO                 *iostreams.IOStreams
	NameResolver       resolver.ResolverFunction
	VisibilityResolver resolver.ResolverFunction
	Resolver           *resolver.Resolver
	Description        string
}

func NewCmdWhat(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := &CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Config:     f.Config,
	}

	cmd := &cobra.Command{
		Use:  "what [<name>]",
		Args: cobra.MaximumNArgs(1),
		PreRunE: func(cmd *cobra.Command, args []string) error {
			opts.NameResolver = resolver.ResolverFunc(resolver.EnvStrat("REPO_NAME"), resolver.ArgsStrat(args, 0), promptNameStrat(opts.IO.CanPrompt()))

			r := resolver.New()

			r.AddStrat("visibility", resolver.MutuallyExclusiveBoolFlagsStrat(cmd, "public", "private", "internal"))
			r.AddStrat("visibility", promptVisibilityStrat(opts.IO.CanPrompt()))

			r.AddStrat("editor", resolver.StringFlagStrat(cmd, "editor"))
			r.AddStrat("editor", resolver.ConfigStrat(opts.Config, "", "editor"))
			r.AddStrat("editor", resolver.ValueStrat("cat"))

			opts.VisibilityResolver = r.ResolverFunc("visibility")
			opts.Resolver = r

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return createRun(opts)
		},
	}

	cmd.Flags().StringP("editor", "e", "", "some editor")
	cmd.Flags().Bool("public", false, "some visibility")
	cmd.Flags().Bool("private", false, "less visibility")
	cmd.Flags().Bool("internal", false, "no visibility")
	cmd.Flags().StringVarP(&opts.Description, "description", "d", "", "Description of repository")

	return cmd
}

func createRun(opts *CreateOptions) error {
	name, err := opts.NameResolver()
	if err != nil {
		return err
	}

	visibility, err := opts.VisibilityResolver()
	if err != nil {
		return err
	}

	editor, err := opts.Resolver.Resolve("editor")
	if err != nil {
		return err
	}

	fmt.Fprintf(opts.IO.Out, "%s - %s - %s", name, visibility, editor)

	return nil
}

func promptNameStrat(canPrompt bool) resolver.Strat {
	return func() (interface{}, error) {
		if !canPrompt {
			return nil, errors.New("name argument required when not running interactively")
		}

		qs := []*survey.Question{}
		getNameQuestion := &survey.Question{
			Name: "repoName",
			Prompt: &survey.Input{
				Message: "Repo name",
			},
		}
		qs = append(qs, getNameQuestion)

		answer := struct {
			RepoName string
		}{}

		err := prompt.SurveyAsk(qs, &answer)
		if err != nil {
			return nil, err
		}

		return answer.RepoName, nil
	}
}

func promptVisibilityStrat(canPrompt bool) resolver.Strat {
	return func() (interface{}, error) {
		if !canPrompt {
			return nil, errors.New("visibility argument required when not running interactively")
		}

		qs := []*survey.Question{}

		getVisibilityQuestion := &survey.Question{
			Name: "repoVisibility",
			Prompt: &survey.Select{
				Message: "Visibility",
				Options: []string{"Public", "Private", "Internal"},
			},
		}
		qs = append(qs, getVisibilityQuestion)

		answer := struct {
			RepoVisibility string
		}{}

		err := prompt.SurveyAsk(qs, &answer)
		if err != nil {
			return nil, err
		}

		return strings.ToUpper(answer.RepoVisibility), nil
	}
}
