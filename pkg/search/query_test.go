package search

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var trueBool = true

func TestQueryString(t *testing.T) {
	tests := []struct {
		name  string
		query Query
		out   string
	}{
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.out, tt.query.String())
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
