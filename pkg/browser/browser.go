package browser

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
	"github.com/google/shlex"
)

// BrowserEnv simply returns the $BROWSER environment variable
func FromEnv() string {
	return os.Getenv("BROWSER")
}

// Command produces an exec.Cmd respecting runtime.GOOS and $BROWSER environment variable
func Command(url string) (*exec.Cmd, error) {
	launcher := FromEnv()
	if launcher != "" {
		return FromLauncher(launcher, url)
	}
	return ForOS(runtime.GOOS, url), nil
}

// ForOS produces an exec.Cmd to open the web browser for different OS
func ForOS(goos, url string) *exec.Cmd {
	exe := "open"
	var args []string
	switch goos {
	case "darwin":
		args = append(args, url)
	case "windows":
		exe, _ = lookPath("cmd")
		r := strings.NewReplacer("&", "^&")
		args = append(args, "/c", "start", r.Replace(url))
	default:
		exe = linuxExe()
		args = append(args, url)
	}

	cmd := exec.Command(exe, args...)
	cmd.Stderr = os.Stderr
	return cmd
}

// FromLauncher parses the launcher string based on shell splitting rules
func FromLauncher(launcher, url string) (*exec.Cmd, error) {
	args, err := shlex.Split(launcher)
	if err != nil {
		return nil, err
	}

	exe, err := lookPath(args[0])
	if err != nil {
		return nil, err
	}

	args = append(args, url)
	cmd := exec.Command(exe, args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd, nil
}

func linuxExe() string {
	exe := "xdg-open"

	_, err := lookPath(exe)
	if err != nil {
		_, err := lookPath("wslview")
		if err == nil {
			exe = "wslview"
		}
	}

	return exe
}

var lookPath = safeexec.LookPath
