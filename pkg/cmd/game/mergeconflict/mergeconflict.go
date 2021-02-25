package mergeconflict

import (
	"math/rand"
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
	Draw()
	Update()
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

func (g *GameObject) Draw() {
	screen := g.Game.Screen
	style := g.Game.Style
	lines := strings.Split(g.Sprite, "\n")
	for i, l := range lines {
		drawStr(screen, g.x, g.y+i, style, l)
	}
}

type Direction int // either -1 or 1

type Issue struct {
	GameObject
	dir Direction
}

func NewIssue(x, y int, dir Direction, text string, game *Game) *Issue {
	return &Issue{
		dir: dir,
		GameObject: GameObject{
			x:      x,
			y:      y,
			w:      len(text),
			h:      1,
			Sprite: text,
			Game:   game,
		},
	}
}

func (i *Issue) Update() {
	i.Transform(int(i.dir), 0)
	if i.dir > 0 && i.x > 5+i.Game.MaxWidth {
		// hoping this is enough for GC to claim
		i.Game.Destroy(i)
	}

	if i.dir < 0 && i.x < -5-len(i.Sprite) {
		i.Game.Destroy(i)
	}
}

// would be nice to just call "spawn" at random intervals but have the spawner lock itself if it's already got something still going
// how should it track if it's active?
type IssueSpawner struct {
	GameObject
	issues []string
	locked bool
}

func NewIssueSpawner(x, y int, game *Game) *IssueSpawner {
	return &IssueSpawner{
		GameObject: GameObject{
			x:    x,
			y:    y,
			Game: game,
		},
	}
}

func (is *IssueSpawner) Spawn() {
	// TODO LOCKING AND UNLOCKING
	if !is.locked || len(is.issues) == 0 {
		// TODO eventually, notice if all spawners are empty and trigger win condition
		return
	}

	// TODO punting on unlocking for now so each spawner will only spawn once
	is.locked = true

	issueText := is.issues[0]
	is.issues = is.issues[1:]
	// is.x is either 0 or maxwidth
	x := is.x
	var dir Direction
	dir = -1
	if is.x == 0 {
		x = 0 - len(issueText) + 1
		dir = 1
	}

	is.Game.AddDrawable(NewIssue(x, is.y, dir, issueText, is.Game))
}

// TODO grab full issue list, shuffle it, then round robin add to each issue spawner one at a time

func (is *IssueSpawner) AddIssue(issue string) {
	is.issues = append(is.issues, issue)
}

type CommitLauncher struct {
	GameObject
	shas []string
}

type CommitShot struct {
	GameObject
	// TODO store colors here i guess?
}

func (cs *CommitShot) Update() {}

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

func (cl *CommitLauncher) Update() {}

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
	MaxWidth  int
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
		gobj.Draw()
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
		Screen:   s,
		Style:    style,
		MaxWidth: 80,
	}

	// TODO get real issues
	issues := []string{
		"#123 Florp the jobnicorn",
		"#666 your software has inadvertantly caused a rift between realities and now a legion of demons is slipping through",
		"#56789 This repository has TOO MANY ISSUES",
		"#126 Flop the bazbotter",
		"#223 bump a dependency",
		"#323 quux a barbaz",
		"#423 bazzle the foobar machine",
		"#523 bazbar the fooquux",
		"#623 have you ever really thought about",
		"#723 there is nothing",
		"#823 but have you ever really looked at the stars",
		"#923 too many icicles",
		"#133 determine best way to bar the foo",
		"#143 refactor the test suite's implementation of things that should never be said aloud",
		"#163 it's miller time",
	}

	rand.Shuffle(len(issues), func(i, j int) {
		issues[i], issues[j] = issues[j], issues[i]
	})

	// TODO enforce game dimensions, don't bother supporting resizes

	issueSpawners := []*IssueSpawner{}
	i := 0
	y := 2
	x := 0
	for i < 10 {
		if i%2 == 0 {
			x = 0
		} else {
			x = game.MaxWidth
		}

		issueSpawners = append(issueSpawners, NewIssueSpawner(x, y+i, game))

		i++
	}

	for ix, issueText := range issues {
		spawnerIx := ix % len(issueSpawners)
		issueSpawners[spawnerIx].AddIssue(issueText)
	}

	// TODO get real commits
	cl := NewCommitLauncher(game, []string{
		"42c111790c",
		"bd86fdfe2e",
		"a16d7a0c56",
		"5698c23c10",
		"d24c3076e3",
		"e4ce0d76aa",
		"896f2273e8",
		"66d4307bce",
		"79b77b4273",
		"61eb7eeeab",
		"56ead91702",
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

	// TODO scroll issue titles
	// TODO collision detection
	// TODO removal of shas after firing
	// TODO UI

loop:
	for {
		select {
		case <-quit:
			break loop
		case <-time.After(time.Millisecond * 100):
		}

		s.Clear()
		// TODO
		spawner := issueSpawners[rand.Intn(len(issueSpawners))]
		spawner.Spawn()
		// taking a break to eat, next is:
		// - pick a random spawner
		// - call spawn
		// - figure out locking
		// - consider an Update function for all drawables so that issues can animate themselves across the screen
		// - actually animate the issues
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
