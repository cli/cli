package browser

import (
	"bufio"
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
	exe := "open"
	var args []string
	switch goos {
	case "darwin":
		args = append(args, url)
	case "windows":
		exe = "cmd"
		r := strings.NewReplacer("&", "^&")
		args = append(args, "/c", "start", r.Replace(url))
	default:
		if inWsl() {
			exe = "wslview"
		} else {
			exe = "xdg-open"
		}
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

	args = append(args, url)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// Determine if gh is running inside of WSL2
func inWsl() bool {
	file, err := os.Open("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan() // Read single line
	if err := scanner.Err(); err != nil {
		return false
	}

	os := scanner.Text()
	return strings.Contains(os, "microsoft-WSL2")
}
