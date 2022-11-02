package browse

import (
	"fmt"
	"io"
	"log"
	"testing"

	"github.com/rivo/tview"
)

/*

What can be usefully unit tested?

- the extList can be pretty tested.
- i could manufacture key presses and send them into the SetInputCapture function to test that behavior if I'm feeling wild
- readmegetter is easy to test -- and honestly could be reduced to a simple function at this point since it doesn't do in-memory caching anymore.
- install/remove if I factor it out of SetInputCapture


What is not easily testable?

- loadSelectedReadme because of its use of QueueUpdateDraw, but this might be testable if I just replace the function on the app struct?

*/

func Test_extList(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	list := tview.NewList()
	extEntries := []extEntry{} // TODO
	app := tview.NewApplication()

	extList := newExtList(app, list, extEntries, logger)

	fmt.Printf("DBG %#v\n", extList)

	// TODO
}
