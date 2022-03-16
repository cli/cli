package search

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/cli/cli/v2/pkg/text"
)

const (
	KindRepositories = "repositories"
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
	Archived         *bool
	Created          string
	Followers        string
	Fork             string
	Forks            string
	GoodFirstIssues  string
	HelpWantedIssues string
	In               []string
	Is               string
	Language         string
	License          []string
	Org              string
	Pushed           string
	Size             string
	Stars            string
	Topic            []string
	Topics           string
}

func (q Query) String() string {
	qualifiers := formatQualifiers(q.Qualifiers)
	keywords := formatKeywords(q.Keywords)
	all := append(keywords, qualifiers...)
	return strings.Join(all, " ")
}

func (q Qualifiers) Map() map[string][]string {
	m := map[string][]string{}
	v := reflect.ValueOf(q)
	t := reflect.TypeOf(q)
	for i := 0; i < v.NumField(); i++ {
		fieldName := t.Field(i).Name
		key := text.CamelToKebab(fieldName)
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
		ks[i] = quote(k)
	}
	return ks
}
