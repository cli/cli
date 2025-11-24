package search

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unicode"
)

const (
	KindRepositories = "repositories"
	KindCode         = "code"
	KindIssues       = "issues"
	KindCommits      = "commits"
)

type Query struct {
	// Keywords holds the list of keywords to search for. These keywords are
	// treated as individual components of a search query, and will get quoted
	// as needed. This is useful when the input can be supplied as a list of
	// search keywords.
	//
	// This field is overridden by ImmutableKeywords.
	Keywords []string

	// ImmutableKeywords holds the search keywords as a single string, and will
	// be treated as is (e.g. no additional quoting). This is useful when the
	// input is meant to be taken verbatim from the user.
	//
	// This field takes precedence over Keywords.
	ImmutableKeywords string

	Kind       string
	Limit      int
	Order      string
	Page       int
	Qualifiers Qualifiers
	Sort       string
}

type Qualifiers struct {
	Archived            *bool
	Assignee            string
	Author              string
	AuthorDate          string
	AuthorEmail         string
	AuthorName          string
	Base                string
	Closed              string
	Commenter           string
	Comments            string
	Committer           string
	CommitterDate       string
	CommitterEmail      string
	CommitterName       string
	Created             string
	Draft               *bool
	Extension           string
	Filename            string
	Followers           string
	Fork                string
	Forks               string
	GoodFirstIssues     string
	Hash                string
	Head                string
	HelpWantedIssues    string
	In                  []string
	Interactions        string
	Involves            string
	Is                  []string
	Label               []string
	Language            string
	License             []string
	Mentions            string
	Merge               *bool
	Merged              string
	Milestone           string
	No                  []string
	Parent              string
	Path                string
	Project             string
	Pushed              string
	Reactions           string
	Repo                []string
	Review              string
	ReviewRequested     string
	ReviewedBy          string
	Size                string
	Stars               string
	State               string
	Status              string
	Team                string
	TeamReviewRequested string
	Topic               []string
	Topics              string
	Tree                string
	Type                string
	Updated             string
	User                []string
}

// String returns the string representation of the query which can be used with
// the legacy search backend, which is used in global search GUI (i.e.
// github.com/search), or Pull Requests tab (in repositories). Note that this is
// a common query format that can be used to search for various entity types
// (e.g., issues, commits, repositories, etc)
//
// With the legacy search backend, the query is made of concatenating keywords
// and qualifiers with whitespaces. Note that at the backend side, most of the
// repeated qualifiers are AND-ed, while a handful of qualifiers (i.e.
// is:private/public, repo:, user:, or in:) are implicitly OR-ed. The legacy
// search backend does not support the advanced syntax which allows for nested
// queries and explicit OR operators.
//
// At the moment, the advanced search syntax is only available for searching
// issues, and it's called advanced issue search.
func (q Query) StandardSearchString() string {
	qualifiers := formatQualifiers(q.Qualifiers, nil)
	var keywords []string
	if q.ImmutableKeywords != "" {
		keywords = []string{q.ImmutableKeywords}
	} else if ks := formatKeywords(q.Keywords); len(ks) > 0 {
		keywords = ks
	}
	all := append(keywords, qualifiers...)
	return strings.TrimSpace(strings.Join(all, " "))
}

// AdvancedIssueSearchString returns the string representation of the query
// compatible with the advanced issue search syntax. The query can be used in
// Issues tab (of repositories) and the Issues dashboard (i.e.
// github.com/issues).
//
// As the name suggests, this query syntax is only supported for searching
// issues (i.e. issues and PRs). The advanced syntax allows nested queries and
// explicit OR operators. Unlike the legacy search backend, the advanced issue
// search does not OR repeated instances of special qualifiers (i.e.
// is:private/public, repo:, user:, or in:).
//
// To keep the gh experience consistent and backward-compatible, the mentioned
// special qualifiers are explicitly grouped and combined with an OR operator.
//
// The advanced syntax is documented at https://github.blog/changelog/2025-03-06-github-issues-projects-api-support-for-issues-advanced-search-and-more
func (q Query) AdvancedIssueSearchString() string {
	qualifiers := strings.Join(formatQualifiers(q.Qualifiers, formatAdvancedIssueSearch), " ")
	keywords := q.ImmutableKeywords
	if keywords == "" {
		keywords = strings.Join(formatKeywords(q.Keywords), " ")
	}

	if qualifiers == "" && keywords == "" {
		return ""
	}

	if qualifiers != "" && keywords != "" {
		// We should surround keywords with brackets to avoid leaking of any operators, especially "OR"s.
		return fmt.Sprintf("( %s ) %s", keywords, qualifiers)
	}

	if keywords != "" {
		return keywords
	}
	return qualifiers
}

