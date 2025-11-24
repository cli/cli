package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var trueBool = true

func TestStandardSearchString(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		out   string
	}{
		{
			name: "empty query",
			out:  "",
		},
		{
			name: "converts query to string",
			query: Query{
				Keywords: []string{"some", "keywords"},
				Qualifiers: Qualifiers{
					Archived:         &trueBool,
					AuthorEmail:      "foo@example.com",
					CommitterDate:    "2021-02-28",
					Created:          "created",
					Extension:        "go",
					Filename:         ".vimrc",
					Followers:        "1",
					Fork:             "true",
					Forks:            "2",
					GoodFirstIssues:  "3",
					HelpWantedIssues: "4",
					In:               []string{"description", "readme"},
					Language:         "language",
					License:          []string{"license"},
					Pushed:           "updated",
					Size:             "5",
					Stars:            "6",
					Topic:            []string{"topic"},
					Topics:           "7",
					User:             []string{"user1", "user2"},
					Is:               []string{"public"},
				},
			},
			out: "some keywords archived:true author-email:foo@example.com committer-date:2021-02-28 " +
				"created:created extension:go filename:.vimrc followers:1 fork:true forks:2 good-first-issues:3 help-wanted-issues:4 " +
				"in:description in:readme is:public language:language license:license pushed:updated size:5 " +
				"stars:6 topic:topic topics:7 user:user1 user:user2",
		},
		{
			name: "quotes keywords",
			query: Query{
				Keywords: []string{"quote keywords"},
			},
			out: `"quote keywords"`,
		},
		{
			name: "quotes keywords that are qualifiers",
			query: Query{
				Keywords: []string{"quote:keywords", "quote:multiword keywords"},
			},
			out: `quote:keywords quote:"multiword keywords"`,
		},
		{
			name: "quotes qualifiers",
			query: Query{
				Qualifiers: Qualifiers{
					Topic: []string{"quote qualifier"},
				},
			},
			out: `topic:"quote qualifier"`,
		},
		{
			name: "respects immutable keywords",
			query: Query{
				ImmutableKeywords: "immutable keyword that should be left as is",
			},
			out: `immutable keyword that should be left as is`,
		},
		{
			name: "respects immutable keywords, with qualifiers",
			query: Query{
				ImmutableKeywords: "immutable keyword that should be left as is",
				Qualifiers: Qualifiers{
					Topic: []string{"quote qualifier"},
				},
			},
			out: `immutable keyword that should be left as is topic:"quote qualifier"`,
		},
		{
			name: "prioritises immutable keywords over keywords slice",
			query: Query{
				Keywords:          []string{"foo", "bar"},
				ImmutableKeywords: "immutable keyword",
			},
			out: `immutable keyword`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, tt.query.StandardSearchString())
		})
	}
}

func TestAdvancedIssueSearchString(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		out   string
	}{
		{
			name: "empty query",
			out:  "",
		},
		{
			name: "quotes keywords",
			query: Query{
				Keywords: []string{"quote keywords"},
			},
			out: `"quote keywords"`,
		},
		{
			name: "quotes keywords that are qualifiers",
			query: Query{
				Keywords: []string{"quote:keywords", "quote:multiword keywords"},
			},
			out: `quote:keywords quote:"multiword keywords"`,
		},
		{
			name: "quotes qualifiers",
			query: Query{
				Qualifiers: Qualifiers{
					Label: []string{"quote qualifier"},
				},
			},
			out: `label:"quote qualifier"`,
		},
		{
			name: "respects immutable keywords",
			query: Query{
				ImmutableKeywords: "immutable keyword that should be left as is",
			},
			out: `immutable keyword that should be left as is`,
		},
		{
			name: "respects immutable keywords, with qualifiers",
			query: Query{
				ImmutableKeywords: "immutable keyword that should be left as is",
				Qualifiers: Qualifiers{
					Topic: []string{"quote qualifier"},
				},
			},
			out: `( immutable keyword that should be left as is ) topic:"quote qualifier"`,
		},
		{
			name: "prioritises immutable keywords over keywords slice",
			query: Query{
				Keywords:          []string{"foo", "bar"},
				ImmutableKeywords: "immutable keyword",
			},
			out: `immutable keyword`,
		},
		{
			name: "unused qualifiers should not appear in query",
			query: Query{
				Keywords: []string{"keyword"},
				Qualifiers: Qualifiers{
					Label: []string{"foo", "bar"},
				},
			},
			out: `( keyword ) label:bar label:foo`,
		},
		{
			name: "special qualifiers when used once",
			query: Query{
				Keywords: []string{"keyword"},
				Qualifiers: Qualifiers{
					Repo: []string{"foo/bar"},
					Is:   []string{"private"},
					User: []string{"johndoe"},
					In:   []string{"title"},
				},
			},
			out: `( keyword ) in:title is:private repo:foo/bar user:johndoe`,
		},
		{
			name: "special qualifiers are OR-ed when used multiple times",
			query: Query{
				Keywords: []string{"keyword"},
				Qualifiers: Qualifiers{
					Repo: []string{"foo/bar", "foo/baz"},
					Is:   []string{"private", "public", "issue", "pr", "open", "closed", "locked", "unlocked", "merged", "unmerged", "blocked", "blocking", "foo"}, // "foo" is to ensure only "public" and "private" are grouped
					User: []string{"johndoe", "janedoe"},
					In:   []string{"title", "body", "comments", "foo"}, // "foo" is to ensure only "title", "body", and "comments" are grouped
				},
			},
			out: `( keyword ) (in:body OR in:comments OR in:title) in:foo (is:blocked OR is:blocking) (is:closed OR is:open) (is:issue OR is:pr) (is:locked OR is:unlocked) (is:merged OR is:unmerged) (is:private OR is:public) is:foo (repo:foo/bar OR repo:foo/baz) (user:janedoe OR user:johndoe)`,
		},
		{
			// Since this is a general purpose package, we can't assume with know all
			// use cases of special qualifiers. So, here we ensure unknown values are
			// not OR-ed by default.
			name: "special qualifiers without special values",
			query: Query{
				Keywords: []string{"keyword"},
				Qualifiers: Qualifiers{
					Is: []string{"foo", "bar"},
					In: []string{"foo", "bar"},
				},
			},
			out: `( keyword ) in:bar in:foo is:bar is:foo`,
		},
		{
			name: "non-special qualifiers used multiple times",
			query: Query{
				Keywords: []string{"keyword"},
				Qualifiers: Qualifiers{
					In:      []string{"foo", "bar"}, // "in:" is a special qualifier but its values here are not special
					Is:      []string{"foo", "bar"}, // "is:" is a special qualifier but its values here are not special
					Label:   []string{"foo", "bar"},
					License: []string{"foo", "bar"},
					No:      []string{"foo", "bar"},
					Topic:   []string{"foo", "bar"},
				},
			},
			out: `( keyword ) in:bar in:foo is:bar is:foo label:bar label:foo license:bar license:foo no:bar no:foo topic:bar topic:foo`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, tt.query.AdvancedIssueSearchString())
		})
	}
}

