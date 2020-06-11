package command

import (
	"bytes"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/cli/cli/utils"
)

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

func init() {
	RootCmd.AddCommand(creditsCmd)

	creditsCmd.Flags().BoolP("static", "s", false, "Print a static version of the credits")
}

var creditsCmd = &cobra.Command{
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
	Args:   cobra.ExactArgs(0),
	RunE:   ghCredits,
	Hidden: true,
}

func ghCredits(cmd *cobra.Command, _ []string) error {
	args := []string{"cli/cli"}
	return credits(cmd, args)
}

func credits(cmd *cobra.Command, args []string) error {
	isWindows := runtime.GOOS == "windows"
	ctx := contextForCommand(cmd)
	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	var owner string
	var repo string

	if len(args) == 0 {
		baseRepo, err := determineBaseRepo(client, cmd, ctx)
		if err != nil {
			return err
		}

		owner = baseRepo.RepoOwner()
		repo = baseRepo.RepoName()
	} else {
		parts := strings.SplitN(args[0], "/", 2)
		owner = parts[0]
		repo = parts[1]
	}

	type Contributor struct {
		Login string
	}

	type Result []Contributor

	result := Result{}
	body := bytes.NewBufferString("")
	path := fmt.Sprintf("repos/%s/%s/contributors", owner, repo)

	err = client.REST("GET", path, body, &result)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	isTTY := false
	outFile, isFile := out.(*os.File)
	if isFile {
		isTTY = utils.IsTerminal(outFile)
		if isTTY {
			// FIXME: duplicates colorableOut
			out = utils.NewColorable(outFile)
		}
	}

	static, err := cmd.Flags().GetBool("static")
	if err != nil {
		return err
	}

	static = static || isWindows

	if isTTY && static {
		fmt.Fprintln(out, "THANK YOU CONTRIBUTORS!!! <3")
		fmt.Println()
	}

	logins := []string{}
	for x, c := range result {
		if isTTY && !static {
			logins = append(logins, getColor(x)(c.Login))
		} else {
			fmt.Fprintf(out, "%s\n", c.Login)
		}
	}

	if !isTTY || static {
		return nil
	}

	rand.Seed(time.Now().UnixNano())

	lines := []string{}

	thankLines := strings.Split(thankYou, "\n")
	for x, tl := range thankLines {
		lines = append(lines, getColor(x)(tl))
	}
	lines = append(lines, "")
	lines = append(lines, logins...)
	lines = append(lines, "( <3 press ctrl-c to quit <3 )")

	termWidth, termHeight, err := terminal.GetSize(int(outFile.Fd()))
	if err != nil {
		return err
	}

	margin := termWidth / 3

	starLinesLeft := []string{}
	for x := 0; x < len(lines); x++ {
		starLinesLeft = append(starLinesLeft, starLine(margin))
	}

	starLinesRight := []string{}
	for x := 0; x < len(lines); x++ {
		lineWidth := termWidth - (margin + len(lines[x]))
		starLinesRight = append(starLinesRight, starLine(lineWidth))
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

func starLine(width int) string {
	line := ""
	starChance := 0.1
	for y := 0; y < width; y++ {
		chance := rand.Float64()
		if chance <= starChance {
			charRoll := rand.Float64()
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

func getColor(x int) func(string) string {
	rainbow := []func(string) string{
		utils.Magenta,
		utils.Red,
		utils.Yellow,
		utils.Green,
		utils.Cyan,
		utils.Blue,
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
