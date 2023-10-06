package queryfilter_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/internal/queryfilter"
	"github.com/stretchr/testify/require"
)

type Project struct {
	Name     string `json:"name"`
	Owner    string `json:"owner"`
	Template bool   `json:"template"`
}

type MarkTemplateMutation struct {
	Project Project `json:"project"`
}

func populateProjectFromStringJSON(t *testing.T, a any) {
	t.Helper()

	jsonString := `{"name": "CLI Backlog", "owner": "cli", "template": true}`
	require.NoError(t, json.Unmarshal([]byte(jsonString), a))
}

func populateMutationFromStringJSON(t *testing.T, a any) {
	t.Helper()

	jsonString := `{ "project": {"name": "CLI Backlog", "owner": "cli", "template": true} }`
	require.NoError(t, json.Unmarshal([]byte(jsonString), a))
}

func TestFilterTemplate(t *testing.T) {
	t.Skip()
	var project Project

	v := reflect.ValueOf(project)
	for i := 0; i < v.NumField(); i++ {
		fmt.Println(v.Field(i).Type())
		fmt.Println(v.Field(i))
	}

	filteredProject, err := queryfilter.FilterTemplate(project)
	require.NoError(t, err)

	v = reflect.ValueOf(filteredProject)
	for i := 0; i < v.NumField(); i++ {
		fmt.Println(v.Field(i).Type())
		fmt.Println(v.Field(i))
	}

	populateProjectFromStringJSON(t, &filteredProject)

	fmt.Printf("%+v\n", filteredProject)

	m, err := json.Marshal(filteredProject)
	require.NoError(t, err)

	require.NoError(t, json.Unmarshal(m, &project))

	fmt.Printf("%+v\n", project)
}

func TestFilterTemplateNested(t *testing.T) {
	var mutation MarkTemplateMutation

	v := reflect.ValueOf(mutation)
	for i := 0; i < v.NumField(); i++ {
		fmt.Println(v.Field(i).Type())
		fmt.Println(v.Field(i))
	}

	fmt.Println("----------")

	filteredMutation, err := queryfilter.FilterTemplate(&mutation)
	require.NoError(t, err)

	v = reflect.ValueOf(filteredMutation)
	for i := 0; i < v.NumField(); i++ {
		fmt.Println(v.Field(i).Type())
		fmt.Println(v.Field(i))
	}

	fmt.Println("----------")

	populateMutationFromStringJSON(t, &filteredMutation)

	fmt.Printf("%+v\n", filteredMutation)

	m, err := json.Marshal(filteredMutation)
	require.NoError(t, err)

	fmt.Println(string(m))

	require.NoError(t, json.Unmarshal(m, &mutation))

	fmt.Printf("%+v\n", mutation)
}
