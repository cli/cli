package iapi

import (
	"fmt"
	"net/http"
	"time"

	"github.com/cli/cli/api"
	"github.com/cli/cli/internal/ghrepo"
	apiCmd "github.com/cli/cli/pkg/cmd/api"
	"github.com/cli/cli/pkg/cmdutil"
	"github.com/cli/cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type ApiOptions struct {
	IO         *iostreams.IOStreams
	HttpClient func() (*http.Client, error)
	BaseRepo   func() (ghrepo.Interface, error)
	Branch     func() (string, error)
	Builder    QueryBuilder
}

func NewCmdApi(f *cmdutil.Factory, runF func(*ApiOptions) error) *cobra.Command {
	opts := ApiOptions{
		IO:         f.IOStreams,
		HttpClient: f.HttpClient,
		BaseRepo:   f.BaseRepo,
		Branch:     f.Branch,
	}

	cmd := &cobra.Command{
		Use:  "iapi",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, args []string) error {
			return apiRun(&opts)
		},
	}

	return cmd
}

func apiRun(opts *ApiOptions) error {
	httpClient, err := opts.HttpClient()
	if err != nil {
		return err
	}
	cachedClient := api.NewCachedClient(httpClient, time.Hour*48)
	apiClient := api.NewClientFromHTTP(cachedClient)

	err = addQuery(apiClient, opts)
	if err != nil {
		return err
	}
	err = addArguments(apiClient, opts.Builder, true)
	if err != nil {
		return err
	}
	err = addFields(apiClient, opts.Builder, true)
	if err != nil {
		return err
	}

	return loop(apiClient, opts)
}

func loop(client *api.Client, opts *ApiOptions) error {
	for {
		next, err := nextSurvey()
		if err != nil {
			return err
		}
		switch next {
		case "Add arguments":
			err = addArguments(client, opts.Builder, false)
			if err != nil {
				return err
			}
		case "Add fields":
			err = addFields(client, opts.Builder, false)
			if err != nil {
				return err
			}
		case "Show query":
			query := opts.Builder.Generate()
			fmt.Println(query)
		case "Run query":
			query := opts.Builder.Generate()
			executeQuery(opts, query)
		case "Exit":
			return nil
		}
	}
}

func addQuery(client *api.Client, opts *ApiOptions) error {
	var response queryTypes
	err := client.GraphQL("github.com", queryTypesQuery(), nil, &response)
	if err != nil {
		return err
	}
	m := map[string]string{}
	for _, v := range response.Schema.QueryType.Fields {
		if v.Type.Name != "" {
			m[v.Name] = v.Type.Name
			continue
		}
		if v.Type.OfType.Name != "" {
			m[v.Name] = v.Type.OfType.Name
			continue
		}
		m[v.Name] = v.Name
	}
	o := []string{}
	for k, _ := range m {
		o = append(o, k)
	}
	name, err := queryTypeSurvey(o)
	if err != nil {
		return err
	}
	opts.Builder = NewQueryBuilder(name)
	opts.Builder.CurrentNode().AddKind(m[name])
	return nil
}

func addArguments(client *api.Client, builder QueryBuilder, skip bool) error {
	currentNode := builder.CurrentNode()
	if !skip {
		var err error
		var names []string
		for _, v := range builder.ArgumentNodes() {
			names = append(names, v.Name)
		}
		name, err := objectsSurvey(names)
		if err != nil {
			return err
		}
		currentNode = builder.FindNode(name)
	}
	options := []string{}
	var err error
	parentNode := currentNode.Parent
	if parentNode == nil {
		var response queryTypes
		err = client.GraphQL("github.com", queryTypesQuery(), nil, &response)
		if err != nil {
			return err
		}
		for _, v := range response.Schema.QueryType.Fields {
			if v.Name == currentNode.Name {
				for _, a := range v.Args {
					options = append(options, a.Name)
				}
			}
		}
	} else {
		options, err = retrieveArgumentOpts(client, parentNode.Kind, currentNode.Name)
		if err != nil {
			return err
		}
	}
	if len(options) > 0 {
		currentNode.AddArgumentsAvailable(true)
	}
	arguments, err := argumentsSurvey(options)
	if err != nil {
		return err
	}
	if arguments != nil {
		currentNode.AddArguments(arguments)
	}
	return nil
}

func addFields(client *api.Client, builder QueryBuilder, skip bool) error {
	node := builder.CurrentNode()
	if !skip {
		var names []string
		for _, v := range builder.FieldNodes() {
			names = append(names, v.Name)
		}
		name, err := objectsSurvey(names)
		if err != nil {
			return err
		}
		node = builder.FindNode(name)
	}
	m, err := retrieveFieldOpts(client, node.Kind)
	if err != nil {
		return err
	}
	var options []string
	for key, _ := range m {
		options = append(options, key)
	}
	if len(options) > 0 {
		node.AddFieldsAvailable(true)
	}
	fields, err := fieldsSurvey(options)
	if err != nil {
		return err
	}
	for _, v := range fields {
		nn := node.AddNode(v)
		nn.AddKind(m[v].Kind)
		nn.AddArgumentsAvailable(m[v].Args)
		fa, err := retrieveFieldsAvailable(client, nn.Kind)
		if err != nil {
			return err
		}
		nn.AddFieldsAvailable(fa)
	}
	return nil
}

func executeQuery(opts *ApiOptions, query string) error {
	q := fmt.Sprintf("query=%s", query)
	o := apiCmd.ApiOptions{
		HttpClient:  opts.HttpClient,
		IO:          opts.IO,
		BaseRepo:    opts.BaseRepo,
		Branch:      opts.Branch,
		RequestPath: "graphql",
		RawFields:   []string{q},
	}
	return apiCmd.IapiRun(&o)
}
