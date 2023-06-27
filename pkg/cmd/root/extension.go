package root

import (
	"errors"
	"fmt"
	"os/exec"

	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ExternalCommandExitError struct {
	*exec.ExitError
}

func NewCmdExtension(io *iostreams.IOStreams, em extensions.ExtensionManager, ext extensions.Extension) *cobra.Command {
	return &cobra.Command{
		Use:   ext.Name(),
		Short: fmt.Sprintf("Extension %s", ext.Name()),
		RunE: func(c *cobra.Command, args []string) error {
			args = append([]string{ext.Name()}, args...)
			if _, err := em.Dispatch(args, io.In, io.Out, io.ErrOut); err != nil {
				var execError *exec.ExitError
				if errors.As(err, &execError) {
					return &ExternalCommandExitError{execError}
				}
				return fmt.Errorf("failed to run extension: %w\n", err)
			}
			return nil
		},
		GroupID: "extension",
		Annotations: map[string]string{
			"skipAuthCheck": "true",
		},
		DisableFlagParsing: true,
	}
}
