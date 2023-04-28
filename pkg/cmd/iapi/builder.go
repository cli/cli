package iapi

import (
	"fmt"
	"strconv"
	"strings"
)

type QueryBuilder interface {
	ParentNode() *Node
	CurrentNode() *Node
	ChildNodes() []*Node
	AllNodes() []*Node
	FieldNodes() []*Node
	ArgumentNodes() []*Node
	FindNode(string) *Node
	Generate() string
}

// AddNode(string) *Node
// AddArgument(string, string)
// AddArguments(map[string]string)
// AddKind(string)
// AddArgumentsAvailable(bool)
// AddFieldsAvailable(bool)

type Node struct {
	Name      string
	Kind      string
	Parent    *Node
	Nodes     []*Node
	Arguments map[string]string

	ArgumentsAvailable bool
	FieldsAvailable    bool
}

func NewQueryBuilder(name string) QueryBuilder {
	return &Node{Name: name, Kind: name, Nodes: []*Node{}, Arguments: map[string]string{}}
}

func (n *Node) ParentNode() *Node {
	return n.Parent
}

func (n *Node) CurrentNode() *Node {
	return n
}

func (n *Node) ChildNodes() []*Node {
	return n.Nodes
}

func (n *Node) AllNodes() []*Node {
	var ns []*Node
	ns = append(ns, n)
	for _, cn := range n.ChildNodes() {
		ns = append(ns, cn.AllNodes()...)
	}
	return ns
}

func (n *Node) FieldNodes() []*Node {
	nodes := []*Node{}
	for _, nn := range n.AllNodes() {
		if nn.FieldsAvailable {
			nodes = append(nodes, nn)
		}
	}
	return nodes
}

func (n *Node) ArgumentNodes() []*Node {
	nodes := []*Node{}
	for _, nn := range n.AllNodes() {
		if nn.ArgumentsAvailable {
			nodes = append(nodes, nn)
		}
	}
	return nodes
}

func (n *Node) FindNode(name string) *Node {
	for _, s := range n.AllNodes() {
		if s.Name == name {
			return s
		}
	}
	return nil
}

func (n *Node) AddArgument(key string, value string) {
	n.Arguments[key] = value
}

func (n *Node) AddArguments(args map[string]string) {
	n.Arguments = args
}

func (n *Node) AddKind(k string) {
	n.Kind = k
}

func (n *Node) ArgumentsToString() string {
	if len(n.Arguments) == 0 {
		return ""
	}
	var args []string
	for k, v := range n.Arguments {
		if _, err := strconv.Atoi(v); err == nil {
			args = append(args, fmt.Sprintf("%s:%s", k, v))
		} else {
			args = append(args, fmt.Sprintf("%s:\"%s\"", k, v))
		}
	}
	return fmt.Sprintf("(%s)", strings.Join(args, ", "))
}

func (n *Node) AddNode(name string) *Node {
	nn := Node{Name: name, Kind: name, Parent: n, Nodes: []*Node{}, Arguments: map[string]string{}}
	n.Nodes = append(n.Nodes, &nn)
	return &nn
}

func (n *Node) AddArgumentsAvailable(b bool) {
	n.ArgumentsAvailable = b
}

func (n *Node) AddFieldsAvailable(b bool) {
	n.FieldsAvailable = b
}

func (n *Node) Generate() string {
	var b strings.Builder
	fmt.Fprintf(&b, "query {")
	n.generate(&b)
	fmt.Fprintf(&b, "}")
	return b.String()
}

func (n *Node) generate(b *strings.Builder) {
	fmt.Fprintf(b, " %s", n.Name)
	fmt.Fprintf(b, n.ArgumentsToString())
	if len(n.Nodes) > 0 {
		fmt.Fprintf(b, "{")
		for _, v := range n.Nodes {
			v.generate(b)
		}
		fmt.Fprintf(b, " }")
	}
}
