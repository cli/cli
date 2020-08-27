package garden

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type Geometry struct {
	Width      int
	Height     int
	Density    float64
	Repository ghrepo.Interface
}

type Player struct {
	X                   int
	Y                   int
	Char                string
	Geo                 *Geometry
	ShoeMoistureContent int
}

type Commit struct {
	Email  string
	Handle string
	Sha    string
	Char   string
}

type Cell struct {
	Char       string
	StatusLine string
}

const (
	DirUp = iota
	DirDown
	DirLeft
	DirRight
)

type Direction = int

func (p *Player) move(direction Direction) {
	switch direction {
	case DirUp:
		if p.Y == 0 {
			return
		}
		p.Y--
	case DirDown:
		if p.Y == p.Geo.Height-1 {
			return
		}
		p.Y++
	case DirLeft:
		if p.X == 0 {
			return
		}
		p.X--
	case DirRight:
		if p.X == p.Geo.Width-1 {
			return
		}
		p.X++
	}
}

type GardenOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	RepoArg string
}

func NewCmdGarden(f *cmdutil.Factory, runF func(*GardenOptions) error) *cobra.Command {
	opts := GardenOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:    "garden [<repository>]",
		Short:  "Explore a git repository as a garden",
		Long:   "Use WASD or vi keys to move. q to quit.",
		Hidden: true,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			if runF != nil {
				return runF(&opts)
			}
			return gardenRun(&opts)
		},
	}

	return cmd
}

