package test

import "os"

func SuppressAndRestoreOutput() func() {
	null, _ := os.Open(os.DevNull)
	stdOut := os.Stdout
	stdErr := os.Stderr
	os.Stdout = null
	os.Stderr = null
	return func() {
		defer null.Close()
		os.Stdout = stdOut
		os.Stderr = stdErr
	}
}
