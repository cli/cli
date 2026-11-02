package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"text/scanner"
)

// RequestOwner extracts repository context without consuming the request body.
func RequestOwner(req *http.Request) (string, error) {
	parts := strings.Split(strings.Trim(req.URL.Path, "/"), "/")
	if len(parts) >= 3 && parts[0] == "api" && parts[1] == "v3" {
		parts = parts[2:]
	}
	if len(parts) > 1 {
		switch parts[0] {
		case "repos", "orgs":
			return parts[1], nil
		}
	}
	if !strings.HasSuffix(req.URL.Path, "/graphql") || req.GetBody == nil {
		return "", nil
	}
	body, err := req.GetBody()
	if err != nil {
		return "", err
	}
	defer body.Close()
	var query struct {
		Variables map[string]json.RawMessage `json:"variables"`
		Query     string                     `json:"query"`
	}
	if err := json.NewDecoder(body).Decode(&query); err != nil {
		return "", fmt.Errorf("read GraphQL identity context: %w", err)
	}
	owner := ""
	addOwner := func(value string) error {
		if owner != "" && value != "" && !strings.EqualFold(value, owner) {
			return errors.New("GraphQL request has multiple owners; split the request by owner")
		}
		if value != "" {
			owner = value
		}
		return nil
	}
	for _, name := range []string{"owner", "ownerName", "repositoryOwner", "repoOwner"} {
		raw, ok := query.Variables[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", fmt.Errorf("GraphQL owner variable %q is not a string", name)
		}
		if err := addOwner(value); err != nil {
			return "", err
		}
	}
	var lex scanner.Scanner
	lex.Init(strings.NewReader(query.Query))
	lex.Mode = scanner.ScanIdents | scanner.ScanStrings
	var scanErr error
	lex.Error = func(_ *scanner.Scanner, message string) { scanErr = errors.New(message) }
	for token := lex.Scan(); token != scanner.EOF; token = lex.Scan() {
		if token == '#' {
			for char := lex.Next(); char != '\n' && char != scanner.EOF; char = lex.Next() {
			}
			continue
		}
		if token != scanner.Ident || lex.TokenText() != "owner" {
			continue
		}
		if lex.Scan() != ':' {
			continue
		}
		token = lex.Scan()
		if token == scanner.String {
			value, err := strconv.Unquote(lex.TokenText())
			if err != nil {
				return "", err
			}
			if err := addOwner(value); err != nil {
				return "", err
			}
		} else if token == '$' && lex.Scan() == scanner.Ident {
			if raw, ok := query.Variables[lex.TokenText()]; ok {
				var value string
				if err := json.Unmarshal(raw, &value); err != nil {
					return "", fmt.Errorf("GraphQL owner variable is not a string: %w", err)
				}
				if err := addOwner(value); err != nil {
					return "", err
				}
			}
		}
	}
	if scanErr != nil {
		return "", fmt.Errorf("read GraphQL identity context: %w", scanErr)
	}
	return owner, nil
}
