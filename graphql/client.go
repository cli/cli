package graphql

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/user"
	"regexp"
)

type graphQLResponse struct {
	Data   interface{}
	Errors []struct {
		Message string
	}
}

/*
GraphQL usage

type repoResponse struct {
	repository struct {
		createdAt string
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
func GraphQL(query string, variables map[string]string, v interface{}) error {
	url := "https://api.github.com/graphql"
	reqBody, err := json.Marshal(map[string]interface{}{"query": query, "variables": variables})
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		panic(err)
	}

	req.Header.Set("Authorization", "token "+getToken())
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	return handleResponse(resp, v)
}

func handleResponse(resp *http.Response, v interface{}) error {
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if success {
		gr := &graphQLResponse{Data: v}
		err := json.NewDecoder(resp.Body).Decode(gr)
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

	return handleHTTPError(resp)
}

func handleHTTPError(resp *http.Response) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var message string
	var parsedBody struct {
		Message string `json:"message"`
	}
	err = json.Unmarshal(body, &parsedBody)
	if err != nil {
		message = string(body)
	} else {
		message = parsedBody.Message
	}

	return fmt.Errorf("http error, '%s' failed (%d): '%s'", resp.Request.URL, resp.StatusCode, message)
}

// THIS IS A BULLSHIT FUNCTION THAT SHOULD BE REMOVED
func getToken() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	content, err := ioutil.ReadFile(usr.HomeDir + "/.config/hub")
	if err != nil {
		panic(err)
	}

	r := regexp.MustCompile(`oauth_token: (\w+)`)
	token := r.FindStringSubmatch(string(content))
	return token[1]
}
