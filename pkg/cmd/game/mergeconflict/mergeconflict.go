package mergeconflict

import (
	"net/http"
	"strings"
	"time"

	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

type MCOpts struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
}

func NewCmdMergeconflict(f *cmdutil.Factory, runF func(*MCOpts) error) *cobra.Command {
	opts := &MCOpts{
		HttpClient: f.HttpClient,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "mergeconflict",
		Short: "A game about issue triage",
		RunE: func(cmd *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo
			if runF != nil {
				return runF(opts)
			}

			return mergeconflictRun(opts)
		},
	}

	return cmd
}

type Drawable interface {
	Draw(s tcell.Screen, style tcell.Style)
}

type GameObject struct {
	x      int
	y      int
	w      int
	h      int
	Sprite string
	Game   *Game
}

func (g *GameObject) Transform(x, y int) {
	g.x += x
	g.y += y
}

func (g *GameObject) Draw(s tcell.Screen, style tcell.Style) {
	lines := strings.Split(g.Sprite, "\n")
	for i, l := range lines {
		drawStr(s, g.x, g.y+i, style, l)
	}
}

type CommitLauncher struct {
	GameObject
	shas []string
}

type CommitShot struct {
	GameObject
	// TODO store colors here i guess?
}

func NewCommitShot(g *Game, x, y int, sha string) *CommitShot {
	sprite := ""
	for _, c := range sha {
		sprite += string(c) + "\n"
	}
	return &CommitShot{
		GameObject: GameObject{
			Sprite: sprite,
			x:      x,
			y:      y,
			w:      1,
			h:      len(sha),
			Game:   g,
		},
	}
}

func (cl *CommitLauncher) Launch() {
	if len(cl.shas) == 1 {
		// TODO need to signal game over
		return
	}
	sha := cl.shas[0]
	cl.shas = cl.shas[1:]
	shot := NewCommitShot(cl.Game, cl.x+3, cl.y-len(sha), sha)
	cl.Game.AddDrawable(shot)
}

func NewCommitLauncher(g *Game, shas []string) *CommitLauncher {
	return &CommitLauncher{
		shas: shas,
		GameObject: GameObject{
			Sprite: "-=$^$=-",
			w:      7,
			h:      1,
			Game:   g,
		},
	}
}

type Game struct {
	drawables []Drawable
	Screen    tcell.Screen
	Style     tcell.Style
}

func (g *Game) AddDrawable(d Drawable) {
	g.drawables = append(g.drawables, d)
}

func (g *Game) Destroy(d Drawable) {
	newDrawables := []Drawable{}
	for _, dd := range g.drawables {
		if dd == d {
			continue
		}
		newDrawables = append(newDrawables, dd)
	}
	g.drawables = newDrawables
}

func (g *Game) Draw() {
	for _, gobj := range g.drawables {
		gobj.Draw(g.Screen, g.Style)
	}
}

func mergeconflictRun(opts *MCOpts) error {
	style := tcell.StyleDefault

	s, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = s.Init(); err != nil {
		return err
	}
	s.SetStyle(style)

	game := &Game{
		Screen: s,
		Style:  style,
	}

	cl := NewCommitLauncher(game, []string{
		"42c111790cdfff5",
		"bd86fdfe2e43049",
		"a16d7a0c5650212",
		"5698c23c1041df3",
		"d24c3076e3c3a04",
		"e4ce0d76aac90a0",
		"896f2273e85da9a",
		"66d4307bce0d018",
		"79b77b4273f2ce3",
		"61eb7eeeab3f346",
		"56ead91702e6157",
	})

	cl.Transform(37, 20)

	game.AddDrawable(cl)

	quit := make(chan struct{})
	go func() {
		for {
			ev := s.PollEvent()
			switch ev := ev.(type) {
			case *tcell.EventKey:
				switch ev.Rune() {
				case ' ':
					cl.Launch()
				case 'q':
					close(quit)
					return
				}
				switch ev.Key() {
				case tcell.KeyEscape:
					close(quit)
					return
				case tcell.KeyCtrlL:
					s.Sync()
				case tcell.KeyLeft:
					cl.Transform(-1, 0)
				case tcell.KeyRight:
					cl.Transform(1, 0)
				}
			case *tcell.EventResize:
				s.Sync()
			}
		}
	}()

loop:
	for {
		select {
		case <-quit:
			break loop
		case <-time.After(time.Millisecond * 100):
		}

		s.Clear()
		game.Draw()
		drawStr(s, 20, 0, style, "!!! M E R G E  C O N F L I C T !!!")
		s.Show()
	}

	s.Fini()

	return nil
}

func drawStr(s tcell.Screen, x, y int, style tcell.Style, str string) {
	for _, c := range str {
		var comb []rune
		w := runewidth.RuneWidth(c)
		if w == 0 {
			comb = []rune{c}
			c = ' '
			w = 1
		}
		s.SetContent(x, y, c, comb, style)
		x += w
	}
}
