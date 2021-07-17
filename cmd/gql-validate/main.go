package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

func main() {
	if err := mainRun(os.Args, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func mainRun(args []string, stdout io.Writer, stderr io.Writer) error {
	if len(args) < 3 {
		return errors.New("usage: gql-validate <schema-file> <query>")
	}

	schemaFile := args[1]
	bb, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		return fmt.Errorf("error loading schema file: %w", err)
	}
	schema, schemaErr := gqlparser.LoadSchema(&ast.Source{
		Name:    schemaFile,
		Input:   string(bb),
		BuiltIn: true,
	})
	if schemaErr != nil {
		return fmt.Errorf("error loading schema: %w", schemaErr)
	}

	doc, queryErr := gqlparser.LoadQuery(schema, args[2])
	if len(queryErr) > 0 {
		return fmt.Errorf("query error: %w", queryErr)
	}

	fmt.Fprintf(stdout, "doc: %#v\n", doc.Operations)
	return nil
}
