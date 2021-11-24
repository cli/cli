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

func createFakeListEndpointServer(t *testing.T, initalTotal int, finalTotal int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/codespaces" {
			t.Fatal("Incorrect path")
		}

		page := 1
		if r.URL.Query().Get("page") != "" {
			page, _ = strconv.Atoi(r.URL.Query().Get("page"))
		}

		per_page := 0
		if r.URL.Query().Get("per_page") != "" {
			per_page, _ = strconv.Atoi(r.URL.Query().Get("per_page"))
		}

		response := struct {
			Codespaces []*Codespace `json:"codespaces"`
			TotalCount int          `json:"total_count"`
		}{
			Codespaces: []*Codespace{},
			TotalCount: finalTotal,
		}

		switch page {
		case 1:
			response.Codespaces = generateCodespaceList(0, per_page)
			response.TotalCount = initalTotal
			w.Header().Set("Link", fmt.Sprintf(`<http://%[1]s/user/codespaces?page=3&per_page=%[2]d>; rel="last", <http://%[1]s/user/codespaces?page=2&per_page=%[2]d>; rel="next"`, r.Host, per_page))
		case 2:
			response.Codespaces = generateCodespaceList(per_page, per_page*2)
			response.TotalCount = finalTotal
			w.Header().Set("Link", fmt.Sprintf(`<http://%s/user/codespaces?page=3&per_page=%d>; rel="next"`, r.Host, per_page))
		case 3:
			response.Codespaces = generateCodespaceList(per_page*2, per_page*3-per_page/2)
			response.TotalCount = finalTotal
		default:
			t.Fatal("Should not check extra page")
		}

		data, _ := json.Marshal(response)
		fmt.Fprint(w, string(data))
	}))
}

func TestListCodespaces_limited(t *testing.T) {
	svr := createFakeListEndpointServer(t, 200, 200)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	codespaces, err := api.ListCodespaces(ctx, 200)
	if err != nil {
		t.Fatal(err)
	}

	if len(codespaces) != 200 {
		t.Fatalf("expected 200 codespace, got %d", len(codespaces))
	}
	if codespaces[0].Name != "codespace-0" {
		t.Fatalf("expected codespace-0, got %s", codespaces[0].Name)
	}
	if codespaces[199].Name != "codespace-199" {
		t.Fatalf("expected codespace-199, got %s", codespaces[0].Name)
	}
}

func TestListCodespaces_unlimited(t *testing.T) {
	svr := createFakeListEndpointServer(t, 200, 200)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	codespaces, err := api.ListCodespaces(ctx, -1)
	if err != nil {
		t.Fatal(err)
	}

	if len(codespaces) != 250 {
		t.Fatalf("expected 250 codespace, got %d", len(codespaces))
	}
	if codespaces[0].Name != "codespace-0" {
		t.Fatalf("expected codespace-0, got %s", codespaces[0].Name)
	}
	if codespaces[249].Name != "codespace-249" {
		t.Fatalf("expected codespace-249, got %s", codespaces[0].Name)
	}
}
