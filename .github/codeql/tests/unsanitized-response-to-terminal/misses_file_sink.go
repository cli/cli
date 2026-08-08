package fixtures

import (
	"io"
	"net/http"
	"os"
)

// The body is copied to a file on disk, not a terminal. Must NOT be flagged.
func DownloadToFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
