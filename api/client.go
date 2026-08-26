package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	ghauth "github.com/cli/go-gh/v2/pkg/auth"
)

const (
	apiVersion      = "X-GitHub-Api-Version"
	apiVersionValue = "2022-11-28"
	authorization   = "Authorization"
	cacheTTL        = "X-GH-CACHE-TTL"
	graphqlFeatures = "GraphQL-Features"
	features        = "merge_queue"
	userAgent       = "User-Agent"
)

var linkRE = regexp.MustCompile(`<([^>]+)>;\s*rel="([^"]+)"`)
var requiredScopesRE = regexp.MustCompile(`one of the following scopes: \[(.+?)]`)

func NewClientFromHTTP(httpClient *http.Client) *Client {
	client := &Client{http: httpClient}
	return client
}

type Client struct {
	http *http.Client
}

func (c *Client) HTTP() *http.Client {
	return c.http
}

type GraphQLError struct {
	*ghAPI.GraphQLError
}

// ScopeRequirement is a set of OAuth scopes where any one scope satisfies the requirement.
type ScopeRequirement struct {
	// Scopes lists the alternative scopes that satisfy the requirement.
	Scopes []string
}

func (r ScopeRequirement) contains(scope string) bool {
	return slices.Contains(r.Scopes, scope)
}

// ScopeRequirements is a set of OAuth scope requirements that must all be satisfied.
type ScopeRequirements []ScopeRequirement

func (requirements ScopeRequirements) containingAny(scopes ...string) ScopeRequirements {
	var matches ScopeRequirements
	for _, requirement := range requirements {
		for _, scope := range scopes {
			if requirement.contains(scope) {
				matches = append(matches, requirement)
				break
			}
		}
	}
	return normalizeScopeRequirements(matches)
}

// MissingScopesError reports OAuth scope requirements for an API operation.
type MissingScopesError struct {
	// Requirements lists the scope requirements that must all be satisfied.
	Requirements ScopeRequirements
}

func (err MissingScopesError) Error() string {
	message := fmt.Sprintf(
		"error: your authentication token is missing required scopes [%s]",
		formatScopeRequirements(err.Requirements, " or ", ", "),
	)
	if len(err.Requirements) == 1 && len(err.Requirements[0].Scopes) > 1 {
		return fmt.Sprintf(
			"%s\nUpdate your authentication token to include one of: %s",
			message,
			strings.Join(err.Requirements[0].Scopes, ", "),
		)
	}

	formatted := make([]string, 0, len(err.Requirements))
	for _, requirement := range err.Requirements {
		if len(requirement.Scopes) == 1 {
			formatted = append(formatted, requirement.Scopes[0])
		} else {
			formatted = append(formatted, fmt.Sprintf("one of (%s)", strings.Join(requirement.Scopes, ", ")))
		}
	}
	return fmt.Sprintf(
		"%s\nUpdate your authentication token to include: %s",
		message,
		strings.Join(formatted, " and "),
	)
}

// GraphQLScopeRequirements returns OAuth scope requirements reported by a GraphQL response.
// Each returned requirement preserves scopes that the server reports as alternatives.
func GraphQLScopeRequirements(err error) ScopeRequirements {
	var gerr GraphQLError
	if !errors.As(err, &gerr) {
		return nil
	}

	var requirements ScopeRequirements
	for _, graphQLError := range gerr.Errors {
		if graphQLError.Type != "INSUFFICIENT_SCOPES" {
			continue
		}
		m := requiredScopesRE.FindStringSubmatch(graphQLError.Message)
		if m == nil {
			continue
		}
		var scopes []string
		for _, scope := range strings.Split(m[1], ",") {
			scope = strings.Trim(scope, "' ")
			if scope != "" {
				scopes = append(scopes, scope)
			}
		}
		if len(scopes) > 0 {
			requirements = append(requirements, ScopeRequirement{Scopes: scopes})
		}
	}
	return normalizeScopeRequirements(requirements)
}

// GraphQLMissingScopeError returns a user-facing error when a GraphQL response reports a requirement containing any of scopes.
func GraphQLMissingScopeError(err error, scopes ...string) error {
	requirements := GraphQLScopeRequirements(err).containingAny(scopes...)
	if len(requirements) == 0 {
		return nil
	}
	return MissingScopesError{Requirements: requirements}
}