func formatAdvancedIssueSearch(qualifier string, vs []string) (s []string, applicable bool) {
	switch qualifier {
	case "in":
		return formatSpecialQualifiers("in", vs, [][]string{{"title", "body", "comments"}}), true
	case "is":
		return formatSpecialQualifiers("is", vs, [][]string{{"blocked", "blocking"}, {"closed", "open"}, {"issue", "pr"}, {"locked", "unlocked"}, {"merged", "unmerged"}, {"private", "public"}}), true
	case "user", "repo":
		return []string{groupWithOR(qualifier, vs)}, true
	}
	// Let the default formatting take over
	return nil, false
}

func formatSpecialQualifiers(qualifier string, vs []string, specialGroupsToOR [][]string) []string {
	specialGroups := make([][]string, len(specialGroupsToOR))
	rest := make([]string, 0, len(vs))
	for _, v := range vs {
		var isSpecial bool
		for i, subValuesToOR := range specialGroupsToOR {
			if slices.Contains(subValuesToOR, v) {
				specialGroups[i] = append(specialGroups[i], v)
				isSpecial = true
				break
			}
		}

		if isSpecial {
			continue
		}

		rest = append(rest, v)
	}

	all := make([]string, 0, len(specialGroups)+len(rest))

	for _, group := range specialGroups {
		if len(group) == 0 {
			continue
		}
		all = append(all, groupWithOR(qualifier, group))
	}

	if len(rest) > 0 {
		for _, v := range rest {
			all = append(all, fmt.Sprintf("%s:%s", qualifier, quote(v)))
		}
	}

	slices.Sort(all)
	return all
}

func groupWithOR(qualifier string, vs []string) string {
	if len(vs) == 0 {
		return ""
	}

	all := make([]string, 0, len(vs))
	for _, v := range vs {
		all = append(all, fmt.Sprintf("%s:%s", qualifier, quote(v)))
	}

	if len(all) == 1 {
		return all[0]
	}

	slices.Sort(all)
	return fmt.Sprintf("(%s)", strings.Join(all, " OR "))
}

func (q Qualifiers) Map() map[string][]string {
	m := map[string][]string{}
	v := reflect.ValueOf(q)
	t := reflect.TypeOf(q)
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name
		key := camelToKebab(fieldName)
		typ := v.FieldByName(fieldName).Kind()
		value := v.FieldByName(fieldName)
		switch typ {
		case reflect.Ptr:
			if value.IsNil() {
				continue
			}
			v := reflect.Indirect(value)
			m[key] = []string{fmt.Sprintf("%v", v)}
		case reflect.Slice:
			if value.IsNil() {
				continue
			}
			s := []string{}
			for i := 0; i < value.Len(); i++ {
				if value.Index(i).IsZero() {
					continue
				}
				s = append(s, fmt.Sprintf("%v", value.Index(i)))
			}
			m[key] = s
		default:
			if value.IsZero() {
				continue
			}
			m[key] = []string{fmt.Sprintf("%v", value)}
		}
	}
	return m
}

func quote(s string) string {
	if strings.ContainsAny(s, " \"\t\r\n") {
		return fmt.Sprintf("%q", s)
	}
	return s
}

// formatQualifiers renders qualifiers into a plain query.
//
// The formatter is a custom formatting function that can be used to modify the
// output of each qualifier. If the formatter returns (nil, false) the default
// formatting will be applied.
func formatQualifiers(qs Qualifiers, formatter func(qualifier string, vs []string) (s []string, applicable bool)) []string {
	type entry struct {
		key    string
		values []string
	}

	var all []entry
	for k, vs := range qs.Map() {
		if len(vs) == 0 {
			continue
		}

		e := entry{key: k}

		if formatter != nil {
			if s, applicable := formatter(k, vs); applicable {
				e.values = s
				all = append(all, e)
				continue
			}
		}

		for _, v := range vs {
			e.values = append(e.values, fmt.Sprintf("%s:%s", k, quote(v)))
		}
		if len(e.values) > 1 {
			slices.Sort(e.values)
		}
		all = append(all, e)
	}

	slices.SortFunc(all, func(a, b entry) int {
		return strings.Compare(a.key, b.key)
	})

	result := make([]string, 0, len(all))
	for _, e := range all {
		result = append(result, e.values...)
	}
	return result
}

func formatKeywords(ks []string) []string {
	result := make([]string, len(ks))
	for i, k := range ks {
		before, after, found := strings.Cut(k, ":")
		if !found {
			result[i] = quote(k)
		} else {
			result[i] = fmt.Sprintf("%s:%s", before, quote(after))
		}
	}
	return result
}

// CamelToKebab returns a copy of the string s that is converted from camel case form to '-' separated form.
func camelToKebab(s string) string {
	var output []rune
	var segment []rune
	for _, r := range s {
		if !unicode.IsLower(r) && string(r) != "-" && !unicode.IsNumber(r) {
			output = addSegment(output, segment)
			segment = nil
		}
		segment = append(segment, unicode.ToLower(r))
	}
	output = addSegment(output, segment)
	return string(output)
}

func addSegment(inrune, segment []rune) []rune {
	if len(segment) == 0 {
		return inrune
	}
	if len(inrune) != 0 {
		inrune = append(inrune, '-')
	}
	inrune = append(inrune, segment...)
	return inrune
}
