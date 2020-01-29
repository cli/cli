package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/google/shlex"
)

// Command produces an exec.Cmd respecting runtime.GOOS and $BROWSER environment variable
func Command(url string) (*exec.Cmd, error) {
	launcher := os.Getenv("BROWSER")
	if launcher != "" {
		return FromLauncher(launcher, url)
	}
	return ForOS(runtime.GOOS, url), nil
}

// ForOS produces an exec.Cmd to open the web browser for different OS
func ForOS(goos, url string) *exec.Cmd {
	var args []string
	switch goos {
	case "darwin":
		args = []string{"open"}
	case "windows":
		args = []string{"cmd", "/c", "start"}
		r := strings.NewReplacer("&", "^&")
		url = r.Replace(url)
	default:
		args = []string{"xdg-open"}
	}

	args = append(args, url)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd
}

// FromLauncher parses the launcher string based on shell splitting rules
func FromLauncher(launcher, url string) (*exec.Cmd, error) {
	args, err := shlex.Split(launcher)
	if err != nil {
		return nil, err
	}

	args = append(args, url)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd, nil
}
