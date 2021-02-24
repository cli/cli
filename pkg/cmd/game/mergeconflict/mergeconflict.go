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

func (cl *CommitLauncher) launch() {
	// TODO
}

func NewCommitLauncher(shas []string) *CommitLauncher {
	return &CommitLauncher{
		shas: shas,
		GameObject: GameObject{
			Sprite: "-=$^$=-",
			w:      7,
			h:      1,
		},
	}
}

func mergeconflictRun(opts *MCOpts) error {
	// TODO

	cl := NewCommitLauncher([]string{
		"1234567890",
		"0987654321",
	})

	cl.Transform(37, 12)

	gobjs := []Drawable{
		cl,
	}

	style := tcell.StyleDefault

	s, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = s.Init(); err != nil {
		return err
	}

	s.SetStyle(style)
	quit := make(chan struct{})
	go func() {
		for {
			ev := s.PollEvent()
			switch ev := ev.(type) {
			case *tcell.EventKey:
				switch ev.Rune() {
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
		for _, gobj := range gobjs {
			gobj.Draw(s, style)
		}
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
