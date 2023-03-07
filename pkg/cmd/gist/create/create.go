package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/browser"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/text"
	"github.com/cli/cli/v2/pkg/cmd/gist/shared"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/spf13/cobra"
)

type CreateOptions struct {
	IO *iostreams.IOStreams

	Description      string
	Public           bool
	Filenames        []string
	FilenameOverride string
	WebMode          bool

	Config     func() (config.Config, error)
	HttpClient func() (*http.Client, error)
	Browser    browser.Browser
}

func NewCmdCreate(f *cmdutil.Factory, runF func(*CreateOptions) error) *cobra.Command {
	opts := CreateOptions{
		IO:         f.IOStreams,
		Config:     f.Config,
		HttpClient: f.HttpClient,
		Browser:    f.Browser,
	}

	cmd := &cobra.Command{
		Use:   "create [<filename>... | -]",
		Short: "Create a new gist",
		Long: heredoc.Doc(`
			Create a new GitHub gist with given contents.

			Gists can be created from one or multiple files. Alternatively, pass "-" as
			file name to read from standard input.

			By default, gists are secret; use '--public' to make publicly listed ones.
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
				return cmdutil.FlagErrorf("no filenames passed and nothing on STDIN")
			}
			return nil
		},
		Aliases: []string{"new"},
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
	cmd.Flags().BoolVarP(&opts.Public, "public", "p", false, "List the gist publicly (default: secret)")
	cmd.Flags().StringVarP(&opts.FilenameOverride, "filename", "f", "", "Provide a filename to be used when reading from standard input")
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

	cs := opts.IO.ColorScheme()
	gistName := guessGistName(files)

	processMessage := "Creating gist..."
	completionMessage := "Created gist"
	if gistName != "" {
		if len(files) > 1 {
			processMessage = "Creating gist with multiple files"
		} else {
			processMessage = fmt.Sprintf("Creating gist %s", gistName)
		}
		if opts.Public {
			completionMessage = fmt.Sprintf("Created %s gist %s", cs.Red("public"), gistName)
		} else {
			completionMessage = fmt.Sprintf("Created %s gist %s", cs.Green("secret"), gistName)
		}
	}

	errOut := opts.IO.ErrOut
	fmt.Fprintf(errOut, "%s %s\n", cs.Gray("-"), processMessage)

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	opts.IO.StartProgressIndicator()
	gist, err := createGist(httpClient, host, opts.Description, opts.Public, files)
	opts.IO.StopProgressIndicator()
	if err != nil {
		var httpError api.HTTPError
		if errors.As(err, &httpError) {
			if httpError.StatusCode == http.StatusUnprocessableEntity {
				if detectEmptyFiles(files) {
					fmt.Fprintf(errOut, "%s Failed to create gist: %s\n", cs.FailureIcon(), "a gist file cannot be blank")
					return cmdutil.SilentError
				}
			}
		}
		return fmt.Errorf("%s Failed to create gist: %w", cs.Red("X"), err)
	}

	fmt.Fprintf(errOut, "%s %s\n", cs.SuccessIconWithColor(cs.Green), completionMessage)

	if opts.WebMode {
		fmt.Fprintf(opts.IO.Out, "Opening %s in your browser.\n", text.DisplayURL(gist.HTMLURL))

		return opts.Browser.Browse(gist.HTMLURL)
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
			content, err = io.ReadAll(stdin)
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

			content, err = os.ReadFile(f)
			if err != nil {
				return fs, fmt.Errorf("failed to read file %s: %w", f, err)
			}

			filename = filepath.Base(f)
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
	body := &shared.Gist{
		Description: description,
		Public:      public,
		Files:       files,
	}

	requestBody := &bytes.Buffer{}
	enc := json.NewEncoder(requestBody)
	if err := enc.Encode(body); err != nil {
		return nil, err
	}

	u := ghinstance.RESTPrefix(hostname) + "gists"
	req, err := http.NewRequest(http.MethodPost, u, requestBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return nil, api.HandleHTTPError(api.EndpointNeedsScopes(resp, "gist"))
	}

	result := &shared.Gist{}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(result); err != nil {
		return nil, err
	}

	return result, nil
}

func detectEmptyFiles(files map[string]*shared.GistFile) bool {
	for _, file := range files {
		if strings.TrimSpace(file.Content) == "" {
			return true
		}
	}
	return false
}
