//go:build !windows
// +build !windows

package findsh

import "os/exec"

// Find locates the `sh` interpreter on the system.
func Find() (string, error) {
	return exec.LookPath("sh")
}
