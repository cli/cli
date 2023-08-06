package codespace

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

type selectOptions struct {
	filePath string
	selector *CodespaceSelector
}

func newSelectCmd(app *App) *cobra.Command {
	var (
		opts selectOptions
	)

	selectCmd := &cobra.Command{
		Use:    "select",
		Short:  "Select a Codespace",
		Hidden: true,
		Args:   noArgsConstraint,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Select(cmd.Context(), opts)
		},
	}

	opts.selector = AddCodespaceSelector(selectCmd, app.apiClient)
	selectCmd.Flags().StringVarP(&opts.filePath, "file", "f", "", "Output file path")
	return selectCmd
}

// Hidden codespace `select` command allows to reuse existing codespace selection
// dialog by external GH CLI extensions. By default output selected codespace name
// into `stdout`. Pass `--file`(`-f`) flag along with a file path to output selected
// codespace name into a file instead.
//
// ## Examples
//
// With `stdout` output:
//
// ```shell
//
//	gh codespace select
//
// ```
//
// With `into-a-file` output:
//
// ```shell
//
//	gh codespace select --file /tmp/selected_codespace.txt
//
// ```
func (a *App) Select(ctx context.Context, opts selectOptions) (err error) {
	codespace, err := opts.selector.Select(ctx)
	if err != nil {
		return err
	}

	if opts.filePath != "" {
		f, err := os.Create(opts.filePath)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}

		defer safeClose(f, &err)

		_, err = f.WriteString(codespace.Name)
		if err != nil {
			return fmt.Errorf("failed to write codespace name to output file: %w", err)
		}

		return nil
	}

	fmt.Fprintln(a.io.Out, codespace.Name)

	return nil
}
