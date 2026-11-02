package identity

import (
	"os"
	"os/exec"

	"github.com/cli/cli/v2/internal/identity"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/safeexec"
)

func runSSH(f *cmdutil.Factory, args []string) error {
	s, err := identity.Open()
	if err != nil {
		return err
	}
	account, host := os.Getenv("GH_IDENTITY_ACCOUNT"), os.Getenv("GH_IDENTITY_HOST")
	command, err := s.SSHCommand("", account != "")
	if err != nil {
		return err
	}
	if account != "" {
		args, err = s.SSHArgumentsForAccount(args, "", host, account)
	} else {
		args, err = s.SSHArguments(args, "")
	}
	if err != nil {
		return err
	}
	ssh, err := safeexec.LookPath(command[0])
	if err != nil {
		return err
	}
	child := exec.Command(ssh, append(command[1:], args...)...)
	child.Stdin, child.Stdout, child.Stderr = f.IOStreams.In, f.IOStreams.Out, f.IOStreams.ErrOut
	return child.Run()
}
