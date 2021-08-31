package codespaces

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/go-liveshare"
)

// StartPortForwarding starts LiveShare port forwarding of traffic
// between the LiveShare client and the specified local port, or, if
// zero, a port chosen at random; the effective port number is
// returned.  Forwarding continues in the background until an error is
// encountered (including cancellation of the context). Therefore
// clients must cancel the context.
//
// The session name is used (along with the port) to generate
// names for streams, and may appear in error messages.
//
// TODO(adonovan): simplify API concurrency from API. Either:
// 1) return a stop function so that clients don't forget to stop forwarding.
// 2) avoid creating a goroutine and returning a channel. Use approach of
//    http.ListenAndServe, which runs until it encounters an error
//    (incl. cancellation). But this means we can't return the port.
//    Can we make the client responsible for supplying it?
// 3) return a PortForwarding object that encapsulates the port,
//    and has NewRemoteCommand as a method. It will need a Stop method,
//    and an Error method for querying whether the session has failed
//    asynchronously.
func StartPortForwarding(ctx context.Context, lsclient *liveshare.Client, sessionName string, localPort int) (int, <-chan error, error) {
	server, err := liveshare.NewServer(lsclient)
	if err != nil {
		return 0, nil, fmt.Errorf("new liveshare server: %v", err)
	}

	if localPort == 0 {
		localPort = rand.Intn(9999-2000) + 2000
		// TODO(adonovan): retry if port is taken?
	}

	// TODO(josebalius): This port won't always be 2222
	if err := server.StartSharing(ctx, sessionName, 2222); err != nil {
		return 0, nil, fmt.Errorf("sharing sshd port: %v", err)
	}

	tunnelClosed := make(chan error)
	go func() {
		// TODO(adonovan): simplify liveshare API to combine NewPortForwarder and Start
		// methods into a single ForwardPort call, like http.ListenAndServe.
		// (Start is a misnomer: it runs the complete session.)
		// Also document that it never returns a nil error.
		portForwarder := liveshare.NewPortForwarder(lsclient, server, localPort)
		if err := portForwarder.Start(ctx); err != nil {
			tunnelClosed <- fmt.Errorf("forwarding port: %v", err)
			return
		}
		tunnelClosed <- nil
	}()

	return localPort, tunnelClosed, nil
}

// Shell runs an interactive secure shell over an existing
// port-forwarding session. It runs until the shell is terminated
// (including by cancellation of the context).
func Shell(ctx context.Context, log logger, port int, destination string, usingCustomPort bool) error {
	cmd, connArgs := newSSHCommand(ctx, port, destination, "")

	if usingCustomPort {
		log.Println("Connection Details: ssh " + destination + " " + strings.Join(connArgs, " "))
	}

	return cmd.Run()
}

// NewRemoteCommand returns a partially populated exec.Cmd that will
// securely run a shell command on the remote machine.
func NewRemoteCommand(ctx context.Context, tunnelPort int, destination, command string) *exec.Cmd {
	cmd, _ := newSSHCommand(ctx, tunnelPort, destination, command)
	return cmd
}

// newSSHCommand populates an exec.Cmd to run a command (or if blank,
// an interactive shell) over ssh.
func newSSHCommand(ctx context.Context, port int, dst, command string) (*exec.Cmd, []string) {
	connArgs := []string{"-p", strconv.Itoa(port), "-o", "NoHostAuthenticationForLocalhost=yes"}
	// TODO(adonovan): eliminate X11 and X11Trust flags where unneeded.
	cmdArgs := append([]string{dst, "-X", "-Y", "-C"}, connArgs...) // X11, X11Trust, Compression

	// An empty command enables port forwarding but not execution.
	if command != "" {
		cmdArgs = append(cmdArgs, command)
	}

	cmd := exec.CommandContext(ctx, "ssh", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	return cmd, connArgs
}
