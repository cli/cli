package main

import "os"

// writeFile creates a file at the given filename and writes the provided bytes
// to it. Errors if the file already exists.
func writeFile(filename string, bytes []byte) error {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(bytes)
	return err
}
