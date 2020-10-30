package protip

import "strings"

type Sprite struct {
	Width   int
	Height  int
	Frames  []string
	frameIx int
}

func (s *Sprite) AddFrame(f string) {
	s.Frames = append(s.Frames, f)
}

func (s *Sprite) IncrFrame() {
	s.frameIx = (s.frameIx + 1) % len(s.Frames)
}

func (s *Sprite) Cells() [][]rune {
	out := [][]rune{}
	curFrame := s.Frames[s.frameIx]
	lines := strings.Split(curFrame, "\n")
	for _, line := range lines {
		out = append(out, []rune(line))
	}
	return out
}

func Manatee() *Sprite {
	s := &Sprite{
		Width:  40,
		Height: 14,
	}

	s.AddFrame(`
                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/



(art by MJP)
`)
	s.AddFrame(`

                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/


(art by MJP)
`)
	s.AddFrame(`


                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/

(art by MJP)
`)
	s.AddFrame(`



                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/
(art by MJP)
`)
	s.AddFrame(`


                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/

(art by MJP)
`)
	s.AddFrame(`

                     _.---.._
        _        _.-'         ''-.
      .'  '-,_.-'                 '''.
     (       _                     o  :
      '._ .-'  '-._         \  \-  ---]
                    '-.___.-')  )..-'
                             (_/


(art by MJP)
`)

	return s
}

func Clippy() *Sprite {
	s := &Sprite{
		Width:  15,
		Height: 15,
	}
	s.AddFrame(`
        ___
       ^   ^
       O   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       O   O
       |   |
       ||  |
       ||_/  -
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       O   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       o   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       -   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       o   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)
	s.AddFrame(`
        ___
       ^   ^
       O   O
       |   |
       ||  |
       ||_/  /
       |    |
       |   |
       \___/

`)

	return s
}
