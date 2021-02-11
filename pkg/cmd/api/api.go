package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/internal/ghrepo"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/cli/cli/pkg/jsoncolor"
	"github.com/spf13/cobra"
)

type ApiOptions struct {
	IO *iostreams.IOStreams

	Hostname            string
	RequestMethod       string
	RequestMethodPassed bool
	RequestPath         string
	RequestInputFile    string
	MagicFields         []string
	RawFields           []string
	RequestHeaders      []string
	ShowResponseHeaders bool
	Paginate            bool
	Silent              bool

	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
}

func NewCmdApi(f *cmdutil.Factory, runF func(*ApiOptions) error) *cobra.Command {
	opts := ApiOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated GitHub API request",
		Long: heredoc.Docf(`
			Makes an authenticated HTTP request to the GitHub API and prints the response.

			The endpoint argument should either be a path of a GitHub API v3 endpoint, or
			"graphql" to access the GitHub API v4.

			Placeholder values ":owner", ":repo", and ":branch" in the endpoint argument will
			get replaced with values from the repository of the current directory.

			The default HTTP request method is "GET" normally and "POST" if any parameters
			were added. Override the method with %[1]s--method%[1]s.

			Pass one or more %[1]s--raw-field%[1]s values in "key=value" format to add
			JSON-encoded string parameters to the POST body.

			The %[1]s--field%[1]s flag behaves like %[1]s--raw-field%[1]s with magic type conversion based
			on the format of the value:

			- literal values "true", "false", "null", and integer numbers get converted to
			  appropriate JSON types;
			- placeholder values ":owner", ":repo", and ":branch" get populated with values
			  from the repository of the current directory;
			- if the value starts with "@", the rest of the value is interpreted as a
			  filename to read the value from. Pass "-" to read from standard input.

			For GraphQL requests, all fields other than "query" and "operationName" are
			interpreted as GraphQL variables.

			Raw request body may be passed from the outside via a file specified by %[1]s--input%[1]s.
			Pass "-" to read from standard input. In this mode, parameters specified via
			%[1]s--field%[1]s flags are serialized into URL query parameters.

			In %[1]s--paginate%[1]s mode, all pages of results will sequentially be requested until
			there are no more pages of results. For GraphQL requests, this requires that the
			original query accepts an %[1]s$endCursor: String%[1]s variable and that it fetches the
			%[1]spageInfo{ hasNextPage, endCursor }%[1]s set of fields from a collection.
		`, "`"),
		Example: heredoc.Doc(`
			# list releases in the current repository
			$ gh api repos/:owner/:repo/releases

			# post an issue comment
			$ gh api repos/:owner/:repo/issues/123/comments -f body='Hi from CLI'

			# add parameters to a GET request
			$ gh api -X GET search/issues -f q='repo:cli/cli is:open remote'

			# set a custom HTTP header
			$ gh api -H 'Accept: application/vnd.github.XYZ-preview+json' ...

			# list releases with GraphQL
			$ gh api graphql -F owner=':owner' -F name=':repo' -f query='
			  query($name: String!, $owner: String!) {
			    repository(owner: $owner, name: $name) {
			      releases(last: 3) {
			        nodes { tagName }
			      }
			    }
			  }
			'

			# list all repositories for a user
			$ gh api graphql --paginate -f query='
			  query($endCursor: String) {
			    viewer {
			      repositories(first: 100, after: $endCursor) {
			        nodes { nameWithOwner }
			        pageInfo {
			          hasNextPage
			          endCursor
			        }
			      }
			    }
			  }
			'
		`),
		Annotations: map[string]string{
			"help:environment": heredoc.Doc(`
				GH_TOKEN, GITHUB_TOKEN (in order of precedence): an authentication token for
				github.com API requests.

				GH_ENTERPRISE_TOKEN, GITHUB_ENTERPRISE_TOKEN (in order of precedence): an
				authentication token for API requests to GitHub Enterprise.

				GH_HOST: make the request to a GitHub host other than github.com.
			`),
		},
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.RequestMethodPassed = c.Flags().Changed("method")

			if c.Flags().Changed("hostname") {
				if err := ghinstance.HostnameValidator(opts.Hostname); err != nil {
					return &cmdutil.FlagError{Err: fmt.Errorf("error parsing --hostname: %w", err)}
				}
			}

			if opts.Paginate && !strings.EqualFold(opts.RequestMethod, "GET") && opts.RequestPath != "graphql" {
				return &cmdutil.FlagError{Err: errors.New(`the '--paginate' option is not supported for non-GET requests`)}
			}
			if opts.Paginate && opts.RequestInputFile != "" {
				return &cmdutil.FlagError{Err: errors.New(`the '--paginate' option is not supported with '--input'`)}
			}

			if runF != nil {
				return runF(&opts)
			}
			return apiRun(&opts)
		},
	}

	cmd.Flags().StringVar(&opts.Hostname, "hostname", "", "The GitHub hostname for the request (default \"github.com\")")
	cmd.Flags().StringVarP(&opts.RequestMethod, "method", "X", "GET", "The HTTP method for the request")
	cmd.Flags().StringArrayVarP(&opts.MagicFields, "field", "F", nil, "Add a typed parameter in `key=value` format")
	cmd.Flags().StringArrayVarP(&opts.RawFields, "raw-field", "f", nil, "Add a string parameter in `key=value` format")
	cmd.Flags().StringArrayVarP(&opts.RequestHeaders, "header", "H", nil, "Add a HTTP request header in `key:value` format")
	cmd.Flags().BoolVarP(&opts.ShowResponseHeaders, "include", "i", false, "Include HTTP response headers in the output")
	cmd.Flags().BoolVar(&opts.Paginate, "paginate", false, "Make additional HTTP requests to fetch all pages of results")
	cmd.Flags().StringVar(&opts.RequestInputFile, "input", "", "The `file` to use as body for the HTTP request")
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	return cmd
}

