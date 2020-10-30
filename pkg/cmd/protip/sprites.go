package protip

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
