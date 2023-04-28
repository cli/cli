package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/cli/cli/internal/ghinstance"
	"github.com/cli/cli/pkg/cmd/factory"
	"github.com/shurcooL/githubv4"
	"github.com/shurcooL/graphql"
)

type QueryBuilder interface {
	AddField(string)
	AddNode(string) *Node
	AddVariable(string, string)
	Generate() reflect.Value
}

type Node struct {
	Kind      string
	Fields    []string
	Nodes     []*Node
	Arrays    []*Node
	Variables map[string]string
}

type Comments struct {
	Nodes      []Comment
	TotalCount int
}

type Comment struct {
	Body        string
	IsMinimized bool
}

type PullRequest struct {
	Comments Comments
}

type Repository struct {
	Name string
}

func QueryBuilderForKind(kind string) QueryBuilder {
	return &Node{Kind: kind, Fields: []string{}, Nodes: []*Node{}, Variables: map[string]string{}}
}

func (n *Node) AddField(field string) {
	n.Fields = append(n.Fields, field)
}

func (n *Node) AddVariable(key string, value string) {
	n.Variables[key] = value
}

func (n *Node) AddNode(kind string) *Node {
	nn := Node{Kind: kind, Fields: []string{}, Nodes: []*Node{}, Variables: map[string]string{}}
	n.Nodes = append(n.Nodes, &nn)
	return &nn
}

func (n *Node) AddArray(kind string) *Node {
	nn := Node{Kind: kind, Fields: []string{}, Nodes: []*Node{}, Variables: map[string]string{}}
	n.Arrays = append(n.Arrays, &nn)
	return &nn
}

func (n *Node) Generate() reflect.Value {
	x := reflect.StructOf(n.generate())
	b := reflect.StructField{
		Name: strings.Title(n.Kind),
		Type: x,
		Tag:  variablesToTag(n),
	}
	y := reflect.StructOf([]reflect.StructField{b})
	return reflect.New(y).Elem()
}

func (n *Node) generate() []reflect.StructField {
	fields := []reflect.StructField{}
	t := typeFromKind(n.Kind)

	for _, f := range n.Fields {
		s, _ := t.FieldByName(strings.Title(f))
		n := reflect.StructField{
			Name: strings.Title(f),
			Type: s.Type,
		}
		fields = append(fields, n)
	}

	for _, a := range n.Arrays {
		x := reflect.StructOf(a.generate())
		b := reflect.StructField{
			Name: strings.Title(a.Kind),
			Type: reflect.SliceOf(x),
			Tag:  variablesToTag(a),
		}
		fields = append(fields, b)
	}

	for _, nn := range n.Nodes {
		x := reflect.StructOf(nn.generate())
		b := reflect.StructField{
			Name: strings.Title(nn.Kind),
			Type: x,
			Tag:  variablesToTag(nn),
		}
		fields = append(fields, b)
	}

	return fields
}

func variablesToTag(n *Node) reflect.StructTag {
	m := n.Variables
	var b strings.Builder
	if len(m) > 0 {
		fmt.Fprintf(&b, `graphql:"%s(`, n.Kind)
	}
	s := []string{}
	for k, v := range m {
		s = append(s, fmt.Sprintf("%s: %s", k, v))
	}
	fmt.Fprintf(&b, strings.Join(s, ", "))
	if len(m) > 0 {
		fmt.Fprintf(&b, `)"`)
	}
	return reflect.StructTag(b.String())
}

func typeFromKind(kind string) reflect.Type {
	m := map[string]interface{}{
		"repository":  Repository{},
		"pullRequest": PullRequest{},
		"comments":    Comments{},
		"nodes":       Comment{},
	}
	return reflect.TypeOf(m[kind])
}

func main() {
	qb := QueryBuilderForKind("repository")
	qb.AddVariable("owner", "$owner")
	qb.AddVariable("name", "$repo")
	prqb := qb.AddNode("pullRequest")
	prqb.AddVariable("number", "$number")
	cqb := prqb.AddNode("comments")
	cqb.AddVariable("first", "100")
	cqb.AddField("totalCount")
	nqb := cqb.AddArray("nodes")
	nqb.AddField("body")
	// nqb.AddField("isMinimized")

	if os.Getenv("NAME") != "" {
		qb.AddField("name")
	}

	x := qb.Generate()

	// type response struct {
	// 	Repository struct {
	// 		PullRequest struct {
	// 			Comments struct {
	// 				 Nodes []struct {
	//					Body string
	// 				 }
	// 				TotalCount int
	// 			} "graphql:\"comments(first: 100)\""
	// 		} "graphql:\"pullRequest(number: $number)\""
	// 	} "graphql:\"repository(owner: $owner, name: $repo)\""
	// }

	variables := map[string]interface{}{
		"owner":  githubv4.String("cli"),
		"repo":   githubv4.String("cli"),
		"number": githubv4.Int(2776),
	}

	f := factory.New("1.6")
	cfg, _ := f.Config()
	h := factory.NewHTTPClient(f.IOStreams, cfg, "1.6", false)
	c := graphql.NewClient(ghinstance.GraphQLEndpoint("github.com"), h)
	c.Query(context.Background(), x.Addr().Interface(), variables)
	fmt.Println(x)
}
