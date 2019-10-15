package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"

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
graphQL usage

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
func graphQL(query string, variables map[string]string, v interface{}) error {
	url := "https://api.github.com/graphql"
	reqBody, err := json.Marshal(map[string]interface{}{"query": query, "variables": variables})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return err
	}

	ctx, err := context.GetContext()
	if err != nil {
		return err
	}
	token := ctx.Token

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("User-Agent", "GitHub CLI "+version.Version)

	debugRequest(req, string(reqBody))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	debugResponse(resp, string(body))
	return handleResponse(resp, body, v)
}

func handleResponse(resp *http.Response, body []byte, v interface{}) error {
	success := resp.StatusCode >= 200 && resp.StatusCode < 300

	if success {
		gr := &graphQLResponse{Data: v}
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

	return handleHTTPError(resp, body)
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
