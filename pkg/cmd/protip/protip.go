package protip

import (
	"strings"
	"time"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

type ProtipOptions struct {
	IO *iostreams.IOStreams

	Character string
	Sprites   map[string]*Sprite
	Sprite    *Sprite
}

func NewCmdProtip(f *cmdutil.Factory, runF func(*ProtipOptions) error) *cobra.Command {
	opts := &ProtipOptions{
		IO: f.IOStreams,
		Sprites: map[string]*Sprite{
			"clippy": Clippy(),
		},
	}

	cmd := &cobra.Command{
		Use:   "protip",
		Short: "get a random protip about using gh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Character = strings.ToLower(opts.Character)
			// TODO error handling
			opts.Sprite = opts.Sprites[opts.Character]
			if runF != nil {
				return runF(opts)
			}
			return protipRun(opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Character, "character", "c", "clippy", "What helpful animated character you'd like a protip from")

	return cmd
}

func protipRun(opts *ProtipOptions) error {
	tcell.SetEncodingFallback(tcell.EncodingFallbackASCII)

	s, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = s.Init(); err != nil {
		return err
	}

	s.SetStyle(tcell.StyleDefault.
		Foreground(tcell.ColorGreen).
		Background(tcell.ColorBlack))
	s.Clear()

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
				case tcell.KeyEscape, tcell.KeyEnter:
					close(quit)
					return
				case tcell.KeyCtrlL:
					s.Sync()
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
		case <-time.After(time.Millisecond * 50):
		}
		w, h := s.Size()
		s.Clear()
		emitStr(s, w/2-7, h/2, tcell.StyleDefault, "TODO a protip and stuff")
		s.Show()
	}

	s.Fini()

	return nil
}

func emitStr(s tcell.Screen, x, y int, style tcell.Style, str string) {
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

// Want to be able to have a sprite that occupies a rectangle and can animate internally relative to
// its own geometry.

type Sprite struct {
	Width  int
	Height int
	Frames []string
}

func (s *Sprite) AddFrame(f string) {
	s.Frames = append(s.Frames, f)
}
