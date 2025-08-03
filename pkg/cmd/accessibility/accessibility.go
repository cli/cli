package accessibility

import (
	"fmt"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

const (
	webURL = "https://accessibility.github.com/conformance/cli/"
)

type AccessibilityOptions struct {
	IO      *iostreams.IOStreams
	Browser browser.Browser
	Web     bool
}

func NewCmdAccessibility(f *cmdutil.Factory) *cobra.Command {
	opts := AccessibilityOptions{
		IO:      f.IOStreams,
		Browser: f.Browser,
	}

	cmd := &cobra.Command{
		Use:     "accessibility",
		Aliases: []string{"a11y"},
		Short:   "Learn about GitHub CLI's accessibility experiences",
		Long:    longDescription(opts.IO),
		Hidden:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Web {
				if opts.IO.IsStdoutTTY() {
					fmt.Fprintf(opts.IO.ErrOut, "Opening %s in your browser.\n", text.DisplayURL(webURL))
				}
				return opts.Browser.Browse(webURL)
			}

			return cmd.Help()
		},
		Example: heredoc.Doc(`
			# Open the GitHub Accessibility site in your browser
			$ gh accessibility --web

			# Display color using customizable, 4-bit accessible colors
			$ gh config set accessible_colors enabled

			# Use input prompts without redrawing the screen
			$ gh config set accessible_prompter enabled

			# Disable motion-based spinners for progress indicators in favor of text
			$ gh config set spinner disabled
		`),
	}

	cmd.Flags().BoolVarP(&opts.Web, "web", "w", false, "Open the GitHub Accessibility site in your browser")
	cmdutil.DisableAuthCheck(cmd)

	return cmd
}

func longDescription(io *iostreams.IOStreams) string {
	cs := io.ColorScheme()
	title := cs.Bold("Learn about GitHub CLI's accessibility experiences")
	color := cs.Bold("Customizable and contrasting colors")
	prompter := cs.Bold("Non-interactive user input prompting")
	spinner := cs.Bold("Text-based spinners")
	feedback := cs.Bold("Join the conversation")

	return heredoc.Docf(`
		%[2]s

		As the home for all developers, we want every developer to feel welcome in our
		community and be empowered to contribute to the future of global software
		development with everything GitHub has to offer including the GitHub CLI.

		%[3]s

		Text interfaces often use color for various purposes, but insufficient contrast
		or customizability can leave some users unable to benefit.

		For a more accessible experience, the GitHub CLI can use color palettes
		based on terminal background appearance and limit colors to 4-bit ANSI color
		palettes, which users can customize within terminal preferences.

		With this new experience, the GitHub CLI provides multiple options to address
		color usage:

		1. The GitHub CLI will use 4-bit color palette for increased color contrast based
		   on dark and light backgrounds including rendering Markdown based on the
		   GitHub Primer design system.

		   To enable this experience, use one of the following methods:
		   - Run %[1]sgh config set accessible_colors enabled%[1]s
		   - Set %[1]sGH_ACCESSIBLE_COLORS=enabled%[1]s environment variable

		2. The GitHub CLI will display issue and pull request labels' custom RGB colors
		   in terminals with true color support.

		   To enable this experience, use one of the following methods:
		   - Run %[1]sgh config set color_labels enabled%[1]s
		   - Set %[1]sGH_COLOR_LABELS=enabled%[1]s environment variable

		%[4]s

		Interactive text user interfaces manipulate the terminal cursor to redraw parts
		of the screen, which can be difficult for speech synthesizers or braille displays
		to accurately detect and read.

		For a more accessible experience, the GitHub CLI can provide a similar experience using
		non-interactive prompts for user input.

		To enable this experience, use one of the following methods:
		- Run %[1]sgh config set accessible_prompter enabled%[1]s
		- Set %[1]sGH_ACCESSIBLE_PROMPTER=enabled%[1]s environment variable

		%[5]s

		Motion-based spinners communicate in-progress activity by manipulating the
		terminal cursor to create a spinning effect, which may cause discomfort to users
		with motion sensitivity or miscommunicate information to speech synthesizers.

		For a more accessible experience, this interactivity can be disabled in favor
		of text-based progress indicators.

		To enable this experience, use one of the following methods:
		- Run %[1]sgh config set spinner disabled%[1]s
		- Set %[1]sGH_SPINNER_DISABLED=yes%[1]s environment variable

		%[6]s

		We invite you to join us in improving GitHub CLI accessibility by sharing your
		feedback and ideas through GitHub Accessibility feedback channels:

		%[7]s
	`, "`", title, color, prompter, spinner, feedback, webURL)
}
