package command

import (
	"fmt"
	"github.com/cli/cli/api"
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(gistCmd)
	gistCmd.AddCommand(gistCreateCmd)
	gistCreateCmd.Flags().StringP("description", "d", "", "Required. The filenames and content of each file in the gist. The keys in the files object represent the filename and have the type string.")
	gistCreateCmd.Flags().StringP("filename", "f", "", "A descriptive name for this gist.")
	gistCreateCmd.Flags().BoolP("public", "p", false, "When true, the gist will be public and available for anyone to see. Default: false.")
}

var gistCmd = &cobra.Command{
	Use:   "gist",
	Short: "Create gists",
	Long: `Work with GitHub gists.
	`,
}

var gistCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new gist",
	RunE:  gistCreate,
}

func gistCreate(cmd *cobra.Command, args []string) error {
	ctx := contextForCommand(cmd)
	client, err := apiClientForContext(ctx)
	if err != nil {
		return err
	}

	description, err := cmd.Flags().GetString("description")
	if err != nil {
		return err
	}

	public, err := cmd.Flags().GetBool("public")
	if err != nil {
		return err
	}

	filename, err := cmd.Flags().GetString("filename")
	if err != nil {
		return err
	}

	if filename == "" {
		fmt.Fprintf(colorableErr(cmd), "\n<filename> is required\n\n")
		return nil
	}

	user, err := ctx.AuthLogin()
	if err != nil {
		return err
	}

	fmt.Fprintf(colorableErr(cmd), "\nCreating a Gist for %s\n\n", user)

	gist, err := api.GistCreate(client, description, public, filename)
	if err != nil {
		return fmt.Errorf("Failed to create a gist: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), gist.HTMLURL)

	return nil
}