func normalizeScopeRequirements(requirements ScopeRequirements) ScopeRequirements {
	unique := make(map[string]ScopeRequirement)
	for _, requirement := range requirements {
		scopes := append([]string(nil), requirement.Scopes...)
		sort.Slice(scopes, func(i, j int) bool {
			iRead := strings.HasPrefix(scopes[i], "read:")
			jRead := strings.HasPrefix(scopes[j], "read:")
			if iRead != jRead {
				return iRead
			}
			return scopes[i] < scopes[j]
		})
		scopes = slices.Compact(scopes)
		if len(scopes) == 0 {
			continue
		}
		key := strings.Join(scopes, "\x00")
		unique[key] = ScopeRequirement{Scopes: scopes}
	}

	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalized := make(ScopeRequirements, 0, len(keys))
	for _, key := range keys {
		normalized = append(normalized, unique[key])
	}
	return normalized
}

func formatScopeRequirements(requirements ScopeRequirements, alternativeSeparator, requirementSeparator string) string {
	formatted := make([]string, 0, len(requirements))
	for _, requirement := range requirements {
		formattedRequirement := strings.Join(requirement.Scopes, alternativeSeparator)
		if len(requirements) > 1 && len(requirement.Scopes) > 1 {
			formattedRequirement = fmt.Sprintf("(%s)", formattedRequirement)
		}
		formatted = append(formatted, formattedRequirement)
	}
	return strings.Join(formatted, requirementSeparator)
}

type HTTPError struct {
	*ghAPI.HTTPError
	scopesSuggestion string
}

func (err HTTPError) ScopesSuggestion() string {
	return err.scopesSuggestion
}

// GraphQL performs a GraphQL request using the query string and parses the response into data receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) GraphQL(hostname string, query string, variables map[string]any, data any) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Do(query, variables, data))
}

// Mutate performs a GraphQL mutation based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Mutate(hostname, name string, mutation any, variables map[string]any) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Mutate(name, mutation, variables))
}

// Query performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) Query(hostname, name string, query any, variables map[string]any) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.Query(name, query, variables))
}

// QueryWithContext performs a GraphQL query based on a struct and parses the response with the same struct as the receiver. If there are errors in the response,
// GraphQLError will be returned, but the receiver will also be partially populated.
func (c Client) QueryWithContext(ctx context.Context, hostname, name string, query any, variables map[string]any) error {
	opts := clientOptions(hostname, c.http.Transport)
	opts.Headers[graphqlFeatures] = features
	gqlClient, err := ghAPI.NewGraphQLClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(gqlClient.QueryWithContext(ctx, name, query, variables))
}

// REST performs a REST request and parses the response.
func (c Client) REST(hostname string, method string, p string, body io.Reader, data any) error {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := ghAPI.NewRESTClient(opts)
	if err != nil {
		return err
	}
	return handleResponse(restClient.Do(method, p, body, data))
}

