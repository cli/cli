package root

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/cli/cli/v2/internal/update"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/utils"
	"github.com/spf13/cobra"
)

type ExternalCommandExitError struct {
	*exec.ExitError
}

func NewCmdExtension(io *iostreams.IOStreams, em extensions.ExtensionManager, ext extensions.Extension, checkExtensionReleaseInfo func(extensions.ExtensionManager, extensions.Extension) (*update.ReleaseInfo, error)) *cobra.Command {
	updateMessageChan := make(chan *update.ReleaseInfo)
	cs := io.ColorScheme()
	hasDebug, _ := utils.IsDebugEnabled()
	if checkExtensionReleaseInfo == nil {
		checkExtensionReleaseInfo = checkForExtensionUpdate
	}

	return &cobra.Command{
		Use:   ext.Name(),
		Short: fmt.Sprintf("Extension %s", ext.Name()),
		// PreRun handles looking up whether extension has a latest version only when the command is ran.
		PreRun: func(c *cobra.Command, args []string) {
			go func() {
				releaseInfo, err := checkExtensionReleaseInfo(em, ext)
				if err != nil && hasDebug {
					fmt.Fprintf(io.ErrOut, "warning: checking for update failed: %v", err)
				}
				updateMessageChan <- releaseInfo
			}()
		},
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
		// PostRun handles communicating extension release information if found
		PostRun: func(c *cobra.Command, args []string) {
			select {
			case releaseInfo := <-updateMessageChan:
				if releaseInfo != nil {
					stderr := io.ErrOut
					fmt.Fprintf(stderr, "\n\n%s %s → %s\n",
						cs.Yellowf("A new release of %s is available:", ext.Name()),
						cs.Cyan(strings.TrimPrefix(ext.CurrentVersion(), "v")),
						cs.Cyan(strings.TrimPrefix(releaseInfo.Version, "v")))
					if ext.IsPinned() {
						fmt.Fprintf(stderr, "To upgrade, run: gh extension upgrade %s --force\n", ext.Name())
					} else {
						fmt.Fprintf(stderr, "To upgrade, run: gh extension upgrade %s\n", ext.Name())
					}
					fmt.Fprintf(stderr, "%s\n\n",
						cs.Yellow(releaseInfo.URL))
				}
			default:
				// Do not make the user wait for extension update check if incomplete by this time.
				// This is being handled in non-blocking default as there is no context to cancel like in gh update checks.
			}
		},
		GroupID: "extension",
		Annotations: map[string]string{
			"skipAuthCheck": "true",
		},
		DisableFlagParsing: true,
	}
}

func checkForExtensionUpdate(em extensions.ExtensionManager, ext extensions.Extension) (*update.ReleaseInfo, error) {
	if !update.ShouldCheckForExtensionUpdate() {
		return nil, nil
	}

	return update.CheckForExtensionUpdate(em, ext, time.Now())
}