func gardenRun(opts *GardenOptions) error {
	out := opts.IO.Out

	if runtime.GOOS == "windows" {
		return errors.New("sorry :( this command only works on linux and macos")
	}

	if !opts.IO.IsStdoutTTY() {
		return errors.New("must be connected to a terminal")
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	var toView ghrepo.Interface
	apiClient := api.NewClientFromHTTP(httpClient)
	if opts.RepoArg == "" {
		var err error
		toView, err = opts.BaseRepo()
		if err != nil {
			return err
		}
	} else {
		var err error
		viewURL := opts.RepoArg
		if !strings.Contains(viewURL, "/") {
			currentUser, err := api.CurrentLoginName(apiClient, ghinstance.Default())
			if err != nil {
				return err
			}
			viewURL = currentUser + "/" + viewURL
		}
		toView, err = ghrepo.FromFullName(viewURL)
		if err != nil {
			return fmt.Errorf("argument error: %w", err)
		}
	}

	seed := computeSeed(ghrepo.FullName(toView))
	rand.Seed(seed)

	termWidth, termHeight, err := utils.TerminalSize(out)
	if err != nil {
		return err
	}

	termWidth -= 10
	termHeight -= 10

	geo := &Geometry{
		Width:      termWidth,
		Height:     termHeight,
		Repository: toView,
		// TODO based on number of commits/cells instead of just hardcoding
		Density: 0.3,
	}

	maxCommits := geo.Width * geo.Height

	commits, err := getCommits(httpClient, toView, maxCommits)
	if err != nil {
		return err
	}
	player := &Player{0, 0, utils.Bold("@"), geo, 0}

	garden := plantGarden(commits, geo)
	clear(opts.IO)
	drawGarden(out, garden, player)

	// thanks stackoverflow https://stackoverflow.com/a/17278776
	if runtime.GOOS == "darwin" {
		_ = exec.Command("stty", "-f", "/dev/tty", "cbreak", "min", "1").Run()
		_ = exec.Command("stty", "-f", "/dev/tty", "-echo").Run()
	} else {
		_ = exec.Command("stty", "-F", "/dev/tty", "cbreak", "min", "1").Run()
		_ = exec.Command("stty", "-F", "/dev/tty", "-echo").Run()
	}

	var b []byte = make([]byte, 1)
	for {
		_, err := opts.IO.In.Read(b)
		if err != nil {
			return err
		}

		quitting := false
		switch {
		case isLeft(b):
			player.move(DirLeft)
		case isRight(b):
			player.move(DirRight)
		case isUp(b):
			player.move(DirUp)
		case isDown(b):
			player.move(DirDown)
		case isQuit(b):
			quitting = true
		}

		if quitting {
			break
		}

		clear(opts.IO)
		drawGarden(out, garden, player)
	}

	fmt.Println()
	fmt.Println(utils.Bold("You turn and walk away from the wildflower garden..."))

	return nil
}

// TODO fix arrow keys

func isLeft(b []byte) bool {
	return bytes.EqualFold(b, []byte("a")) || bytes.EqualFold(b, []byte("h"))
}

func isRight(b []byte) bool {
	return bytes.EqualFold(b, []byte("d")) || bytes.EqualFold(b, []byte("l"))
}

func isDown(b []byte) bool {
	return bytes.EqualFold(b, []byte("s")) || bytes.EqualFold(b, []byte("j"))
}

func isUp(b []byte) bool {
	return bytes.EqualFold(b, []byte("w")) || bytes.EqualFold(b, []byte("k"))
}

func isQuit(b []byte) bool {
	return bytes.EqualFold(b, []byte("q"))
}

func plantGarden(commits []*Commit, geo *Geometry) [][]*Cell {
	cellIx := 0
	grassCell := &Cell{RGB(0, 200, 0, ","), "You're standing on a patch of grass in a field of wildflowers."}
	garden := [][]*Cell{}
	streamIx := rand.Intn(geo.Width - 1)
	if streamIx == geo.Width/2 {
		streamIx--
	}
	tint := 0
	for y := 0; y < geo.Height; y++ {
		if cellIx == len(commits)-1 {
			break
		}
		garden = append(garden, []*Cell{})
		for x := 0; x < geo.Width; x++ {
			if (y > 0 && (x == 0 || x == geo.Width-1)) || y == geo.Height-1 {
				garden[y] = append(garden[y], &Cell{
					Char:       RGB(0, 150, 0, "^"),
					StatusLine: "You're standing under a tall, leafy tree.",
				})
				continue
			}
			if x == streamIx {
				garden[y] = append(garden[y], &Cell{
					Char:       RGB(tint, tint, 255, "#"),
					StatusLine: "You're standing in a shallow stream. It's refreshing.",
				})
				tint += 15
				streamIx--
				if rand.Float64() < 0.5 {
					streamIx++
				}
				if streamIx < 0 {
					streamIx = 0
				}
				if streamIx > geo.Width {
					streamIx = geo.Width
				}
				continue
			}
			if y == 0 && (x < geo.Width/2 || x > geo.Width/2) {
				garden[y] = append(garden[y], &Cell{
					Char:       RGB(0, 200, 0, ","),
					StatusLine: "You're standing by a wildflower garden. There is a light breeze.",
				})
				continue
			} else if y == 0 && x == geo.Width/2 {
				garden[y] = append(garden[y], &Cell{
					Char:       RGB(139, 69, 19, "+"),
					StatusLine: fmt.Sprintf("You're standing in front of a weather-beaten sign that says %s.", ghrepo.FullName(geo.Repository)),
				})
				continue
			}

			if cellIx == len(commits)-1 {
				garden[y] = append(garden[y], grassCell)
				continue
			}

			chance := rand.Float64()
			if chance <= geo.Density {
				commit := commits[cellIx]
				garden[y] = append(garden[y], &Cell{
					Char:       commits[cellIx].Char,
					StatusLine: fmt.Sprintf("You're standing at a flower called %s planted by %s.", commit.Sha[0:6], commit.Handle),
				})
				cellIx++
			} else {
				garden[y] = append(garden[y], grassCell)
			}
		}
	}

	return garden
}

func drawGarden(out io.Writer, garden [][]*Cell, player *Player) {
	statusLine := ""
	for y, gardenRow := range garden {
		for x, gardenCell := range gardenRow {
			char := ""
			underPlayer := (player.X == x && player.Y == y)
			if underPlayer {
				statusLine = gardenCell.StatusLine
				char = utils.Bold(player.Char)

				if strings.Contains(gardenCell.StatusLine, "stream") {
					player.ShoeMoistureContent = 5
				} else {
					if player.ShoeMoistureContent > 1 {
						statusLine += "\nYour shoes squish with water from the stream."
					} else if player.ShoeMoistureContent == 1 {
						statusLine += "\nYour shoes seem to have dried out."
					}

					if player.ShoeMoistureContent > 0 {
						player.ShoeMoistureContent--
					}
				}
			} else {
				char = gardenCell.Char
			}

			fmt.Fprint(out, char)
		}
		fmt.Fprintln(out)
	}

	fmt.Println()
	fmt.Fprintln(out, utils.Bold(statusLine))
}

func shaToColorFunc(sha string) func(string) string {
	return func(c string) string {
		red, err := strconv.ParseInt(sha[0:2], 16, 64)
		if err != nil {
			panic(err)
		}

		green, err := strconv.ParseInt(sha[2:4], 16, 64)
		if err != nil {
			panic(err)
		}

		blue, err := strconv.ParseInt(sha[4:6], 16, 64)
		if err != nil {
			panic(err)
		}

		return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", red, green, blue, c)
	}
}

func computeSeed(seed string) int64 {
	lol := ""

	for _, r := range seed {
		lol += fmt.Sprintf("%d", int(r))
	}

	result, err := strconv.ParseInt(lol[0:10], 10, 64)
	if err != nil {
		panic(err)
	}

	return result
}

func clear(io *iostreams.IOStreams) {
	cmd := exec.Command("clear")
	cmd.Stdout = io.Out
	_ = cmd.Run()
}

func RGB(r, g, b int, x string) string {
	return fmt.Sprintf("\033[38;2;%d;%d;%dm%s\033[0m", r, g, b, x)
}
