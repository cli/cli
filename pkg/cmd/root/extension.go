package root

import (
	"fmt"

	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

func NewCmdExtension(io *iostreams.IOStreams, em extensions.ExtensionManager, ext extensions.Extension) *cobra.Command {
	var short string
	if ext.IsLocal() {
		short = fmt.Sprintf("Local extension gh-%s", ext.Name())
	} else {
		path := ext.URL()
		if u, err := git.ParseURL(ext.URL()); err == nil {
			if r, err := ghrepo.FromURL(u); err == nil {
				path = ghrepo.FullName(r)
			}
		}
		short = fmt.Sprintf("Extension %s", path)
	}
	return &cobra.Command{
		Use:   ext.Name(),
		Short: short,
		RunE: func(c *cobra.Command, args []string) error {
			args = append([]string{ext.Name()}, args...)
			if _, err := em.Dispatch(args, io.In, io.Out, io.ErrOut); err != nil {
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
