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
	x             int
	y             int
	w             int
	h             int
	Sprite        string
	Game          *Game
	StyleOverride *tcell.Style
}

func (g *GameObject) Transform(x, y int) {
	g.x += x
	g.y += y
}

func (g *GameObject) Draw() {
	screen := g.Game.Screen
	style := g.Game.Style
	if g.StyleOverride != nil {
		style = *g.StyleOverride
	}
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
	style := game.Style.Foreground(tcell.ColorWhite).Background(tcell.ColorBlack)
	return &Issue{
		dir: dir,
		GameObject: GameObject{
			x:             x,
			y:             y,
			w:             len(text),
			h:             1,
			Sprite:        text,
			Game:          game,
			StyleOverride: &style,
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
	if is.countdown > 0 || len(is.issues) == 0 {
		// TODO eventually, notice if all spawners are empty and trigger win condition
		//is.Game.Debugf("%s is dry", is)
		return
	}

	issueText := is.issues[0]
	is.issues = is.issues[1:]

	is.countdown = len(issueText) + 3 // add arbitrary cool off

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

func (is *IssueSpawner) AddIssue(issue string) {
	is.issues = append(is.issues, issue)
}

type CommitLauncher struct {
	GameObject
	cooldown     int // prevents double shooting which make bullets collide
	Shas         []string
	rainbowIndex int
}

type CommitShot struct {
	GameObject
	life int
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

func (cl *CommitLauncher) ColorForShot(sha string) tcell.Style {
	style := cl.Game.Style
	switch cl.rainbowIndex {
	case 0:
		style = style.Foreground(tcell.ColorRed)
	case 1:
		style = style.Foreground(tcell.ColorOrange)
	case 2:
		style = style.Foreground(tcell.ColorYellow)
	case 3:
		style = style.Foreground(tcell.ColorGreen)
	case 4:
		style = style.Foreground(tcell.ColorBlue)
	case 5:
		style = style.Foreground(tcell.ColorIndigo)
	case 6:
		style = style.Foreground(tcell.ColorPurple)
	}
	cl.rainbowIndex++
	cl.rainbowIndex %= 7
	return style
}

func (cl *CommitLauncher) Launch() {
	if cl.cooldown > 0 {
		return
	}
	cl.cooldown = 4

	if len(cl.Shas) == 1 {
		// TODO need to signal game over
		return
	}
	sha := cl.Shas[0]
	cl.Shas = cl.Shas[1:]
	shotX := cl.x + 3
	shotY := cl.y - len(sha)
	shot := NewCommitShot(cl.Game, shotX, shotY, sha)
	style := cl.ColorForShot(sha)
	shot.StyleOverride = &style
	// TODO add ToRay to CommitShot
	ray := &Ray{}
	for i := 0; i < len(sha); i++ {
		ray.AddPoint(shotX, shotY+i)
	}
	cl.Game.DetectHits(ray, shot.Sprite)
	cl.Game.AddDrawable(shot)
}

func NewCommitLauncher(g *Game, shas []string) *CommitLauncher {
	return &CommitLauncher{
		Shas: shas,
		GameObject: GameObject{
			Sprite: "-=$^$=-",
			w:      7,
			h:      1,
			Game:   g,
		},
	}
}

type CommitCounter struct {
	cl *CommitLauncher
	GameObject
}

func NewCommitCounter(x, y int, cl *CommitLauncher, game *Game) *CommitCounter {
	return &CommitCounter{
		cl: cl,
		GameObject: GameObject{
			x:      x,
			y:      y,
			h:      1,
			Sprite: fmt.Sprintf("%d commits remain", len(cl.Shas)),
			Game:   game,
		},
	}
}

func (cc *CommitCounter) Update() {
	sprite := fmt.Sprintf("%d commits remain", len(cc.cl.Shas))
	cc.Sprite = sprite
	cc.w = len(sprite)
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

func (g *Game) DetectHits(r *Ray, shaText string) {
	score := g.FindGameObject(func(gobj Drawable) bool {
		_, ok := gobj.(*Score)
		return ok
	})
	scoreLog := g.FindGameObject(func(gobj Drawable) bool {
		_, ok := gobj.(*ScoreLog)
		return ok
	})
	if score == nil {
		panic("could not find score game object")
	}
	if scoreLog == nil {
		panic("could not find score log game object")
	}
	thisShot := 0
	matchesMultiplier := 1
	for i, point := range r.Points {
		r, _, _, _ := g.Screen.GetContent(point.X, point.Y)
		g.Debugf("found at point %s: %s\n", point, string(r))
		if r == ' ' {
			continue
		}
		if byte(r) == shaText[i] {
			g.Debugf("OMG CHARACTER HIT %s\n", string(r))
			matchesMultiplier *= 2
		}

		thisShot++
	}

	bonus := false
	if thisShot == 10 {
		matchesMultiplier *= 2
	}
	if matchesMultiplier > 1 {
		bonus = true
		thisShot *= matchesMultiplier
	}

	if thisShot > 0 {
		scoreLog.(*ScoreLog).Log(thisShot, bonus)
		score.(*Score).Add(thisShot)
	}
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

type ScoreLog struct {
	GameObject
	log []string
}

func NewScoreLog(x, y int, game *Game) *ScoreLog {
	return &ScoreLog{
		GameObject: GameObject{
			x:    x,
			y:    y,
			Game: game,
		},
	}
}

func (sl *ScoreLog) Update() {
	sl.Sprite = strings.Join(sl.log, "\n")
	sl.h = len(sl.log)
	sl.w = 15
}

func (sl *ScoreLog) Log(value int, get bool) {
	msg := fmt.Sprintf("%d points!", value)
	if get {
		msg = fmt.Sprintf("%d points BONUS GET!", value)
	}
	sl.log = append(sl.log, msg)
	if len(sl.log) > 5 {
		sl.log = sl.log[1:]
	}
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

	cc := NewCommitCounter(35, 14, cl, game)
	game.AddDrawable(cc)

	score := NewScore(40, 18, game)
	game.AddDrawable(score)

	scoreLog := NewScoreLog(15, 15, game)
	game.AddDrawable(scoreLog)

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
	// - high score listing
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
		titleStyle := style.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
		drawStr(s, 25, 0, titleStyle, "!!! M E R G E  C O N F L I C T !!!")
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
