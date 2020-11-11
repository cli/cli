package zen

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/cli/cli/pkg/cmdutil"
	"github.com/spf13/cobra"
)

// NewCmdZen create a new command that calls https://api.github.com/zen
func NewCmdZen(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "zen",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := http.Get("https://api.github.com/zen")
			if err != nil {
				fmt.Println(err)
				return
			}

			if resp.StatusCode != http.StatusOK {
				fmt.Println(resp.Status)
			}

			defer resp.Body.Close()
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				fmt.Println(err)
				return
			}

			fmt.Fprint(f.IOStreams.Out, string(data))
		},
	}

	cmdutil.DisableAuthCheck(cmd)

	return cmd
}