func (c Client) RESTWithNext(hostname string, method string, p string, body io.Reader, data any) (string, error) {
	opts := clientOptions(hostname, c.http.Transport)
	restClient, err := ghAPI.NewRESTClient(opts)
	if err != nil {
		return "", err
	}

	resp, err := restClient.Request(method, p, body)
	if err != nil {
		return "", handleResponse(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return "", nil
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	err = json.Unmarshal(b, &data)
	if err != nil {
		return "", err
	}

	var next string
	for _, m := range linkRE.FindAllStringSubmatch(resp.Header.Get("Link"), -1) {
		if len(m) > 2 && m[2] == "next" {
			next = m[1]
		}
	}

	return next, nil
}

// HandleHTTPError parses a http.Response into a HTTPError.
//
// The caller is responsible to close the response body stream.
func HandleHTTPError(resp *http.Response) error {
	return handleResponse(ghAPI.HandleHTTPError(resp))
}

// handleResponse takes a ghAPI.HTTPError or ghAPI.GraphQLError and converts it into an
// HTTPError or GraphQLError respectively.
func handleResponse(err error) error {
	if err == nil {
		return nil
	}

	var restErr *ghAPI.HTTPError
	if errors.As(err, &restErr) {
		return HTTPError{
			HTTPError: restErr,
			scopesSuggestion: generateScopesSuggestion(restErr.StatusCode,
				restErr.Headers.Get("X-Accepted-Oauth-Scopes"),
				restErr.Headers.Get("X-Oauth-Scopes"),
				restErr.RequestURL.Hostname()),
		}
	}

	var gqlErr *ghAPI.GraphQLError
	if errors.As(err, &gqlErr) {
		return GraphQLError{
			GraphQLError: gqlErr,
		}
	}

	return err
}

// ScopesSuggestion is an error messaging utility that prints the suggestion to request additional OAuth
// scopes in case a server response indicates that there are missing scopes.
func ScopesSuggestion(resp *http.Response) string {
	return generateScopesSuggestion(resp.StatusCode,
		resp.Header.Get("X-Accepted-Oauth-Scopes"),
		resp.Header.Get("X-Oauth-Scopes"),
		resp.Request.URL.Hostname())
}

// EndpointNeedsScopes adds additional OAuth scopes to an HTTP response as if they were returned from the
// server endpoint. This improves HTTP 4xx error messaging for endpoints that don't explicitly list the
// OAuth scopes they need.
func EndpointNeedsScopes(resp *http.Response, s string) {
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		oldScopes := resp.Header.Get("X-Accepted-Oauth-Scopes")
		resp.Header.Set("X-Accepted-Oauth-Scopes", fmt.Sprintf("%s, %s", oldScopes, s))
	}
}

func generateScopesSuggestion(statusCode int, endpointNeedsScopes, tokenHasScopes, hostname string) string {
	if statusCode < 400 || statusCode > 499 || statusCode == 422 {
		return ""
	}

	if tokenHasScopes == "" {
		return ""
	}

	gotScopes := map[string]struct{}{}
	for s := range strings.SplitSeq(tokenHasScopes, ",") {
		s = strings.TrimSpace(s)
		gotScopes[s] = struct{}{}

		// Certain scopes may be grouped under a single "top-level" scope. The following branch
		// statements include these grouped/implied scopes when the top-level scope is encountered.
		// See https://docs.github.com/en/developers/apps/building-oauth-apps/scopes-for-oauth-apps.
		if s == "repo" {
			gotScopes["repo:status"] = struct{}{}
			gotScopes["repo_deployment"] = struct{}{}
			gotScopes["public_repo"] = struct{}{}
			gotScopes["repo:invite"] = struct{}{}
			gotScopes["security_events"] = struct{}{}
		} else if s == "user" {
			gotScopes["read:user"] = struct{}{}
			gotScopes["user:email"] = struct{}{}
			gotScopes["user:follow"] = struct{}{}
		} else if s == "codespace" {
			gotScopes["codespace:secrets"] = struct{}{}
		} else if after, ok := strings.CutPrefix(s, "admin:"); ok {
			gotScopes["read:"+after] = struct{}{}
			gotScopes["write:"+strings.TrimPrefix(s, "admin:")] = struct{}{}
		} else if after, ok := strings.CutPrefix(s, "write:"); ok {
			gotScopes["read:"+after] = struct{}{}
		}
	}

	for s := range strings.SplitSeq(endpointNeedsScopes, ",") {
		s = strings.TrimSpace(s)
		if _, gotScope := gotScopes[s]; s == "" || gotScope {
			continue
		}
		return fmt.Sprintf(
			"This API operation needs the %[1]q scope. To request it, run:  gh auth refresh -h %[2]s -s %[1]s",
			s,
			ghauth.NormalizeHostname(hostname),
		)
	}

	return ""
}

func clientOptions(hostname string, transport http.RoundTripper) ghAPI.ClientOptions {
	// AuthToken, and Headers are being handled by transport,
	// so let go-gh know that it does not need to resolve them.
	opts := ghAPI.ClientOptions{
		AuthToken: "none",
		Headers: map[string]string{
			authorization: "",
			apiVersion:    apiVersionValue,
		},
		Host:               hostname,
		SkipDefaultHeaders: true,
		Transport:          transport,
		LogIgnoreEnv:       true,
	}
	return opts
}