func TestQualifiersMap(t *testing.T) {
	tests := []struct {
		name       string
		qualifiers Qualifiers
		out        map[string][]string
	}{
		{
			name: "changes qualifiers to map",
			qualifiers: Qualifiers{
				Archived:         &trueBool,
				AuthorEmail:      "foo@example.com",
				CommitterDate:    "2021-02-28",
				Created:          "created",
				Extension:        "go",
				Filename:         ".vimrc",
				Followers:        "1",
				Fork:             "true",
				Forks:            "2",
				GoodFirstIssues:  "3",
				HelpWantedIssues: "4",
				In:               []string{"readme"},
				Is:               []string{"public"},
				Language:         "language",
				License:          []string{"license"},
				Pushed:           "updated",
				Size:             "5",
				Stars:            "6",
				Topic:            []string{"topic"},
				Topics:           "7",
				User:             []string{"user1", "user2"},
			},
			out: map[string][]string{
				"archived":           {"true"},
				"author-email":       {"foo@example.com"},
				"committer-date":     {"2021-02-28"},
				"created":            {"created"},
				"extension":          {"go"},
				"filename":           {".vimrc"},
				"followers":          {"1"},
				"fork":               {"true"},
				"forks":              {"2"},
				"good-first-issues":  {"3"},
				"help-wanted-issues": {"4"},
				"in":                 {"readme"},
				"is":                 {"public"},
				"language":           {"language"},
				"license":            {"license"},
				"pushed":             {"updated"},
				"size":               {"5"},
				"stars":              {"6"},
				"topic":              {"topic"},
				"topics":             {"7"},
				"user":               {"user1", "user2"},
			},
		},
		{
			name: "excludes unset qualifiers from map",
			qualifiers: Qualifiers{
				Pushed: "updated",
				Size:   "5",
				Stars:  "6",
				User:   []string{"user"},
			},
			out: map[string][]string{
				"pushed": {"updated"},
				"size":   {"5"},
				"stars":  {"6"},
				"user":   {"user"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, tt.qualifiers.Map())
		})
	}
}

func TestCamelToKebab(t *testing.T) {
	tests := []struct {
		name string
		in   string
		out  string
	}{
		{
			name: "single lowercase word",
			in:   "test",
			out:  "test",
		},
		{
			name: "multiple mixed words",
			in:   "testTestTest",
			out:  "test-test-test",
		},
		{
			name: "multiple uppercase words",
			in:   "TestTest",
			out:  "test-test",
		},
		{
			name: "multiple lowercase words",
			in:   "testtest",
			out:  "testtest",
		},
		{
			name: "multiple mixed words with number",
			in:   "test2Test",
			out:  "test2-test",
		},
		{
			name: "multiple lowercase words with number",
			in:   "test2test",
			out:  "test2test",
		},
		{
			name: "multiple lowercase words with dash",
			in:   "test-test",
			out:  "test-test",
		},
		{
			name: "multiple uppercase words with dash",
			in:   "Test-Test",
			out:  "test--test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, camelToKebab(tt.in))
		})
	}
}
