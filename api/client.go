package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

	"github.com/github/gh-cli/context"
	"github.com/github/gh-cli/version"
)

type graphQLResponse struct {
	Data   interface{}
	Errors []struct {
		Message string
	}
}

/*
GraphQL: Declared as an external variable so it can be mocked in tests

type repoResponse struct {
	Repository struct {
		CreatedAt string
	}
}

query := `query {
	repository(owner: "golang", name: "go") {
		createdAt
	}
}`

variables := map[string]string{}

var resp repoResponse
err := graphql(query, map[string]string{}, &resp)
if err != nil {
	panic(err)
}

fmt.Printf("%+v\n", resp)
*/
var GraphQL = func(query string, variables map[string]string, data interface{}) error {
	url := "https://api.github.com/graphql"
	reqBody, err := json.Marshal(map[string]interface{}{"query": query, "variables": variables})
	if err != nil {
		return err
	}

	body, err := request("POST", url, reqBody)
	if err != nil {
		return err
	}

	return handleGraphQLResponse(body, data)
}

func Get(path string, variables map[string]string, data interface{}) error {
	return rest("GET", path, variables, data)
}

func Post(path string, variables map[string]string, data interface{}) error {
	return rest("POST", path, variables, data)
}

func rest(method string, path string, variables map[string]string, data interface{}) error {
	url := "https://api.github.com/" + strings.TrimLeft(path, "/")
	reqBody, err := json.Marshal(variables)
	if err != nil {
		return err
	}

	body, err := request(method, url, reqBody)
	if err != nil {
		return err
	}

	err = json.Unmarshal(body, &data)
	if err != nil {
		return err
	}

	return nil
}

func request(method string, url string, reqBody []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	token, err := context.Current().AuthToken()
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "GitHub CLI "+version.Version)

	debugRequest(req, string(reqBody))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	debugResponse(resp, string(body))

	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	if !success {
		return nil, handleHTTPError(resp, body)
	}

	return body, nil
}

func handleGraphQLResponse(body []byte, data interface{}) error {
	gr := &graphQLResponse{Data: data}
	err := json.Unmarshal(body, &gr)
	if err != nil {
		return err
	}

	if len(gr.Errors) > 0 {
		errorMessages := gr.Errors[0].Message
		for _, e := range gr.Errors[1:] {
			errorMessages += ", " + e.Message
		}
		return fmt.Errorf("graphql error: '%s'", errorMessages)
	}
	return nil

}

func handleHTTPError(resp *http.Response, body []byte) error {
	var message string
	var parsedBody struct {
		Message string `json:"message"`
	}
	err := json.Unmarshal(body, &parsedBody)
	if err != nil {
		message = string(body)
	} else {
		message = parsedBody.Message
	}

	return fmt.Errorf("http error, '%s' failed (%d): '%s'", resp.Request.URL, resp.StatusCode, message)
}

func debugRequest(req *http.Request, body string) {
	if _, ok := os.LookupEnv("DEBUG"); !ok {
		return
	}

	fmt.Printf("DEBUG: GraphQL request to %s:\n %s\n\n", req.URL, body)
}

func debugResponse(resp *http.Response, body string) {
	if _, ok := os.LookupEnv("DEBUG"); !ok {
		return
	}

	fmt.Printf("DEBUG: GraphQL response:\n%+v\n\n%s\n\n", resp, body)
}
