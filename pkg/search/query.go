package search

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"
)

const (
	KindRepositories = "repositories"
	KindIssues       = "issues"
	KindCommits      = "commits"
)

type Query struct {
	Keywords   []string
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

func (q Query) String() string {
	qualifiers := formatQualifiers(q.Qualifiers)
	keywords := formatKeywords(q.Keywords)
	all := append(keywords, qualifiers...)
	return strings.TrimSpace(strings.Join(all, " "))
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

func formatQualifiers(qs Qualifiers) []string {
	var all []string
	for k, vs := range qs.Map() {
		for _, v := range vs {
			all = append(all, fmt.Sprintf("%s:%s", k, quote(v)))
		}
	}
	sort.Strings(all)
	return all
}

func formatKeywords(ks []string) []string {
	for i, k := range ks {
		before, after, found := strings.Cut(k, ":")
		if !found {
			ks[i] = quote(k)
		} else {
			ks[i] = fmt.Sprintf("%s:%s", before, quote(after))
		}
	}
	return ks
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
