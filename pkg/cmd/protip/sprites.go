package protip

func Clippy() *Sprite {
	s := &Sprite{15, 15}
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
