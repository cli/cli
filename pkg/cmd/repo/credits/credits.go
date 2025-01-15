package credits

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type CreditsOptions struct {
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	IO         *iostreams.IOStreams

	Repository string
	Static     bool
}

func NewCmdCredits(f *cmdutil.Factory, runF func(*CreditsOptions) error) *cobra.Command {
	opts := &CreditsOptions{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
		BaseRepo:   f.BaseRepo,
		Repository: "cli/cli",
	}

	cmd := &cobra.Command{
		Use:   "credits",
		Short: "View credits for this tool",
		Long:  `View animated credits for gh, the tool you are currently using :)`,
		Example: heredoc.Doc(`
			# see a credits animation for this project
			$ gh credits

			# display a non-animated thank you
			$ gh credits -s

			# just print the contributors, one per line
			$ gh credits | cat
		`),
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}

			return creditsRun(opts)
		},
		Hidden: true,
	}

	cmd.Flags().BoolVarP(&opts.Static, "static", "s", false, "Print a static version of the credits")

	return cmd
}

func NewCmdRepoCredits(f *cmdutil.Factory, runF func(*CreditsOptions) error) *cobra.Command {
	opts := &CreditsOptions{
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "credits [<repository>]",
		Short: "View credits for a repository",
		Example: heredoc.Doc(`
			# view credits for the current repository
			$ gh repo credits

			# view credits for a specific repository
			$ gh repo credits cool/repo

			# print a non-animated thank you
			$ gh repo credits -s

			# pipe to just print the contributors, one per line
			$ gh repo credits | cat
		`),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.Repository = args[0]
			}

			if runF != nil {
				return runF(opts)
			}

			return creditsRun(opts)
		},
		Hidden: true,
	}

	cmd.Flags().BoolVarP(&opts.Static, "static", "s", false, "Print a static version of the credits")

	return cmd
}

func creditsRun(opts *CreditsOptions) error {
	isWindows := runtime.GOOS == "windows"
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	client := api.NewClientFromHTTP(httpClient)

	var baseRepo ghrepo.Interface
	if opts.Repository == "" {
		baseRepo, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		baseRepo, err = ghrepo.FromFullName(opts.Repository)
		if err != nil {
			return err
		}
	}

	type Contributor struct {
		Login string
		Type  string
	}

	type Result []Contributor

	result := Result{}
	body := bytes.NewBufferString("")
	path := fmt.Sprintf("repos/%s/%s/contributors", baseRepo.RepoOwner(), baseRepo.RepoName())

	err = client.REST(baseRepo.RepoHost(), "GET", path, body, &result)
	if err != nil {
		return err
	}

	isTTY := opts.IO.IsStdoutTTY()

	static := opts.Static || isWindows

	out := opts.IO.Out
	cs := opts.IO.ColorScheme()

	if isTTY && static {
		fmt.Fprintln(out, "THANK YOU CONTRIBUTORS!!! <3")
		fmt.Fprintln(out, "")
	}

	logins := []string{}
	for x, c := range result {
		if c.Type != "User" {
			continue
		}

		if isTTY && !static {
			logins = append(logins, cs.ColorFromString(getColor(x))(c.Login))
		} else {
			fmt.Fprintf(out, "%s\n", c.Login)
		}
	}

	if !isTTY || static {
		return nil
	}

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	lines := []string{}

	thankLines := strings.Split(thankYou, "\n")
	for x, tl := range thankLines {
		lines = append(lines, cs.ColorFromString(getColor(x))(tl))
	}
	lines = append(lines, "")
	lines = append(lines, logins...)
	lines = append(lines, "( <3 press ctrl-c to quit <3 )")

	termWidth, termHeight, err := utils.TerminalSize(out)
	if err != nil {
		return err
	}

	margin := termWidth / 3

	starLinesLeft := []string{}
	for x := 0; x < len(lines); x++ {
		starLinesLeft = append(starLinesLeft, starLine(r, margin))
	}

	starLinesRight := []string{}
	for x := 0; x < len(lines); x++ {
		lineWidth := termWidth - (margin + len(lines[x]))
		starLinesRight = append(starLinesRight, starLine(r, lineWidth))
	}

	loop := true
	startx := termHeight - 1
	li := 0

	for loop {
		clear()
		for x := 0; x < termHeight; x++ {
			if x == startx || startx < 0 {
				starty := 0
				if startx < 0 {
					starty = int(math.Abs(float64(startx)))
				}
				for y := starty; y < li+1; y++ {
					if y >= len(lines) {
						continue
					}
					starLineLeft := starLinesLeft[y]
					starLinesLeft[y] = twinkle(starLineLeft)
					starLineRight := starLinesRight[y]
					starLinesRight[y] = twinkle(starLineRight)
					fmt.Fprintf(out, "%s %s %s\n", starLineLeft, lines[y], starLineRight)
				}
				li += 1
				x += li
			} else {
				fmt.Fprintf(out, "\n")
			}
		}
		if li < len(lines) {
			startx -= 1
		}
		time.Sleep(300 * time.Millisecond)
	}

	return nil
}

func starLine(r *rand.Rand, width int) string {
	line := ""
	starChance := 0.1
	for y := 0; y < width; y++ {
		chance := r.Float64()
		if chance <= starChance {
			charRoll := r.Float64()
			switch {
			case charRoll < 0.3:
				line += "."
			case charRoll > 0.3 && charRoll < 0.6:
				line += "+"
			default:
				line += "*"
			}
		} else {
			line += " "
		}
	}

	return line
}

func twinkle(starLine string) string {
	starLine = strings.ReplaceAll(starLine, ".", "P")
	starLine = strings.ReplaceAll(starLine, "+", "A")
	starLine = strings.ReplaceAll(starLine, "*", ".")
	starLine = strings.ReplaceAll(starLine, "P", "+")
	starLine = strings.ReplaceAll(starLine, "A", "*")
	return starLine
}

func getColor(x int) string {
	rainbow := []string{
		"magenta",
		"red",
		"yellow",
		"green",
		"cyan",
		"blue",
	}

	ix := x % len(rainbow)

	return rainbow[ix]
}

func clear() {
	// on windows we'd do cmd := exec.Command("cmd", "/c", "cls"); unfortunately the draw speed is so
	// slow that the animation is very jerky, flashy, and painful to look at.
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	_ = cmd.Run()
}

var thankYou = `
     _                    _
    | |                  | |
_|_ | |     __,   _  _   | |           __
 |  |/ \   /  |  / |/ |  |/_)   |   | /  \_|   |
 |_/|   |_/\_/|_/  |  |_/| \_/   \_/|/\__/  \_/|_/
                                   /|
                                   \|
                              _
                           o | |                           |
 __   __   _  _  _|_  ,_     | |        _|_  __   ,_    ,  |
/    /  \_/ |/ |  |  /  |  | |/ \_|   |  |  /  \_/  |  / \_|
\___/\__/   |  |_/|_/   |_/|_/\_/  \_/|_/|_/\__/    |_/ \/ o


`
