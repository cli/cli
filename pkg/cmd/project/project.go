package project

import (
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
)

type ProjectOptions struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)

	Selector string
}

func NewCmdProject(f *cmdutil.Factory, runF func(*ProjectOptions) error) *cobra.Command {
	opts := ProjectOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
	}

	cmd := &cobra.Command{
		Use:    "project [<project id>]",
		Short:  "Interact with a GitHub project",
		Long:   "Enter an interactive UI for viewing and modifying a GitHub project",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			// support `-R, --repo` override
			opts.BaseRepo = f.BaseRepo

			if runF != nil {
				return runF(&opts)
			}
			return projectRun(&opts)
		},
	}

	return cmd
}

type Card struct {
	Note       string
	ID         int
	ContentURL string `json:"content_url"`
}

type Column struct {
	Name  string
	ID    int
	Cards []*Card
}

type Project struct {
	Name    string
	ID      int
	Columns []*Column
}

func projectRun(opts *ProjectOptions) error {
	// TODO interactively ask which project they want since these IDs are not easy to get
	projectID := 3514315

	c, err := opts.HttpClient()
	if err != nil {
		return err
	}
	client := api.NewClientFromHTTP(c)

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return err
	}

	project, err := getProject(client, baseRepo, projectID)
	if err != nil {
		return err
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

	// TODO some kind of controller struct to track modal state?
	colWidth := 30
	colHeight := 40
	colsToShow := 3

	cardWidth := colWidth - 2
	cardHeight := 5
	cardsToShow := 7

	selectedColumn := 0
	selectedCard := 0

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
				case 'w':
					if selectedCard > 0 {
						selectedCard--
					}
				case 's':
					if selectedCard < cardsToShow-1 {
						selectedCard++
					}
				case 'a':
					if selectedColumn > 0 {
						selectedColumn--
					}
				case 'd':
					if selectedColumn < colsToShow-1 {
						selectedColumn++
					}
				}
				switch ev.Key() {
				case tcell.KeyEscape:
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
		case <-time.After(time.Millisecond * 100):
		}
		s.Clear()
		drawStr(s, 0, 0, style, project.Name)
		for ix, col := range project.Columns {
			if ix == colsToShow {
				break
			}
			colX := colWidth * ix
			colY := 1
			drawRect(s, style, colX, colY, colWidth, colHeight, false)
			drawStr(s, colX+1, colY+1, style, col.Name)
			for ic, card := range col.Cards {
				if ic == cardsToShow {
					break
				}
				cardStyle := style
				bold := false
				if selectedColumn == ix && selectedCard == ic {
					cardStyle = style.Foreground(tcell.ColorGreen)
					bold = true
				}
				drawRect(s, cardStyle, colX+1, (ic*cardHeight)+colY+2, cardWidth, cardHeight, bold)
				cardNote := card.Note
				if len(card.Note) > cardWidth-2 {
					cardNote = cardNote[0 : cardWidth-2]
				} else if cardNote == "" {
					cardNote = `¯\_(ツ)_/¯`
				}
				drawStr(s, colX+2, (ic*cardHeight)+colY+3, style, cardNote)
			}
		}
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

func drawRect(s tcell.Screen, style tcell.Style, x, y, w, h int, bold bool) {
	// TODO could consolidate these now that I have the correct unicode characters
	topLeftCorner := '┌'
	botLeftCorner := '└'
	topRightCorner := '┐'
	botRightCorner := '┘'
	vertiLine := '│'
	horizLine := '─'

	if bold {
		topLeftCorner = '┏'
		botLeftCorner = '┗'
		topRightCorner = '┓'
		botRightCorner = '┛'
		vertiLine = '┃'
		horizLine = '━'
	}

	for ix := x; ix < x+w; ix++ {
		for iy := y; iy < y+h; iy++ {
			var c rune
			if ix == x && iy == y {
				c = topLeftCorner
			} else if ix == x && iy == y+h-1 {
				c = botLeftCorner
			} else if ix == x {
				c = vertiLine
			} else if ix == x+w-1 && iy == y {
				c = topRightCorner
			} else if ix == x+w-1 && iy == y+h-1 {
				c = botRightCorner
			} else if ix == x+w-1 {
				c = vertiLine
			} else if iy == y {
				c = horizLine
			} else if iy == y+h-1 {
				c = horizLine
			} else {
				continue
			}

			s.SetContent(ix, iy, c, nil, style)
		}
	}

}
