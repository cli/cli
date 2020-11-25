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
	Note string
	ID   int
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

	// some kind of controller struct to track modal state?

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
		drawStr(s, 0, 0, style, project.Name)
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
