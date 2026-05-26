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

	DiscussionNumber int
	Title            string
	Body             string
	BodyFile         string
	Category         string
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
		Long: heredoc.Docf(`
			Edit a GitHub Discussion.

			With %[1]s--title%[1]s, %[1]s--body%[1]s, and %[1]s--category%[1]s flags, the discussion is updated
			non-interactively. Omitting all flags triggers interactive prompts when connected to a terminal.
		`, "`"),
		Example: heredoc.Doc(`
			# Edit interactively
			$ gh discussion edit 123

			# Set a new title
			$ gh discussion edit 123 --title "Updated title"

			# Change the category
			$ gh discussion edit 123 --category "Ideas"

			# Update body from a file
			$ gh discussion edit 123 --body-file body.md
		`),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := cmdutil.MutuallyExclusive("specify only one of --body or --body-file",
				opts.Body != "", opts.BodyFile != ""); err != nil {
				return err
			}

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

			noFlagsSet := opts.Title == "" && opts.Body == "" && opts.BodyFile == "" && opts.Category == ""
			if noFlagsSet && !opts.IO.CanPrompt() {
				return cmdutil.FlagErrorf("specify at least one of --title, --body, --body-file, or --category when not running interactively")
			}

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
		return fmt.Errorf("fetching discussion: %w", err)
	}

	// Resolve body from file if provided.
	if opts.BodyFile != "" {
		bodyBytes, err := cmdutil.ReadFile(opts.BodyFile, opts.IO.In)
		if err != nil {
			return err
		}
		opts.Body = string(bodyBytes)
	}

	input := client.UpdateDiscussionInput{
		DiscussionID: discussion.ID,
	}

	// noFlagsSet omits BodyFile intentionally: ReadFile above already copied its
	// contents into opts.Body, so Body == "" implies no body update was requested.
	noFlagsSet := opts.Title == "" && opts.Body == "" && opts.Category == ""
	if noFlagsSet {
		// Interactive mode: prompt user to select which fields to edit.
		if err := promptEdit(opts, discussion, c, repo, &input); err != nil {
			return err
		}
		// If the user dismissed the prompt without selecting anything, skip the
		// API call — there is nothing to update.
		if input.Title == nil && input.Body == nil && input.CategoryID == nil {
			return nil
		}
	} else {
		// Non-interactive: apply only the flags that were set.
		if opts.Title != "" {
			if strings.TrimSpace(opts.Title) == "" {
				return cmdutil.FlagErrorf("title cannot be blank")
			}
			input.Title = &opts.Title
		}
		if opts.Body != "" {
			input.Body = &opts.Body
		}
		if opts.Category != "" {
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
	}

	opts.IO.StartProgressIndicator()
	updated, err := c.Update(repo, input)
	opts.IO.StopProgressIndicator()
	if err != nil {
		return fmt.Errorf("failed to update discussion: %w", err)
	}

	fmt.Fprintln(opts.IO.Out, updated.URL)
	return nil
}

// promptEdit runs the interactive flow, populating input with user choices.
func promptEdit(opts *EditOptions, discussion *client.Discussion, c client.DiscussionClient, repo ghrepo.Interface, input *client.UpdateDiscussionInput) error {
	choices := []string{"title", "body", "category"}
	selected, err := opts.Prompter.MultiSelect("What would you like to edit?", nil, choices)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}

	for _, idx := range selected {
		switch choices[idx] {
		case "title":
			title, err := opts.Prompter.Input("Discussion title", discussion.Title)
			if err != nil {
				return err
			}
			if strings.TrimSpace(title) == "" {
				return fmt.Errorf("title cannot be blank")
			}
			input.Title = &title

		case "body":
			body, err := opts.Prompter.MarkdownEditor("Discussion body", discussion.Body, false)
			if err != nil {
				return err
			}
			input.Body = &body

		case "category":
			opts.IO.StartProgressIndicator()
			categories, err := c.ListCategories(repo)
			opts.IO.StopProgressIndicator()
			if err != nil {
				return fmt.Errorf("fetching categories: %w", err)
			}
			names := make([]string, len(categories))
			for i, cat := range categories {
				names[i] = cat.Name
			}
			currentName := discussion.Category.Name
			idx, err := opts.Prompter.Select("Discussion category", currentName, names)
			if err != nil {
				return err
			}
			input.CategoryID = &categories[idx].ID
		}
	}

	return nil
}
