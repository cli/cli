package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListCodespaces(t *testing.T) {
	codespaces := []*Codespace{
		{
			Name:       "testcodespace",
			CreatedAt:  "2021-08-09T10:10:24+02:00",
			LastUsedAt: "2021-08-09T13:10:24+02:00",
		},
	}
	svr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := struct {
			Codespaces []*Codespace `json:"codespaces"`
		}{
			Codespaces: codespaces,
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
	codespaces, err := api.ListCodespaces(ctx, "testuser")
	if err != nil {
		t.Fatal(err)
	}

	if len(codespaces) != 1 {
		t.Fatalf("expected 1 codespace, got %d", len(codespaces))
	}

	if codespaces[0].Name != "testcodespace" {
		t.Fatalf("expected testcodespace, got %s", codespaces[0].Name)
	}

}
