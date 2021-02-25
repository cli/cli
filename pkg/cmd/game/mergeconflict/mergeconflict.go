package mergeconflict

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
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
	issues    []string
	countdown int
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

func (is *IssueSpawner) Update() {
	if is.countdown > 0 {
		is.countdown--
	}
}

func (is *IssueSpawner) Spawn() {
	// TODO LOCKING AND UNLOCKING
	if is.countdown > 0 || len(is.issues) == 0 {
		// TODO eventually, notice if all spawners are empty and trigger win condition
		//is.Game.Debugf("%s is dry", is)
		return
	}

	issueText := is.issues[0]
	is.issues = is.issues[1:]

	is.countdown = len(issueText) + 5 // add arbitrary cool off

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
	cooldown int // prevents double shooting which make bullets collide
	shas     []string
}

type CommitShot struct {
	GameObject
	life int
	// TODO store colors here i guess?
}

func (cs *CommitShot) Update() {
	if cs.life == 0 {
		cs.Game.Destroy(cs)
	}
	cs.life--
}

func NewCommitShot(g *Game, x, y int, sha string) *CommitShot {
	sprite := ""
	for _, c := range sha {
		sprite += string(c) + "\n"
	}
	return &CommitShot{
		life: 3,
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

func (cl *CommitLauncher) Update() {
	if cl.cooldown > 0 {
		cl.cooldown--
	}
}

func (cl *CommitLauncher) Launch() {
	if cl.cooldown > 0 {
		return
	}
	cl.cooldown = 4

	if len(cl.shas) == 1 {
		// TODO need to signal game over
		return
	}
	sha := cl.shas[0]
	cl.shas = cl.shas[1:]
	shotX := cl.x + 3
	shotY := cl.y - len(sha)
	shot := NewCommitShot(cl.Game, shotX, shotY, sha)
	// TODO add ToRay to CommitShot
	ray := &Ray{}
	for i := 0; i < len(sha); i++ {
		ray.AddPoint(shotX, shotY+i)
	}
	cl.Game.DetectHits(ray)
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

type Score struct {
	GameObject
	score int
}

func (s *Score) Add(i int) {
	s.score += i
}

func NewScore(x, y int, game *Game) *Score {
	text := fmt.Sprintf("SCORE: %d", 0)
	return &Score{
		GameObject: GameObject{
			x:      x,
			y:      y,
			w:      len(text),
			h:      1,
			Game:   game,
			Sprite: text,
		},
	}
}

func (s *Score) Update() {
	text := fmt.Sprintf("SCORE: %d", s.score)
	s.Sprite = text
	s.w = len(text)
}

type Game struct {
	drawables []Drawable
	Screen    tcell.Screen
	Style     tcell.Style
	MaxWidth  int
	Logger    *log.Logger
}

func (g *Game) Debugf(format string, v ...interface{}) {
	g.Logger.Printf(format, v...)
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

func (g *Game) Update() {
	for _, gobj := range g.drawables {
		gobj.Update()
	}
}

func (g *Game) Draw() {
	for _, gobj := range g.drawables {
		gobj.Draw()
	}
}

func (g *Game) FindGameObject(fn func(Drawable) bool) Drawable {
	for _, gobj := range g.drawables {
		if fn(gobj) {
			return gobj
		}
	}
	return nil
}

func (g *Game) DetectHits(r *Ray) {
	score := g.FindGameObject(func(gobj Drawable) bool {
		_, ok := gobj.(*Score)
		return ok
	})
	if score == nil {
		panic("could not find score game object")
	}
	thisShot := 0
	for _, point := range r.Points {
		r, _, _, _ := g.Screen.GetContent(point.X, point.Y)
		g.Debugf("found at point %s: %s\n", point, string(r))
		if r == ' ' {
			continue
		}
		thisShot++
	}
	// TODO if this knows about the shas then i can do additional bonus based on matching characters
	if thisShot == 10 {
		// TODO announce GET! in a hype zone
		g.Debugf("GET!\n")
		thisShot *= 2
	}

	score.(*Score).Add(thisShot)
}

type Point struct {
	X int
	Y int
}

func (p Point) String() string {
	return fmt.Sprintf("<%d, %d>", p.X, p.Y)
}

type Ray struct {
	Points []Point
}

func (r *Ray) AddPoint(x, y int) {
	r.Points = append(r.Points, Point{X: x, Y: y})
}

func mergeconflictRun(opts *MCOpts) error {
	repo, err := opts.BaseRepo()
	if err != nil {
		return fmt.Errorf("could not determine base repo: %w", err)
	}
	client, err := opts.HttpClient()
	if err != nil {
		return fmt.Errorf("could not build http client: %w", err)
	}

	f, _ := os.Create("mclog.txt")
	logger := log.New(f, "", log.Lshortfile)
	logger.Println("hey what's up")

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
		Logger:   logger,
	}

	issues, err := getIssues(client, repo)
	if err != nil {
		return fmt.Errorf("failed to get issues for %s: %w", ghrepo.FullName(repo), err)
	}

	rand.Shuffle(len(issues), func(i, j int) {
		issues[i], issues[j] = issues[j], issues[i]
	})

	// TODO enforce game dimensions, don't bother supporting resizes

	issueSpawners := []*IssueSpawner{}
	y := 2
	x := 0
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			x = 0
		} else {
			x = game.MaxWidth
		}

		is := NewIssueSpawner(x, y+i, game)

		issueSpawners = append(issueSpawners, is)
		game.AddDrawable(is)
	}

	for ix, issueText := range issues {
		spawnerIx := ix % len(issueSpawners)
		issueSpawners[spawnerIx].AddIssue(issueText)
	}

	shas, err := getShas(client, repo)
	if err != nil {
		return fmt.Errorf("failed to get shas for %s: %w", ghrepo.FullName(repo), err)
	}

	cl := NewCommitLauncher(game, shas)

	cl.Transform(37, 13)

	game.AddDrawable(cl)

	score := NewScore(40, 17, game)
	game.AddDrawable(score)

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

	// TODO UI
	// - hype zone / score log
	// - high score listing
	// - commits left
	// - "now playing" note
	// - key legend
	// TODO high score saving/loading

loop:
	for {
		select {
		case <-quit:
			break loop
		case <-time.After(time.Millisecond * 100):
		}

		s.Clear()
		spawner := issueSpawners[rand.Intn(len(issueSpawners))]
		spawner.Spawn()
		game.Update()
		game.Draw()
		drawStr(s, 20, 0, style, "!!! M E R G E  C O N F L I C T !!!")
		s.Show()
	}

	s.Fini()

	return nil
}

func drawStr(s tcell.Screen, x, y int, style tcell.Style, str string) {
	// TODO put this into Game
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
