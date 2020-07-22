package create

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	IO *iostreams.IOStreams

	Description string
	Public      bool
	Filenames   []string

	HttpClient func() (*http.Client, error)
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := CreateOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
	}

	cmd := &cobra.Command{
		Use:   "create [<filename>... | -]",
		Short: "Create a new gist",
		Long: heredoc.Doc(`
			Create a new GitHub gist with given contents.

			Gists can be created from one or multiple files. Alternatively, pass "-" as
			file name to read from standard input.
			
			By default, gists are private; use '--public' to make publicly listed ones.
		`),
		Example: heredoc.Doc(`
			# publish file 'hello.py' as a public gist
			$ gh gist create --public hello.py
			
			# create a gist with a description
			$ gh gist create hello.py -d "my Hello-World program in Python"

			# create a gist containing several files
			$ gh gist create hello.py world.py cool.txt
			
			# read from standard input to create a gist
			$ gh gist create -
			
			# create a gist from output piped from another command
			$ cat cool.txt | gh gist create
		`),
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return nil
			}
			if opts.IO.IsStdinTTY() {
				return &cmdutil.FlagError{Err: errors.New("no filenames passed and nothing on STDIN")}
			}
			return nil
		},
		RunE: func(c *cobra.Command, args []string) error {
			// TODO redundant?
			opts.HttpClient = f.HttpClient

			opts.Filenames = args

			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "desc", "d", "", "A description for this gist")
	cmd.Flags().BoolVarP(&opts.Public, "public", "p", false, "List the gist publicly (default: private)")
	return cmd
}

func createRun(opts *CreateOptions) error {
	fileArgs := opts.Filenames
	if len(fileArgs) == 0 {
		fileArgs = []string{"-"}
	}

	files, err := processFiles(opts.IO.In, fileArgs)
	if err != nil {
		return fmt.Errorf("failed to collect files for posting: %w", err)
	}

	errOut := opts.IO.ErrOut
	fmt.Fprintf(errOut, "%s Creating gist...\n", utils.Gray("-"))

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	gist, err := apiCreate(httpClient, opts.Description, opts.Public, files)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) {
			if httpError.OAuthScopes != "" && !strings.Contains(httpError.OAuthScopes, "gist") {
				return fmt.Errorf("This command requires the 'gist' OAuth scope.\nPlease re-authenticate by doing `gh config set -h github.com oauth_token ''` and running the command again.")
			}
		}
		return fmt.Errorf("%s Failed to create gist: %w", utils.Red("X"), err)
	}

	fmt.Fprintf(errOut, "%s Created gist\n", utils.Green("✓"))

	fmt.Fprintln(opts.IO.Out, gist.HTMLURL)

	return nil
}

func processFiles(stdin io.ReadCloser, filenames []string) (map[string]string, error) {
	fs := map[string]string{}

	if len(filenames) == 0 {
		return nil, errors.New("no files passed")
	}

	for i, f := range filenames {
		var filename string
		var content []byte
		var err error
		if f == "-" {
			filename = fmt.Sprintf("gistfile%d.txt", i)
			content, err = ioutil.ReadAll(stdin)
			if err != nil {
				return fs, fmt.Errorf("failed to read from stdin: %w", err)
			}
			stdin.Close()
		} else {
			content, err = ioutil.ReadFile(f)
			if err != nil {
				return fs, fmt.Errorf("failed to read file %s: %w", f, err)
			}
			filename = path.Base(f)
		}

		fs[filename] = string(content)
	}

	return fs, nil
}
