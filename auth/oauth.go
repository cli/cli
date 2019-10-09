package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
)

func main() {
	state := "TODO" // replace with random unguessable value

	clientID := os.Getenv("GH_OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("GH_OAUTH_CLIENT_SECRET")

	code := ""
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("redirect_uri", fmt.Sprintf("http://localhost:%d", port))
	q.Set("scope", "repo")
	q.Set("state", state)

	cmd := exec.Command("open", fmt.Sprintf("https://github.com/login/oauth/authorize?%s", q.Encode()))
	err = cmd.Run()
	if err != nil {
		panic(err)
	}

	http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rq := r.URL.Query()
		code = rq.Get("code")
		// TODO: rq.Get("state")
		w.Header().Add("content-type", "text/html")
		fmt.Fprintf(w, "<p>You have authenticated <strong>GitHub CLI</strong>. You may now close this page.</p>")
		defer listener.Close()
	}))

	resp, err := http.PostForm("https://github.com/login/oauth/access_token",
		url.Values{
			"client_id":     {clientID},
			"client_secret": {clientSecret},
			"code":          {code},
			"state":         {state},
		})
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	tokenValues, err := url.ParseQuery(string(body))
	if err != nil {
		panic(err)
	}
	fmt.Printf("OAuth access token: %s\n", tokenValues.Get("access_token"))
}