func apiRun(opts *ApiOptions) error {
	params, err := parseFields(opts)
	if err != nil {
		return err
	}

	isGraphQL := opts.RequestPath == "graphql"
	requestPath, err := fillPlaceholders(opts.RequestPath, opts)
	if err != nil {
		return fmt.Errorf("unable to expand placeholder in path: %w", err)
	}
	method := opts.RequestMethod
	requestHeaders := opts.RequestHeaders
	var requestBody interface{} = params

	if !opts.RequestMethodPassed && (len(params) > 0 || opts.RequestInputFile != "") {
		method = "POST"
	}

	if opts.Paginate && !isGraphQL {
		requestPath = addPerPage(requestPath, 100, params)
	}

	if opts.RequestInputFile != "" {
		file, size, err := openUserFile(opts.RequestInputFile, opts.IO.In)
		if err != nil {
			return err
		}
		defer file.Close()
		requestPath = addQuery(requestPath, params)
		requestBody = file
		if size >= 0 {
			requestHeaders = append([]string{fmt.Sprintf("Content-Length: %d", size)}, requestHeaders...)
		}
	}

	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	headersOutputStream := opts.IO.Out
	if opts.Silent {
		opts.IO.Out = ioutil.Discard
	} else {
		err := opts.IO.StartPager()
		if err != nil {
			return err
		}
		defer opts.IO.StopPager()
	}

	host := ghinstance.OverridableDefault()
	if opts.Hostname != "" {
		host = opts.Hostname
	}

	hasNextPage := true
	for hasNextPage {
		resp, err := httpRequest(httpClient, host, method, requestPath, requestBody, requestHeaders)
		if err != nil {
			return err
		}

		endCursor, err := processResponse(resp, opts, headersOutputStream)
		if err != nil {
			return err
		}

		if !opts.Paginate {
			break
		}

		if isGraphQL {
			hasNextPage = endCursor != ""
			if hasNextPage {
				params["endCursor"] = endCursor
			}
		} else {
			requestPath, hasNextPage = findNextPage(resp)
		}

		if hasNextPage && opts.ShowResponseHeaders {
			fmt.Fprint(opts.IO.Out, "\n")
		}
	}

	return nil
}

func processResponse(resp *http.Response, opts *ApiOptions, headersOutputStream io.Writer) (endCursor string, err error) {
	if opts.ShowResponseHeaders {
		fmt.Fprintln(headersOutputStream, resp.Proto, resp.Status)
		printHeaders(headersOutputStream, resp.Header, opts.IO.ColorEnabled())
		fmt.Fprint(headersOutputStream, "\r\n")
	}

	if resp.StatusCode == 204 {
		return
	}
	var responseBody io.Reader = resp.Body
	defer resp.Body.Close()

	isJSON, _ := regexp.MatchString(`[/+]json(;|$)`, resp.Header.Get("Content-Type"))

	var serverError string
	if isJSON && (opts.RequestPath == "graphql" || resp.StatusCode >= 400) {
		responseBody, serverError, err = parseErrorResponse(responseBody, resp.StatusCode)
		if err != nil {
			return
		}
	}

	var bodyCopy *bytes.Buffer
	isGraphQLPaginate := isJSON && resp.StatusCode == 200 && opts.Paginate && opts.RequestPath == "graphql"
	if isGraphQLPaginate {
		bodyCopy = &bytes.Buffer{}
		responseBody = io.TeeReader(responseBody, bodyCopy)
	}

	if isJSON && opts.IO.ColorEnabled() {
		err = jsoncolor.Write(opts.IO.Out, responseBody, "  ")
	} else {
		_, err = io.Copy(opts.IO.Out, responseBody)
	}
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			err = nil
		} else {
			return
		}
	}

	if serverError != "" {
		fmt.Fprintf(opts.IO.ErrOut, "gh: %s\n", serverError)
		err = cmdutil.SilentError
		return
	} else if resp.StatusCode > 299 {
		fmt.Fprintf(opts.IO.ErrOut, "gh: HTTP %d\n", resp.StatusCode)
		err = cmdutil.SilentError
		return
	}

	if isGraphQLPaginate {
		endCursor = findEndCursor(bodyCopy)
	}

	return
}

