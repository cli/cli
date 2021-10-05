package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func generateCodespaceList(start int, end int) []*Codespace {
	codespacesList := []*Codespace{}
	for i := start; i < end; i++ {
		codespacesList = append(codespacesList, &Codespace{
			Name: fmt.Sprintf("codespace-%d", i),
		})
	}
	return codespacesList
}

func TestListCodespaces(t *testing.T) {

	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/codespaces" {
			t.Fatal("Incorrect path")
		}

		page := 1
		if r.URL.Query().Has("page") {
			page, _ = strconv.Atoi(r.URL.Query()["page"][0])
		}

		per_page := 0
		if r.URL.Query().Has("per_page") {
			per_page, _ = strconv.Atoi(r.URL.Query()["per_page"][0])
		}

		codespaces := []*Codespace{}
		if page == 1 {
			codespaces = generateCodespaceList(0, per_page)
		} else if page == 2 {
			codespaces = generateCodespaceList(per_page, per_page*2)
		}

		response := struct {
			Codespaces []*Codespace `json:"codespaces"`
			TotalCount int          `json:"total_count"`
		}{
			Codespaces: codespaces,
			TotalCount: 100,
		}
		data, _ := json.Marshal(response)
		fmt.Fprint(w, string(data))
	}))
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
		token:     "faketoken",
	}
	ctx := context.TODO()
	codespaces, err := api.ListCodespaces(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(codespaces) != 100 {
		t.Fatalf("expected 100 codespace, got %d", len(codespaces))
	}

	if codespaces[0].Name != "codespace-0" {
		t.Fatalf("expected codespace-0, got %s", codespaces[0].Name)
	}

	if codespaces[99].Name != "codespace-99" {
		t.Fatalf("expected codespace-99, got %s", codespaces[0].Name)
	}

}
