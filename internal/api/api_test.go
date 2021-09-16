package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListCodespaces(t *testing.T) {
	user := &User{
		Login: "testuser",
	}

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
	codespaces, err := api.ListCodespaces(ctx, user)
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

func TestCleanupUnusedCodespaces(t *testing.T) {
	type args struct {
		codespaces    []*Codespace
		thresholdDays int
	}
	tests := []struct {
		name    string
		now     time.Time
		args    args
		wantErr bool
		deleted []*Codespace
	}{
		{
			name: "no codespaces is to be deleted",

			args: args{
				codespaces: []*Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: 1,
			},
			now:     time.Date(2021, 8, 9, 20, 10, 24, 0, time.UTC),
			deleted: []*Codespace{},
		},
		{
			name: "one codespace is to be deleted",

			args: args{
				codespaces: []*Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: 1,
			},
			now: time.Date(2021, 8, 15, 20, 12, 24, 0, time.UTC),
			deleted: []*Codespace{
				{
					Name:       "testcodespace",
					CreatedAt:  "2021-08-09T10:10:24+02:00",
					LastUsedAt: "2021-08-09T13:10:24+02:00",
				},
			},
		},
		{
			name: "threshold is invalid",

			args: args{
				codespaces: []*Codespace{
					{
						Name:       "testcodespace",
						CreatedAt:  "2021-08-09T10:10:24+02:00",
						LastUsedAt: "2021-08-09T13:10:24+02:00",
						Environment: CodespaceEnvironment{
							State: "Shutdown",
						},
					},
				},
				thresholdDays: -1,
			},
			now:     time.Date(2021, 8, 15, 20, 12, 24, 0, time.UTC),
			wantErr: true,
			deleted: []*Codespace{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			now = func() time.Time {
				return tt.now
			}

			a := &API{
				token:  "testtoken",
				client: &http.Client{},
			}
			codespaces, err := a.FilterCodespacesToDelete(tt.args.codespaces, tt.args.thresholdDays)
			if (err != nil) != tt.wantErr {
				t.Errorf("API.CleanupUnusedCodespaces() error = %v, wantErr %v", err, tt.wantErr)
			}

			if len(codespaces) != len(tt.deleted) {
				t.Errorf("expected %d deleted codespaces, got %d", len(tt.deleted), len(codespaces))
			}
		})
	}
}