var placeholderRE = regexp.MustCompile(`\:(owner|repo|branch)\b`)

// fillPlaceholders populates `:owner` and `:repo` placeholders with values from the current repository
func fillPlaceholders(value string, opts *ApiOptions) (string, error) {
	if !placeholderRE.MatchString(value) {
		return value, nil
	}

	baseRepo, err := opts.BaseRepo()
	if err != nil {
		return value, err
	}

	filled := placeholderRE.ReplaceAllStringFunc(value, func(m string) string {
		switch m {
		case ":owner":
			return baseRepo.RepoOwner()
		case ":repo":
			return baseRepo.RepoName()
		case ":branch":
			branch, e := opts.Branch()
			if e != nil {
				err = e
			}
			return branch
		default:
			panic(fmt.Sprintf("invalid placeholder: %q", m))
		}
	})

	if err != nil {
		return value, err
	}

	return filled, nil
}

func printHeaders(w io.Writer, headers http.Header, colorize bool) {
	var names []string
	for name := range headers {
		if name == "Status" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var headerColor, headerColorReset string
	if colorize {
		headerColor = "\x1b[1;34m" // bright blue
		headerColorReset = "\x1b[m"
	}
	for _, name := range names {
		fmt.Fprintf(w, "%s%s%s: %s\r\n", headerColor, name, headerColorReset, strings.Join(headers[name], ", "))
	}
}

func parseFields(opts *ApiOptions) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	for _, f := range opts.RawFields {
		key, value, err := parseField(f)
		if err != nil {
			return params, err
		}
		params[key] = value
	}
	for _, f := range opts.MagicFields {
		key, strValue, err := parseField(f)
		if err != nil {
			return params, err
		}
		value, err := magicFieldValue(strValue, opts)
		if err != nil {
			return params, fmt.Errorf("error parsing %q value: %w", key, err)
		}
		params[key] = value
	}
	return params, nil
}

func parseField(f string) (string, string, error) {
	idx := strings.IndexRune(f, '=')
	if idx == -1 {
		return f, "", fmt.Errorf("field %q requires a value separated by an '=' sign", f)
	}
	return f[0:idx], f[idx+1:], nil
}

func magicFieldValue(v string, opts *ApiOptions) (interface{}, error) {
	if strings.HasPrefix(v, "@") {
		return opts.IO.ReadUserFile(v[1:])
	}

	if n, err := strconv.Atoi(v); err == nil {
		return n, nil
	}

	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return fillPlaceholders(v, opts)
	}
}

func openUserFile(fn string, stdin io.ReadCloser) (io.ReadCloser, int64, error) {
	if fn == "-" {
		return stdin, -1, nil
	}

	r, err := os.Open(fn)
	if err != nil {
		return r, -1, err
	}

	s, err := os.Stat(fn)
	if err != nil {
		return r, -1, err
	}

	return r, s.Size(), nil
}

func parseErrorResponse(r io.Reader, statusCode int) (io.Reader, string, error) {
	bodyCopy := &bytes.Buffer{}
	b, err := ioutil.ReadAll(io.TeeReader(r, bodyCopy))
	if err != nil {
		return r, "", err
	}

	var parsedBody struct {
		Message string
		Errors  []json.RawMessage
	}
	err = json.Unmarshal(b, &parsedBody)
	if err != nil {
		return r, "", err
	}
	if parsedBody.Message != "" {
		return bodyCopy, fmt.Sprintf("%s (HTTP %d)", parsedBody.Message, statusCode), nil
	}

	type errorMessage struct {
		Message string
	}
	var errors []string
	for _, rawErr := range parsedBody.Errors {
		if len(rawErr) == 0 {
			continue
		}
		if rawErr[0] == '{' {
			var objectError errorMessage
			err := json.Unmarshal(rawErr, &objectError)
			if err != nil {
				return r, "", err
			}
			errors = append(errors, objectError.Message)
		} else if rawErr[0] == '"' {
			var stringError string
			err := json.Unmarshal(rawErr, &stringError)
			if err != nil {
				return r, "", err
			}
			errors = append(errors, stringError)
		}
	}

	if len(errors) > 0 {
		return bodyCopy, strings.Join(errors, "\n"), nil
	}

	return bodyCopy, "", nil
}
