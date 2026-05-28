package edit

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/internal/prompter"
	"github.com/cli/cli/v2/pkg/cmd/discussion/client"
	"github.com/cli/cli/v2/pkg/cmd/discussion/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

// EditOptions holds the configuration for the discussion edit command.
type EditOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Client     func() (client.DiscussionClient, error)
	Prompter   prompter.Prompter

	Interactive      bool
	TitleProvided    bool
	BodyProvided     bool
	CategoryProvided bool
	LabelsProvided   bool

	DiscussionNumber int
	Title            string
	Body             string
	BodyFile         string
	Category         string
	AddLabels        []string
	RemoveLabels     []string
}

// NewCmdEdit returns a cobra command for editing a GitHub Discussion.
func NewCmdEdit(f *cmdutil.Factory, runF func(*EditOptions) error) *cobra.Command {
	opts := &EditOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		Prompter:   f.Prompter,
		Client:     shared.DiscussionClientFunc(f),
	}

	cmd := &cobra.Command{
		Use:   "edit {<number> | <url>}",
		Short: "Edit a discussion (preview)",
		Long: heredoc.Doc(`
			Edit a GitHub Discussion.

			Without flags, the command runs interactively when connected to a terminal.
			Use flags to update specific fields non-interactively.
		`),
		Example: heredoc.Doc(`
			# Edit interactively
			$ gh discussion edit 123

			# Update title, body, and category
			$ gh discussion edit 123 --title "Updated title" --body "Updated body" --category "Ideas"

			# Update body from a file
			$ gh discussion edit 123 --body-file body.md

			# Add and remove labels
			$ gh discussion edit 123 --add-label "bug,help wanted" --remove-label "stale"
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			number, repo, err := shared.ParseDiscussionArg(args[0])
			if err != nil {
				return cmdutil.FlagErrorWrap(err)
			}

			if repo != nil {
				opts.BaseRepo = func() (ghrepo.Interface, error) {
					return repo, nil
				}
			} else {
				opts.BaseRepo = f.BaseRepo
			}

			opts.DiscussionNumber = number

			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				cmd.Flags().Changed("body"), cmd.Flags().Changed("body-file")); err != nil {
				return err
			}

			opts.TitleProvided = cmd.Flags().Changed("title")
			opts.BodyProvided = cmd.Flags().Changed("body") || cmd.Flags().Changed("body-file")
			opts.CategoryProvided = cmd.Flags().Changed("category")
			opts.LabelsProvided = len(opts.AddLabels) > 0 || len(opts.RemoveLabels) > 0

			noFlagsSet := !opts.TitleProvided && !opts.BodyProvided && !opts.CategoryProvided && !opts.LabelsProvided
			if noFlagsSet && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("specify at least one flag to update the discussion non-interactively")
			}

			opts.Interactive = noFlagsSet

			if runF != nil {
				return runF(opts)
			}
			return editRun(opts)
		},
	}

	cmdutil.EnableRepoOverride(cmd, f)

	cmd.Flags().StringVarP(&opts.Title, "title", "t", "", "New title for the discussion")
	cmd.Flags().StringVarP(&opts.Body, "body", "b", "", "New body for the discussion")
	cmd.Flags().StringVarP(&opts.BodyFile, "body-file", "F", "", "Read body text from file (use \"-\" to read from standard input)")
	cmd.Flags().StringVarP(&opts.Category, "category", "c", "", "New category name or slug for the discussion")
	cmd.Flags().StringSliceVar(&opts.AddLabels, "add-label", nil, "Add labels by `name`")
	cmd.Flags().StringSliceVar(&opts.RemoveLabels, "remove-label", nil, "Remove labels by `name`")

	return cmd
}

func editRun(opts *EditOptions) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	c, err := opts.Client()
	if err != nil {
		return err
	}

	opts.IO.StartProgressIndicator()
	discussion, err := c.GetByNumber(repo, opts.DiscussionNumber)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	input := client.UpdateDiscussionInput{
		DiscussionID: discussion.ID,
	}

	if opts.Interactive {
		changed, err := promptEdit(opts, discussion, c, repo, &input)
		if err != nil {
			return err
		}

		if !changed {
			return fmt.Errorf("no changes made")
		}
	} else {
		if opts.TitleProvided {
			if strings.TrimSpace(opts.Title) == "" {
				return cmdutil.FlagErrorf("title cannot be blank")
			}
			input.Title = &opts.Title
		}
		if opts.BodyProvided {
			if opts.BodyFile != "" {
				bodyBytes, err := cmdutil.ReadFile(opts.BodyFile, opts.IO.In)
				if err != nil {
					return err
				}
				opts.Body = string(bodyBytes)
			}
			input.Body = &opts.Body
		}
		if opts.CategoryProvided {
			opts.IO.StartProgressIndicator()
			categories, err := c.ListCategories(repo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return fmt.Errorf("fetching categories: %w", err)
			}
			cat, err := shared.MatchCategory(opts.Category, categories)
			if err != nil {
				return err
			}
			input.CategoryID = &cat.ID
		}

		if opts.LabelsProvided {
			opts.IO.StartProgressIndicator()
			allLabels, err := c.ListLabels(repo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return fmt.Errorf("fetching labels: %w", err)
			}
			if len(opts.AddLabels) > 0 {
				input.AddLabelIDs, err = shared.ResolveLabels(allLabels, opts.AddLabels)
				if err != nil {
					return err
				}
			}
			if len(opts.RemoveLabels) > 0 {
				input.RemoveLabelIDs, err = shared.ResolveLabels(allLabels, opts.RemoveLabels)
				if err != nil {
					return err
				}
			}
		}
	}

	opts.IO.StartProgressIndicator()
	updated, err := c.Update(repo, input)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return err
	}

	fmt.Fprintln(opts.IO.Out, updated.URL)
	return nil
}

// promptEdit runs the interactive flow, populating input with user choices. It returns a boolean indicating whether any
// changes were made, and an error if the process failed.
func promptEdit(opts *EditOptions, discussion *client.Discussion, c client.DiscussionClient, repo ghrepo.Interface, input *client.UpdateDiscussionInput) (bool, error) {
	choices := []string{"Title", "Body", "Category"}
	selected, err := opts.Prompter.MultiSelect("What would you like to edit?", nil, choices)
	if err != nil {
		return false, err
	}
	if len(selected) == 0 {
		return false, nil
	}

	for _, idx := range selected {
		switch choices[idx] {
		case "Title":
			title, err := opts.Prompter.Input("Title", discussion.Title)
			if err != nil {
				return false, err
			}
			if strings.TrimSpace(title) == "" {
				return false, fmt.Errorf("title cannot be blank")
			}
			input.Title = &title

		case "Body":
			body, err := opts.Prompter.MarkdownEditor("Body", discussion.Body, false)
			if err != nil {
				return false, err
			}
			input.Body = &body

		case "Category":
			opts.IO.StartProgressIndicator()
			categories, err := c.ListCategories(repo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return false, err
			}
			names := make([]string, len(categories))
			for i, cat := range categories {
				names[i] = cat.Name
			}
			currentName := discussion.Category.Name
			idx, err := opts.Prompter.Select("Category", currentName, names)
			if err != nil {
				return false, err
			}
			input.CategoryID = &categories[idx].ID
		}
	}

	return true, nil
}
