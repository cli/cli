package terminal

import (
	"fmt"
	"unicode"
)

type RuneReader struct {
	stdio  Stdio
	cursor *Cursor
	state  runeReaderState
}

func NewRuneReader(stdio Stdio) *RuneReader {
	return &RuneReader{
		stdio: stdio,
		state: newRuneReaderState(stdio.In),
	}
}

func (rr *RuneReader) printChar(char rune, mask rune) {
	// if we don't need to mask the input
	if mask == 0 {
		// just print the character the user pressed
		fmt.Fprintf(rr.stdio.Out, "%c", char)
	} else {
		// otherwise print the mask we were given
		fmt.Fprintf(rr.stdio.Out, "%c", mask)
	}
}

func (rr *RuneReader) ReadLine(mask rune) ([]rune, error) {
	line := []rune{}
	// we only care about horizontal displacements from the origin so start counting at 0
	index := 0

	cursor := &Cursor{
		In:  rr.stdio.In,
		Out: rr.stdio.Out,
	}

	// we get the terminal width and height (if resized after this point the property might become invalid)
	terminalSize, _ := cursor.Size(rr.Buffer())
	// we set the current location of the cursor once
	cursorCurrent, _ := cursor.Location(rr.Buffer())

	for {
		// wait for some input
		r, _, err := rr.ReadRune()
		if err != nil {
			return line, err
		}
		// increment cursor location
		cursorCurrent.X++

		// if the user pressed enter or some other newline/termination like ctrl+d
		if r == '\r' || r == '\n' || r == KeyEndTransmission {
			// delete what's printed out on the console screen (cleanup)
			for index > 0 {
				if cursorCurrent.CursorIsAtLineBegin() {
					EraseLine(rr.stdio.Out, ERASE_LINE_END)
					cursor.PreviousLine(1)
					cursor.Forward(int(terminalSize.X))
					cursorCurrent.X = terminalSize.X
					cursorCurrent.Y--

				} else {
					cursor.Back(1)
					cursorCurrent.X--
				}
				index--
			}
			// move the cursor the a new line
			cursor.MoveNextLine(cursorCurrent, terminalSize)

			// we're done processing the input
			return line, nil
		}
		// if the user interrupts (ie with ctrl+c)
		if r == KeyInterrupt {
			// go to the beginning of the next line
			fmt.Fprint(rr.stdio.Out, "\r\n")

			// we're done processing the input, and treat interrupt like an error
			return line, InterruptErr
		}

		// allow for backspace/delete editing of inputs
		if r == KeyBackspace || r == KeyDelete {
			// and we're not at the beginning of the line
			if index > 0 && len(line) > 0 {
				// if we are at the end of the word
				if index == len(line) {
					// just remove the last letter from the internal representation
					line = line[:len(line)-1]
					// go back one
					if cursorCurrent.X == 1 {
						cursor.PreviousLine(1)
						cursor.Forward(int(terminalSize.X))
					} else {
						cursor.Back(1)
					}

					// clear the rest of the line
					EraseLine(rr.stdio.Out, ERASE_LINE_END)
				} else {
					// we need to remove a character from the middle of the word

					// remove the current index from the list
					line = append(line[:index-1], line[index:]...)

					// save the current position of the cursor, as we have to move the cursor one back to erase the current symbol
					// and then move the cursor for each symbol in line[index-1:] to print it out, afterwards we want to restore
					// the cursor to its previous location.
					cursor.Save()

					// clear the rest of the line
					cursor.Back(1)

					// print what comes after
					for _, char := range line[index-1:] {
						//Erase symbols which are left over from older print
						EraseLine(rr.stdio.Out, ERASE_LINE_END)
						// print characters to the new line appropriately
						rr.printChar(char, mask)

					}
					// erase what's left over from last print
					if cursorCurrent.Y < terminalSize.Y {
						cursor.NextLine(1)
						EraseLine(rr.stdio.Out, ERASE_LINE_END)
					}
					// restore cursor
					cursor.Restore()
					if cursorCurrent.CursorIsAtLineBegin() {
						cursor.PreviousLine(1)
						cursor.Forward(int(terminalSize.X))
					} else {
						cursor.Back(1)
					}
				}

				// decrement the index
				index--
			} else {
				// otherwise the user pressed backspace while at the beginning of the line
				soundBell(rr.stdio.Out)
			}

			// we're done processing this key
			continue
		}

		// if the left arrow is pressed
		if r == KeyArrowLeft {
			// if we have space to the left
			if index > 0 {
				//move the cursor to the prev line if necessary
				if cursorCurrent.CursorIsAtLineBegin() {
					cursor.PreviousLine(1)
					cursor.Forward(int(terminalSize.X))
				} else {
					cursor.Back(1)
				}
				//decrement the index
				index--

			} else {
				// otherwise we are at the beginning of where we started reading lines
				// sound the bell
				soundBell(rr.stdio.Out)
			}

			// we're done processing this key press
			continue
		}

		// if the right arrow is pressed
		if r == KeyArrowRight {
			// if we have space to the right
			if index < len(line) {
				// move the cursor to the next line if necessary
				if cursorCurrent.CursorIsAtLineEnd(terminalSize) {
					cursor.NextLine(1)
				} else {
					cursor.Forward(1)
				}
				index++

			} else {
				// otherwise we are at the end of the word and can't go past
				// sound the bell
				soundBell(rr.stdio.Out)
			}

			// we're done processing this key press
			continue
		}
		// the user pressed one of the special keys
		if r == SpecialKeyHome {
			for index > 0 {
				if cursorCurrent.CursorIsAtLineBegin() {
					cursor.PreviousLine(1)
					cursor.Forward(int(terminalSize.X))
					cursorCurrent.X = terminalSize.X
					cursorCurrent.Y--

				} else {
					cursor.Back(1)
					cursorCurrent.X--
				}
				index--
			}
			continue
			// user pressed end
		} else if r == SpecialKeyEnd {
			for index != len(line) {
				if cursorCurrent.CursorIsAtLineEnd(terminalSize) {
					cursor.NextLine(1)
					cursorCurrent.X = COORDINATE_SYSTEM_BEGIN
					cursorCurrent.Y++

				} else {
					cursor.Forward(1)
					cursorCurrent.X++
				}
				index++
			}
			continue
			// user pressed forward delete key
		} else if r == SpecialKeyDelete {
			// if index at the end of the line nothing to delete
			if index != len(line) {
				// save the current position of the cursor, as we have to  erase the current symbol
				// and then move the cursor for each symbol in line[index:] to print it out, afterwards we want to restore
				// the cursor to its previous location.
				cursor.Save()
				// remove the symbol after the cursor
				line = append(line[:index], line[index+1:]...)
				// print the updated line
				for _, char := range line[index:] {
					EraseLine(rr.stdio.Out, ERASE_LINE_END)
					// print out the character
					rr.printChar(char, mask)
				}
				// erase what's left on last line
				if cursorCurrent.Y < terminalSize.Y {
					cursor.NextLine(1)
					EraseLine(rr.stdio.Out, ERASE_LINE_END)
				}
				// restore cursor
				cursor.Restore()
				if len(line) == 0 || index == len(line) {
					EraseLine(rr.stdio.Out, ERASE_LINE_END)
				}
			}
			continue
		}

		// if the letter is another escape sequence
		if unicode.IsControl(r) || r == IgnoreKey {
			// ignore it
			continue
		}

		// the user pressed a regular key

		// if we are at the end of the line
		if index == len(line) {
			// just append the character at the end of the line
			line = append(line, r)
			// save the location of the cursor
			index++
			// print out the character
			rr.printChar(r, mask)
		} else {
			// we are in the middle of the word so we need to insert the character the user pressed
			line = append(line[:index], append([]rune{r}, line[index:]...)...)
			// save the current position of the cursor, as we have to move the cursor back to erase the current symbol
			// and then move for each symbol in line[index:] to print it out, afterwards we want to restore
			// cursor's location to its previous one.
			cursor.Save()
			EraseLine(rr.stdio.Out, ERASE_LINE_END)
			// remove the symbol after the cursor
			// print the updated line
			for _, char := range line[index:] {
				EraseLine(rr.stdio.Out, ERASE_LINE_END)
				// print out the character
				rr.printChar(char, mask)
				cursorCurrent.X++
			}
			// if we are at the last line, we want to visually insert a new line and append to it.
			if cursorCurrent.CursorIsAtLineEnd(terminalSize) && cursorCurrent.Y == terminalSize.Y {
				// add a new line to the terminal
				fmt.Fprintln(rr.stdio.Out)
				// restore the position of the cursor horizontally
				cursor.Restore()
				// restore the position of the cursor vertically
				cursor.Up(1)
			} else {
				// restore cursor
				cursor.Restore()
			}
			// check if cursor needs to move to next line
			cursorCurrent, _ = cursor.Location(rr.Buffer())
			if cursorCurrent.CursorIsAtLineEnd(terminalSize) {
				cursor.NextLine(1)
			} else {
				cursor.Forward(1)
			}
			// increment the index
			index++

		}
	}
}
