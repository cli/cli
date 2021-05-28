package mergeconflict

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/internal/config"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/prompt"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type MCOpts struct {
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams
	BaseRepo   func() (ghrepo.Interface, error)
	Debug      bool
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
	cmd.Flags().BoolVarP(&opts.Debug, "debug", "d", false, "enable logging")

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

func (g *GameObject) Update() {}

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
	// I disabled the background because i didn't like how spaces left behind had gray behind them while spaces between issues were black. mainly i just want it to be consistent.
	style := game.Style.Foreground(tcell.ColorWhite) //.Background(tcell.ColorBlack)
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

func (i *Issue) LetterAt(x int) rune {
	return rune(i.Sprite[x])
}

func (i *Issue) DestroyLetterAt(x int) {
	newSprite := ""
	for ix := 0; ix < i.w; ix++ {
		if ix == x {
			newSprite += " "
		} else {
			newSprite += string(i.Sprite[ix])
		}
	}
	i.Sprite = newSprite
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

type Burst struct {
	GameObject
	life int
}

func NewBurst(x, y int, g *Game) *Burst {
	style := g.Style.Foreground(tcell.ColorYellow)
	return &Burst{
		life: 3,
		GameObject: GameObject{
			x:             x - 1,
			y:             y - 1,
			w:             3,
			h:             3,
			Game:          g,
			StyleOverride: &style,
			Sprite: `\ /

/ \`,
		},
	}
}

func NewBigBurst(x, y int, g *Game) *Burst {
	style := g.Style.Foreground(tcell.ColorPink)
	return &Burst{
		life: 3,
		GameObject: GameObject{
			x:             x - 6,
			y:             y - 3,
			w:             13,
			h:             6,
			Game:          g,
			StyleOverride: &style,
			Sprite: `*     *
    \   /
*)___\ /___(*
     / \
    /   \
   *     *`,
		},
	}
}

func (b *Burst) Update() {
	if b.life == 0 {
		b.Game.Destroy(b)
		return
	}
	b.life--
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
	sha  string
}

func (cs *CommitShot) Update() {
	if cs.life == 0 {
		cs.Game.Destroy(cs)
	}
	cs.life--
}

func (cs *CommitShot) LetterAt(y int) rune {
	return rune(cs.sha[y])
}

func NewCommitShot(g *Game, x, y int, sha string) *CommitShot {
	sprite := ""
	for i := len(sha) - 1; i >= 0; i-- {
		sprite += string(sha[i]) + "\n"
	}
	return &CommitShot{
		life: 3,
		sha:  sha,
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
	cl.Game.DetectHits(ray, shot)
	cl.Game.AddDrawable(shot)
}

func NewCommitLauncher(g *Game, shas []string) *CommitLauncher {
	style := g.Style.Foreground(tcell.ColorPurple)
	return &CommitLauncher{
		Shas: shas,
		GameObject: GameObject{
			Sprite:        "-=$^$=-",
			w:             7,
			h:             1,
			Game:          g,
			StyleOverride: &style,
		},
	}
}

type CommitCounter struct {
	cl *CommitLauncher
	GameObject
}

func NewCommitCounter(x, y int, cl *CommitLauncher, game *Game) *CommitCounter {
	style := game.Style.Background(tcell.ColorCornflowerBlue)
	return &CommitCounter{
		cl: cl,
		GameObject: GameObject{
			x:             x,
			y:             y,
			h:             1,
			Sprite:        fmt.Sprintf("%d commits remain", len(cl.Shas)),
			Game:          game,
			StyleOverride: &style,
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
	style := game.Style.Foreground(tcell.ColorGold)
	text := fmt.Sprintf("SCORE: %d", 0)
	return &Score{
		GameObject: GameObject{
			x:             x,
			y:             y,
			w:             len(text),
			h:             1,
			Game:          game,
			Sprite:        text,
			StyleOverride: &style,
		},
	}
}

func (s *Score) Update() {
	text := fmt.Sprintf("SCORE: %d", s.score)
	s.Sprite = text
	s.w = len(text)
}

type Game struct {
	debug     bool
	drawables []Drawable
	Screen    tcell.Screen
	Style     tcell.Style
	MaxWidth  int
	Logger    *log.Logger
	State     map[string]interface{}
}

func (g *Game) Debugf(format string, v ...interface{}) {
	if g.debug == false {
		return
	}
	g.Logger.Printf(format, v...)
}

func (g *Game) LoadState() error {
	stateFilePath := filepath.Join(config.ConfigDir(), "mc.yml")

	g.State = map[string]interface{}{}
	g.State["HighScores"] = map[string]int{}

	content, err := ioutil.ReadFile(stateFilePath)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(content, &g.State)
	if err != nil {
		return err
	}

	return nil
}

func (g *Game) SaveState() error {
	// TODO

	return nil
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

func (g *Game) FilterGameObjects(fn func(Drawable) bool) []Drawable {
	out := []Drawable{}
	for _, gobj := range g.drawables {
		if fn(gobj) {
			out = append(out, gobj)
		}
	}
	return out
}

func (g *Game) DetectHits(r *Ray, shot *CommitShot) {
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

	// TODO dirty to do side effects in a filter, consider renaming/tweaking
	_ = g.FilterGameObjects(func(gobj Drawable) bool {
		issue, ok := gobj.(*Issue)
		if !ok {
			return false
		}
		shotX := r.Points[0].X
		shotY := r.Points[len(r.Points)-1].Y
		if shotX < issue.x || shotX >= issue.x+issue.w {
			return false
		}

		r := issue.LetterAt(shotX - issue.x)
		if r == ' ' {
			return false
		}

		thisShot++

		issue.DestroyLetterAt(shotX - issue.x)

		var burst *Burst

		if r == shot.LetterAt(shotY-issue.y) {
			g.Debugf("OMG CHARACTER HIT %s\n", string(r))
			matchesMultiplier *= 2
			burst = NewBigBurst(shotX, issue.y, g)
		} else {
			burst = NewBurst(shotX, issue.y, g)
		}
		g.AddDrawable(burst)

		return true
	})

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

func NewLegend(x, y int, game *Game) *GameObject {
	return &GameObject{
		x:    x,
		y:    y,
		Game: game,
		Sprite: `move:  ← →
space: fire
q:     quit`,
	}
}

func NewHighScores(x, y int, g *Game) *GameObject {
	sprite := "~* high scores *~"
	highScores, ok := g.State["HighScores"].(map[string]int)
	if ok {
		for k, v := range highScores {
			sprite += fmt.Sprintf("\n%s %d", k, v)
		}
	}
	return &GameObject{
		x:      x,
		y:      y,
		Game:   g,
		Sprite: sprite,
	}
}

type ScoreLog struct {
	GameObject
	log []string
}

func NewScoreLog(x, y int, game *Game) *ScoreLog {
	style := game.Style.Foreground(tcell.ColorBlack).Background(tcell.ColorWhite)
	return &ScoreLog{
		GameObject: GameObject{
			x:             x,
			y:             y,
			Game:          game,
			StyleOverride: &style,
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

	debug := opts.Debug

	var logger *log.Logger
	if debug {
		f, _ := os.Create("mclog.txt")
		logger = log.New(f, "", log.Lshortfile)
		logger.Println("hey what's up")
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

	game := &Game{
		debug:    debug,
		Screen:   s,
		Style:    style,
		MaxWidth: 80,
		Logger:   logger,
	}

	err = game.LoadState()
	if err != nil {
		game.Debugf("failed to load state: %s", err)
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

	score := NewScore(38, 18, game)
	game.AddDrawable(score)

	scoreLog := NewScoreLog(15, 15, game)
	game.AddDrawable(scoreLog)

	game.AddDrawable(NewLegend(1, 15, game))

	highScores := NewHighScores(60, 15, game)
	game.AddDrawable(highScores)

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

	// TODO this following code is very bad, abstract to function and clean up
	// TODO GetState helper on Game
	// TODO likely reference issue on the high score map
	hs := map[string]int{}
	hs, ok := game.State["HighScores"].(map[string]int)
	if !ok {
		game.Debugf("failed to save high scores")
		return nil
	}

	maxScore := 0
	for _, v := range hs {
		if v > maxScore {
			maxScore = v
		}
	}

	if score.score >= maxScore && score.score > 0 {
		answer := false
		err = prompt.SurveyAskOne(
			&survey.Confirm{
				Message: "new high score! save it?",
			}, &answer)
		if err == nil && answer {
			answer := ""
			err = prompt.SurveyAskOne(
				&survey.Input{
					Message: "name",
				}, &answer)
			if err == nil {
				hs[answer] = score.score
				err = game.SaveState()
				if err != nil {
					game.Debugf("failed to save state: %s", err)
				}
			}
		}
	}

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
