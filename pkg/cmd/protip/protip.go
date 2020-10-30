package protip

import (
	"fmt"
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
			"clippy":  Clippy(),
			"manatee": Manatee(),
		},
	}

	cmd := &cobra.Command{
		Use:   "protip",
		Short: "get a random protip about using gh",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Character = strings.ToLower(opts.Character)
			if s, ok := opts.Sprites[opts.Character]; ok {
				opts.Sprite = s
			} else {
				return fmt.Errorf("no such character: %s", opts.Character)
			}

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

	tip := getProtip()

	style := tcell.StyleDefault

	s, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = s.Init(); err != nil {
		return err
	}

	s.SetStyle(style.
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
		case <-time.After(time.Millisecond * 500):
		}
		s.Clear()
		drawSprite(s, 0, 0, style, opts.Sprite)
		drawProtip(s, opts.Sprite.Width, 1, style, tip)
		s.Show()
	}

	s.Fini()

	return nil
}

func drawProtip(s tcell.Screen, startX, startY int, st tcell.Style, tipLines []string) {
	// Should look like this:
	/*

	  *--------------------------------*
	  | To merge a PR, review it until |
	  | it is approved.                |
	  |________________________________*
	 /

	*/

	width := len(tipLines[0]) + 5
	pad := func(s string) string {
		out := " | " + s
		spaces := width - len(out) - 1
		for i := 0; i < spaces; i++ {
			out += " "
		}
		out += "|"

		return out
	}

	// draw top border
	topBorder := " *"
	bottomBorder := " |"
	for x := 0; x < width-4; x++ {
		topBorder += "-"
		bottomBorder += "_"
	}
	topBorder += "-*"
	bottomBorder += "_*"

	drawStr(s, startX, startY, st, topBorder)

	y := startY + 1

	for iy, line := range tipLines {
		drawStr(s, startX, y+iy, st, pad(line))
		y += iy
	}

	drawStr(s, startX, y+1, st, bottomBorder)
	drawStr(s, startX, y+2, st, "/")
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

func drawSprite(s tcell.Screen, x, y int, st tcell.Style, sp *Sprite) {
	cells := sp.Cells()
	for y := 0; y < len(cells); y++ {
		row := cells[y]
		for x := 0; x < len(row); x++ {
			s.SetContent(x, y, cells[y][x], nil, st)
		}
	}
	sp.IncrFrame()
}
