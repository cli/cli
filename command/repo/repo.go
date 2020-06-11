package repo

import (
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/cli/cli/command"
	"github.com/cli/cli/git"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/internal/run"
	"github.com/spf13/cobra"
)

var repoCmd = &cobra.Command{
	Use:   "repo <command>",
	Short: "Create, clone, fork, and view repositories",
	Long:  `Work with GitHub repositories`,
	Example: `$ gh repo create
$ gh repo clone cli/cli 
$ gh repo view --web
`,
	Annotations: map[string]string{
		"IsCore": "true",
		"help:arguments": `
A repository can be supplied as an argument in any of the following formats:
- "OWNER/REPO"
- by URL, e.g. "https://github.com/OWNER/REPO"`},
}

func init() {
	// root repo cmd here ...
}

func parseCloneArgs(extraArgs []string) (args []string, target string) {
	args = extraArgs

	if len(args) > 0 {
		if !strings.HasPrefix(args[0], "-") {
			target, args = args[0], args[1:]
		}
	}
	return
}

func addUpstreamRemote(cmd *cobra.Command, parentRepo ghrepo.Interface, cloneDir string) error {
	upstreamURL := command.FormatRemoteURL(cmd, ghrepo.FullName(parentRepo))
	cloneCmd := git.GitCommand("-C", cloneDir, "remote", "add", "-f", "upstream", upstreamURL)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr

	return run.PrepareCmd(cloneCmd).Run()
}

func isURL(arg string) bool {
	return strings.HasPrefix(arg, "http:/") || strings.HasPrefix(arg, "https:/")
}

var Since = func(t time.Time) time.Duration {
	return time.Since(t)
}

var Confirm = func(prompt string, result *bool) error {
	p := &survey.Confirm{
		Message: prompt,
		Default: true,
	}
	return survey.AskOne(p, result)
}
