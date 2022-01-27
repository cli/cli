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

func createFakeSearchReposServer(t *testing.T, searchText string, responseRepos []*Repository) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/repositories" {
			t.Fatal("Incorrect path")
		}

		if r.URL.Query().Get("q") == "" {
			t.Fatal("Missing 'q' param")
		}

		q := r.URL.Query().Get("q")
		expectedQ := fmt.Sprintf("%s in:name", searchText)
		if q != expectedQ {
			t.Fatalf("expected q = '%s', got '%s", expectedQ, q)
		}

		if r.URL.Query().Get("per_page") == "" {
			t.Fatal("Missing 'per_page' param")
		}

		per_page, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
		expectedPerPage := 7 // This is currently hardcoded in the API logic
		if per_page != expectedPerPage {
			t.Fatalf("expected per_page = %d, got %d", expectedPerPage, per_page)
		}

		if r.URL.Query().Get("sort") == "" {
			t.Fatal("Missing 'sort' param")
		}

		sort := r.URL.Query().Get("sort")
		expectedSort := "stars" // This is currently hardcoded in the API logic
		if sort != expectedSort {
			t.Fatalf("expected sort = '%s', got '%s'", expectedSort, sort)
		}

		response := struct {
			Items []*Repository `json:"items"`
		}{
			responseRepos,
		}

		data, _ := json.Marshal(response)
		fmt.Fprint(w, string(data))
	}))
}

func TestGetRepoSuggestions_noSlash(t *testing.T) {
	searchText := "text"
	repos := []*Repository{
		{FullName: "repo1"},
		{FullName: "repo2"},
	}

	svr := createFakeSearchReposServer(t, searchText, repos)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	repoNames, err := api.GetCodespaceRepoSuggestions(ctx, searchText)
	if err != nil {
		t.Fatal(err)
	}

	if len(repoNames) != len(repos) {
		t.Fatalf("expected %d repos, got %d", len(repos), len(repoNames))
	}

	for i, expectedRepo := range repos {
		expectedName := expectedRepo.FullName
		actualName := repoNames[i]

		if expectedName != actualName {
			t.Fatalf("expected repo name for index %d to be %s, got %s", i, expectedName, actualName)
		}
	}
}

func TestGetRepoSuggestions_withSlash(t *testing.T) {
	searchText := "org/repo"
	queryText := "repo user:org"

	repos := []*Repository{
		{FullName: "repo1"},
		{FullName: "repo2"},
	}

	svr := createFakeSearchReposServer(t, queryText, repos)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	repoNames, err := api.GetCodespaceRepoSuggestions(ctx, searchText)
	if err != nil {
		t.Fatal(err)
	}

	if len(repoNames) != len(repos) {
		t.Fatalf("expected %d repos, got %d", len(repos), len(repoNames))
	}

	for i, expectedRepo := range repos {
		expectedName := expectedRepo.FullName
		actualName := repoNames[i]

		if expectedName != actualName {
			t.Fatalf("expected repo name for index %d to be %s, got %s", i, expectedName, actualName)
		}
	}
}

func TestGetRepoSuggestions_badFormat(t *testing.T) {
	searchText := "org/repo/bad"

	svr := createFakeSearchReposServer(t, "", nil)
	defer svr.Close()

	api := API{
		githubAPI: svr.URL,
		client:    &http.Client{},
	}
	ctx := context.TODO()
	_, err := api.GetCodespaceRepoSuggestions(ctx, searchText)
	if err == nil {
		t.Fatal("Expected error")
	}

	expected := "search value is an incorrect format"
	if err.Error() != expected {
		t.Fatalf("expected error '%s', go '%s'", expected, err.Error())
	}
}
