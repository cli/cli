package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/gist/shared"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/utils"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	IO *iostreams.IOStreams

	Description      string
	Public           bool
	Filenames        []string
	FilenameOverride string
	WebMode          bool

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
			opts.Filenames = args

			if runF != nil {
				return runF(&opts)
			}
			return createRun(&opts)
		},
	}

	cmd.Flags().StringVarP(&opts.Description, "desc", "d", "", "A description for this gist")
	cmd.Flags().BoolVarP(&opts.WebMode, "web", "w", false, "Open the web browser with created gist")
	cmd.Flags().BoolVarP(&opts.Public, "public", "p", false, "List the gist publicly (default: private)")
	cmd.Flags().StringVarP(&opts.FilenameOverride, "filename", "f", "", "Provide a filename to be used when reading from STDIN")
	return cmd
}

func createRun(opts *CreateOptions) error {
	fileArgs := opts.Filenames
	if len(fileArgs) == 0 {
		fileArgs = []string{"-"}
	}

	files, err := processFiles(opts.IO.In, opts.FilenameOverride, fileArgs)
	if err != nil {
		return fmt.Errorf("failed to collect files for posting: %w", err)
	}

	gistName := guessGistName(files)

	processMessage := "Creating gist..."
	completionMessage := "Created gist"
	if gistName != "" {
		if len(files) > 1 {
			processMessage = "Creating gist with multiple files"
		} else {
			processMessage = fmt.Sprintf("Creating gist %s", gistName)
		}
		completionMessage = fmt.Sprintf("Created gist %s", gistName)
	}

	cs := opts.IO.ColorScheme()

	errOut := opts.IO.ErrOut
	fmt.Fprintf(errOut, "%s %s\n", cs.Gray("-"), processMessage)

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	gist, err := createGist(httpClient, ghinstance.OverridableDefault(), opts.Description, opts.Public, files)
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) {
			if httpError.OAuthScopes != "" && !strings.Contains(httpError.OAuthScopes, "gist") {
				return fmt.Errorf("This command requires the 'gist' OAuth scope.\nPlease re-authenticate by doing `gh config set -h github.com oauth_token ''` and running the command again.")
			}
		}
		return fmt.Errorf("%s Failed to create gist: %w", cs.Red("X"), err)
	}

	fmt.Fprintf(errOut, "%s %s\n", cs.SuccessIconWithColor(cs.Green), completionMessage)

	if opts.WebMode {
		fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", utils.DisplayURL(gist.HTMLURL))

		return utils.OpenInBrowser(gist.HTMLURL)
	}

	fmt.Fprintln(opts.IO.Out, gist.HTMLURL)

	return nil
}

func processFiles(stdin io.ReadCloser, filenameOverride string, filenames []string) (map[string]*shared.GistFile, error) {
	fs := map[string]*shared.GistFile{}

	if len(filenames) == 0 {
		return nil, errors.New("no files passed")
	}

	for i, f := range filenames {
		var filename string
		var content []byte
		var err error

		if f == "-" {
			if filenameOverride != "" {
				filename = filenameOverride
			} else {
				filename = fmt.Sprintf("gistfile%d.txt", i)
			}
			content, err = ioutil.ReadAll(stdin)
			if err != nil {
				return fs, fmt.Errorf("failed to read from stdin: %w", err)
			}
			stdin.Close()

			if shared.IsBinaryContents(content) {
				return nil, fmt.Errorf("binary file contents not supported")
			}
		} else {
			isBinary, err := shared.IsBinaryFile(f)
			if err != nil {
				return fs, fmt.Errorf("failed to read file %s: %w", f, err)
			}
			if isBinary {
				return nil, fmt.Errorf("failed to upload %s: binary file not supported", f)
			}

			content, err = ioutil.ReadFile(f)
			if err != nil {
				return fs, fmt.Errorf("failed to read file %s: %w", f, err)
			}

			filename = path.Base(f)
		}

		fs[filename] = &shared.GistFile{
			Content: string(content),
		}
	}

	return fs, nil
}

func guessGistName(files map[string]*shared.GistFile) string {
	filenames := make([]string, 0, len(files))
	gistName := ""

	re := regexp.MustCompile(`^gistfile\d+\.txt$`)
	for k := range files {
		if !re.MatchString(k) {
			filenames = append(filenames, k)
		}
	}

	if len(filenames) > 0 {
		sort.Strings(filenames)
		gistName = filenames[0]
	}

	return gistName
}

func createGist(client *http.Client, hostname, description string, public bool, files map[string]*shared.GistFile) (*shared.Gist, error) {
	path := "gists"

	body := &shared.Gist{
		Description: description,
		Public:      public,
		Files:       files,
	}

	result := shared.Gist{}

	requestByte, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	requestBody := bytes.NewReader(requestByte)

	apiClient := api.NewClientFromHTTP(client)
	err = apiClient.REST(hostname, "POST", path, requestBody, &result)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
