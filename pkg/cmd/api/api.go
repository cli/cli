package api

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
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/gh"
	"github.com/cli/cli/v2/internal/ghinstance"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/factory"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/cli/cli/v2/pkg/jsoncolor"
	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/cli/go-gh/v2/pkg/template"
	"github.com/spf13/cobra"
)

const (
	ttyIndent = "  "
)

type ApiOptions struct {
	AppVersion string
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Config     func() (gh.Config, error)
	HttpClient func() (*http.Client, error)
	IO         *iostreams.IOStreams

	Hostname            string
	RequestMethod       string
	RequestMethodPassed bool
	RequestPath         string
	RequestInputFile    string
	MagicFields         []string
	RawFields           []string
	RequestHeaders      []string
	Previews            []string
	ShowResponseHeaders bool
	Paginate            bool
	Slurp               bool
	Silent              bool
	Template            string
	CacheTTL            time.Duration
	FilterOutput        string
	Verbose             bool
}

func NewCmdApi(f *cmdutil.Factory, runF func(*ApiOptions) error) *cobra.Command {
	opts := ApiOptions{
		AppVersion: f.AppVersion,
		BaseRepo:   f.BaseRepo,
		Branch:     f.Branch,
		Config:     f.Config,
		IO:         f.IOStreams,
	}

	cmd := &cobra.Command{
		Use:   "api <endpoint>",
		Short: "Make an authenticated GitHub API request",
		Long: heredoc.Docf(`
			Makes an authenticated HTTP request to the GitHub API and prints the response.

			The endpoint argument should either be a path of a GitHub API v3 endpoint, or
			%[1]sgraphql%[1]s to access the GitHub API v4.

			Placeholder values %[1]s{owner}%[1]s, %[1]s{repo}%[1]s, and %[1]s{branch}%[1]s in the endpoint
			argument will get replaced with values from the repository of the current
			directory or the repository specified in the %[1]sGH_REPO%[1]s environment variable.
			Note that in some shells, for example PowerShell, you may need to enclose
			any value that contains %[1]s{...}%[1]s in quotes to prevent the shell from
			applying special meaning to curly braces.

			The default HTTP request method is %[1]sGET%[1]s normally and %[1]sPOST%[1]s if any parameters
			were added. Override the method with %[1]s--method%[1]s.

			Pass one or more %[1]s-f/--raw-field%[1]s values in %[1]skey=value%[1]s format to add static string
			parameters to the request payload. To add non-string or placeholder-determined values, see
			%[1]s-F/--field%[1]s below. Note that adding request parameters will automatically switch the
			request method to %[1]sPOST%[1]s. To send the parameters as a %[1]sGET%[1]s query string instead, use
			%[1]s--method GET%[1]s.

			The %[1]s-F/--field%[1]s flag has magic type conversion based on the format of the value:

			- literal values %[1]strue%[1]s, %[1]sfalse%[1]s, %[1]snull%[1]s, and integer numbers get converted to
			  appropriate JSON types;
			- placeholder values %[1]s{owner}%[1]s, %[1]s{repo}%[1]s, and %[1]s{branch}%[1]s get populated with values
			  from the repository of the current directory;
			- if the value starts with %[1]s@%[1]s, the rest of the value is interpreted as a
			  filename to read the value from. Pass %[1]s-%[1]s to read from standard input.

			For GraphQL requests, all fields other than %[1]squery%[1]s and %[1]soperationName%[1]s are
			interpreted as GraphQL variables.

			To pass nested parameters in the request payload, use %[1]skey[subkey]=value%[1]s syntax when
			declaring fields. To pass nested values as arrays, declare multiple fields with the
			syntax %[1]skey[]=value1%[1]s, %[1]skey[]=value2%[1]s. To pass an empty array, use %[1]skey[]%[1]s without a
			value.

			To pass pre-constructed JSON or payloads in other formats, a request body may be read
			from file specified by %[1]s--input%[1]s. Use %[1]s-%[1]s to read from standard input. When passing the
			request body this way, any parameters specified via field flags are added to the query
			string of the endpoint URL.

			In %[1]s--paginate%[1]s mode, all pages of results will sequentially be requested until
			there are no more pages of results. For GraphQL requests, this requires that the
			original query accepts an %[1]s$endCursor: String%[1]s variable and that it fetches the
			%[1]spageInfo{ hasNextPage, endCursor }%[1]s set of fields from a collection. Each page is a separate
			JSON array or object. Pass %[1]s--slurp%[1]s to wrap all pages of JSON arrays or objects
			into an outer JSON array.
		`, "`"),
		Example: heredoc.Doc(`
			# list releases in the current repository
			$ gh api repos/{owner}/{repo}/releases

			# post an issue comment
			$ gh api repos/{owner}/{repo}/issues/123/comments -f body='Hi from CLI'

			# post nested parameter read from a file
			$ gh api gists -F 'files[myfile.txt][content]=@myfile.txt'

			# add parameters to a GET request
			$ gh api -X GET search/issues -f q='repo:cli/cli is:open remote'

			# set a custom HTTP header
			$ gh api -H 'Accept: application/vnd.github.v3.raw+json' ...

			# opt into GitHub API previews
			$ gh api --preview baptiste,nebula ...

			# print only specific fields from the response
			$ gh api repos/{owner}/{repo}/issues --jq '.[].title'

			# use a template for the output
			$ gh api repos/{owner}/{repo}/issues --template \
			  '{{range .}}{{.title}} ({{.labels | pluck "name" | join ", " | color "yellow"}}){{"\n"}}{{end}}'

			# update allowed values of the "environment" custom property in a deeply nested array
			gh api -X PATCH /orgs/{org}/properties/schema \
			   -F 'properties[][property_name]=environment' \
			   -F 'properties[][default_value]=production' \
			   -F 'properties[][allowed_values][]=staging' \
			   -F 'properties[][allowed_values][]=production'

			# list releases with GraphQL
			$ gh api graphql -F owner='{owner}' -F name='{repo}' -f query='
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

			# get the percentage of forks for the current user
			$ gh api graphql --paginate --slurp -f query='
			  query($endCursor: String) {
			    viewer {
			      repositories(first: 100, after: $endCursor) {
			        nodes { isFork }
			        pageInfo {
			          hasNextPage
			          endCursor
			        }
			      }
			    }
			  }
			' | jq 'def count(e): reduce e as $_ (0;.+1);
			[.[].data.viewer.repositories.nodes[]] as $r | count(select($r[].isFork))/count($r[])'
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
		PreRun: func(c *cobra.Command, args []string) {
			opts.BaseRepo = cmdutil.OverrideBaseRepoFunc(f, "")
		},
		RunE: func(c *cobra.Command, args []string) error {
			opts.RequestPath = args[0]
			opts.RequestMethodPassed = c.Flags().Changed("method")

			if runtime.GOOS == "windows" && filepath.IsAbs(opts.RequestPath) {
				return fmt.Errorf(`invalid API endpoint: "%s". Your shell might be rewriting URL paths as filesystem paths. To avoid this, omit the leading slash from the endpoint argument`, opts.RequestPath)
			}

			if c.Flags().Changed("hostname") {
				if err := ghinstance.HostnameValidator(opts.Hostname); err != nil {
					return cmdutil.FlagErrorf("error parsing `--hostname`: %w", err)
				}
			}

			if opts.Paginate && !strings.EqualFold(opts.RequestMethod, "GET") && opts.RequestPath != "graphql" {
				return cmdutil.FlagErrorf("the `--paginate` option is not supported for non-GET requests")
			}

			if err := cmdutil.MutuallyExclusive(
				"the `--paginate` option is not supported with `--input`",
				opts.Paginate,
				opts.RequestInputFile != "",
			); err != nil {
				return err
			}

			if opts.Slurp && !opts.Paginate {
				return cmdutil.FlagErrorf("`--paginate` required when passing `--slurp`")
			}

			if err := cmdutil.MutuallyExclusive(
				"the `--slurp` option is not supported with `--jq` or `--template`",
				opts.Slurp,
				opts.FilterOutput != "",
				opts.Template != "",
			); err != nil {
				return err
			}

			if err := cmdutil.MutuallyExclusive(
				"only one of `--template`, `--jq`, `--silent`, or `--verbose` may be used",
				opts.Verbose,
				opts.Silent,
				opts.FilterOutput != "",
				opts.Template != "",
			); err != nil {
				return err
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
	cmd.Flags().StringSliceVarP(&opts.Previews, "preview", "p", nil, "GitHub API preview `names` to request (without the \"-preview\" suffix)")
	cmd.Flags().BoolVarP(&opts.ShowResponseHeaders, "include", "i", false, "Include HTTP response status line and headers in the output")
	cmd.Flags().BoolVar(&opts.Slurp, "slurp", false, "Use with \"--paginate\" to return an array of all pages of either JSON arrays or objects")
	cmd.Flags().BoolVar(&opts.Paginate, "paginate", false, "Make additional HTTP requests to fetch all pages of results")
	cmd.Flags().StringVar(&opts.RequestInputFile, "input", "", "The `file` to use as body for the HTTP request (use \"-\" to read from standard input)")
	cmd.Flags().BoolVar(&opts.Silent, "silent", false, "Do not print the response body")
	cmd.Flags().StringVarP(&opts.Template, "template", "t", "", "Format JSON output using a Go template; see \"gh help formatting\"")
	cmd.Flags().StringVarP(&opts.FilterOutput, "jq", "q", "", "Query to select values from the response using jq syntax")
	cmd.Flags().DurationVar(&opts.CacheTTL, "cache", 0, "Cache the response, e.g. \"3600s\", \"60m\", \"1h\"")
	cmd.Flags().BoolVar(&opts.Verbose, "verbose", false, "Include full HTTP request and response in the output")
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
	var requestBody interface{}
	if len(params) > 0 {
		requestBody = params
	}

	if !opts.RequestMethodPassed && (len(params) > 0 || opts.RequestInputFile != "") {
		method = "POST"
	}

	if !opts.Silent {
		if err := opts.IO.StartPager(); err == nil {
			defer opts.IO.StopPager()
		} else {
			fmt.Fprintf(opts.IO.ErrOut, "failed to start pager: %v\n", err)
		}
	}

	var bodyWriter io.Writer = opts.IO.Out
	var headersWriter io.Writer = opts.IO.Out
	if opts.Silent {
		bodyWriter = io.Discard
	}
	if opts.Verbose {
		// httpClient handles output when verbose flag is specified.
		bodyWriter = io.Discard
		headersWriter = io.Discard
	}

	if opts.Paginate && !isGraphQL {
		requestPath = addPerPage(requestPath, 100, params)
	}

	// Similar to `jq --slurp`, write all pages JSON arrays or objects into a JSON array.
	if opts.Paginate && opts.Slurp {
		w := &jsonArrayWriter{
			Writer: bodyWriter,
			color:  opts.IO.ColorEnabled(),
		}
		defer w.Close()

		bodyWriter = w
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

	if len(opts.Previews) > 0 {
		requestHeaders = append(requestHeaders, "Accept: "+previewNamesToMIMETypes(opts.Previews))
	}

	cfg, err := opts.Config()
	if err != nil {
		return err
	}

	if opts.HttpClient == nil {
		opts.HttpClient = func() (*http.Client, error) {
			log := opts.IO.ErrOut
			if opts.Verbose {
				log = opts.IO.Out
			}
			opts := api.HTTPClientOptions{
				AppVersion:     opts.AppVersion,
				CacheTTL:       opts.CacheTTL,
				Config:         cfg.Authentication(),
				EnableCache:    opts.CacheTTL > 0,
				Log:            log,
				LogColorize:    opts.IO.ColorEnabled(),
				LogVerboseHTTP: opts.Verbose,
			}
			return api.NewHTTPClient(opts)
		}
	}
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}

	host, _ := cfg.Authentication().DefaultHost()

	if opts.Hostname != "" {
		host = opts.Hostname
	}

	tmpl := template.New(bodyWriter, opts.IO.TerminalWidth(), opts.IO.ColorEnabled())
	err = tmpl.Parse(opts.Template)
	if err != nil {
		return err
	}

	isFirstPage := true
	hasNextPage := true
	for hasNextPage {
		resp, err := httpRequest(httpClient, host, method, requestPath, requestBody, requestHeaders)
		if err != nil {
			return err
		}

		if !isGraphQL {
			requestPath, hasNextPage = findNextPage(resp)
			requestBody = nil // prevent repeating GET parameters
		}

		// Tell optional jsonArrayWriter to start a new page.
		err = startPage(bodyWriter)
		if err != nil {
			return err
		}

		endCursor, err := processResponse(resp, opts, bodyWriter, headersWriter, tmpl, isFirstPage, !hasNextPage)
		if err != nil {
			return err
		}
		isFirstPage = false

		if !opts.Paginate {
			break
		}

		if isGraphQL {
			hasNextPage = endCursor != ""
			if hasNextPage {
				params["endCursor"] = endCursor
			}
		}

		if hasNextPage && opts.ShowResponseHeaders {
			fmt.Fprint(opts.IO.Out, "\n")
		}
	}

	return tmpl.Flush()
}

func processResponse(resp *http.Response, opts *ApiOptions, bodyWriter, headersWriter io.Writer, template *template.Template, isFirstPage, isLastPage bool) (endCursor string, err error) {
	if opts.ShowResponseHeaders {
		fmt.Fprintln(headersWriter, resp.Proto, resp.Status)
		printHeaders(headersWriter, resp.Header, opts.IO.ColorEnabled())
		fmt.Fprint(headersWriter, "\r\n")
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

	if opts.FilterOutput != "" && serverError == "" {
		// TODO: reuse parsed query across pagination invocations
		indent := ""
		if opts.IO.IsStdoutTTY() {
			indent = ttyIndent
		}
		err = jq.EvaluateFormatted(responseBody, bodyWriter, opts.FilterOutput, indent, opts.IO.ColorEnabled())
		if err != nil {
			return
		}
	} else if opts.Template != "" && serverError == "" {
		err = template.Execute(responseBody)
		if err != nil {
			return
		}
	} else if isJSON && opts.IO.ColorEnabled() {
		err = jsoncolor.Write(bodyWriter, responseBody, ttyIndent)
	} else {
		if isJSON && opts.Paginate && !opts.Slurp && !isGraphQLPaginate && !opts.ShowResponseHeaders {
			responseBody = &paginatedArrayReader{
				Reader:      responseBody,
				isFirstPage: isFirstPage,
				isLastPage:  isLastPage,
			}
		}
		_, err = io.Copy(bodyWriter, responseBody)
	}
	if err != nil {
		return
	}

	if serverError == "" && resp.StatusCode > 299 {
		serverError = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if serverError != "" {
		fmt.Fprintf(opts.IO.ErrOut, "gh: %s\n", serverError)
		if msg := api.ScopesSuggestion(resp); msg != "" {
			fmt.Fprintf(opts.IO.ErrOut, "gh: %s\n", msg)
		}
		if u := factory.SSOURL(); u != "" {
			fmt.Fprintf(opts.IO.ErrOut, "Authorize in your web browser: %s\n", u)
		}
		err = cmdutil.SilentError
		return
	}

	if isGraphQLPaginate {
		endCursor = findEndCursor(bodyCopy)
	}

	return
}

var placeholderRE = regexp.MustCompile(`(\:(owner|repo|branch)\b|\{[a-z]+\})`)

// fillPlaceholders replaces placeholders with values from the current repository
func fillPlaceholders(value string, opts *ApiOptions) (string, error) {
	var err error
	return placeholderRE.ReplaceAllStringFunc(value, func(m string) string {
		var name string
		if m[0] == ':' {
			name = m[1:]
		} else {
			name = m[1 : len(m)-1]
		}

		switch name {
		case "owner":
			if baseRepo, e := opts.BaseRepo(); e == nil {
				return baseRepo.RepoOwner()
			} else {
				err = e
			}
		case "repo":
			if baseRepo, e := opts.BaseRepo(); e == nil {
				return baseRepo.RepoName()
			} else {
				err = e
			}
		case "branch":
			if os.Getenv("GH_REPO") != "" {
				err = errors.New("unable to determine an appropriate value for the 'branch' placeholder")
				return m
			}

			if branch, e := opts.Branch(); e == nil {
				return branch
			} else {
				err = e
			}
		}
		return m
	}), err
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
	b, err := io.ReadAll(io.TeeReader(r, bodyCopy))
	if err != nil {
		return r, "", err
	}

	var parsedBody struct {
		Message string
		Errors  json.RawMessage
	}
	err = json.Unmarshal(b, &parsedBody)
	if err != nil {
		return bodyCopy, "", err
	}

	if len(parsedBody.Errors) > 0 && parsedBody.Errors[0] == '"' {
		var stringError string
		if err := json.Unmarshal(parsedBody.Errors, &stringError); err != nil {
			return bodyCopy, "", err
		}
		if stringError != "" {
			if parsedBody.Message != "" {
				return bodyCopy, fmt.Sprintf("%s (%s)", stringError, parsedBody.Message), nil
			}
			return bodyCopy, stringError, nil
		}
	}

	if parsedBody.Message != "" {
		return bodyCopy, fmt.Sprintf("%s (HTTP %d)", parsedBody.Message, statusCode), nil
	}

	if len(parsedBody.Errors) == 0 || parsedBody.Errors[0] != '[' {
		return bodyCopy, "", nil
	}

	var errorObjects []json.RawMessage
	if err := json.Unmarshal(parsedBody.Errors, &errorObjects); err != nil {
		return bodyCopy, "", err
	}

	var objectError struct {
		Message string
	}
	var errors []string
	for _, rawErr := range errorObjects {
		if len(rawErr) == 0 {
			continue
		}
		if rawErr[0] == '{' {
			err := json.Unmarshal(rawErr, &objectError)
			if err != nil {
				return bodyCopy, "", err
			}
			errors = append(errors, objectError.Message)
		} else if rawErr[0] == '"' {
			var stringError string
			err := json.Unmarshal(rawErr, &stringError)
			if err != nil {
				return bodyCopy, "", err
			}
			errors = append(errors, stringError)
		}
	}

	if len(errors) > 0 {
		return bodyCopy, strings.Join(errors, "\n"), nil
	}

	return bodyCopy, "", nil
}

func previewNamesToMIMETypes(names []string) string {
	types := []string{fmt.Sprintf("application/vnd.github.%s-preview+json", names[0])}
	for _, p := range names[1:] {
		types = append(types, fmt.Sprintf("application/vnd.github.%s-preview", p))
	}
	return strings.Join(types, ", ")
}
