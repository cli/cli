package command

import (
	"io"
	"io/ioutil"
	"os"
)

//EnabledTestingModeforComment is to simulate keypress on comments for Testing
var EnabledTestingModeforComment bool = false

//TemporariFileforKeyPress is a tmpfile to simulate input text for Testing
var TemporariFileforKeyPress os.File

//DisableTestingModeforComment will disable simulate keypress
func DisableTestingModeforComment() {
	EnabledTestingModeforComment = false
	defer TemporariFileforKeyPress.Close()
}

//EnableTestingModeforComment will enable simulate keypress
func EnableTestingModeforComment() error {
	EnabledTestingModeforComment = true
	in, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}

	TemporariFileforKeyPress = *in

	return nil
}

//SimulateUserInput is to simulate keypress on comments for Testing
func SimulateUserInput(text string) error {
	_, err := io.WriteString(&TemporariFileforKeyPress, text)
	if err != nil {
		return (err)
	}

	_, err = TemporariFileforKeyPress.Seek(0, os.SEEK_SET)
	if err != nil {
		return (err)
	}
	return nil
}
