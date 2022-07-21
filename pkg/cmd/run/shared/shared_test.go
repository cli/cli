package shared

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/httpmock"
	"github.com/stretchr/testify/assert"
)

func TestPreciseAgo(t *testing.T) {
	const form = "2006-Jan-02 15:04:05"
	now, _ := time.Parse(form, "2021-Apr-12 14:00:00")

	cases := map[string]string{
		"2021-Apr-12 14:00:00": "0s ago",
		"2021-Apr-12 13:59:30": "30s ago",
		"2021-Apr-12 13:59:00": "1m0s ago",
		"2021-Apr-12 13:30:15": "29m45s ago",
		"2021-Apr-12 13:00:00": "1h0m0s ago",
		"2021-Apr-12 02:30:45": "11h29m15s ago",
		"2021-Apr-11 14:00:00": "24h0m0s ago",
		"2021-Apr-01 14:00:00": "264h0m0s ago",
		"2021-Mar-12 14:00:00": "Mar 12, 2021",
	}

	for createdAt, expected := range cases {
		d, _ := time.Parse(form, createdAt)
		got := preciseAgo(now, d)
		if got != expected {
			t.Errorf("expected %s but got %s for %s", expected, got, createdAt)
		}
	}
}

func TestGetAnnotations404(t *testing.T) {
	reg := &httpmock.Registry{}
	defer reg.Verify(t)

	reg.Register(
		httpmock.REST("GET", "repos/OWNER/REPO/check-runs/123456/annotations"),
		httpmock.StatusStringResponse(404, "not found"))

	httpClient := &http.Client{Transport: reg}
	apiClient := api.NewClientFromHTTP(httpClient)
	repo := ghrepo.New("OWNER", "REPO")

	result, err := GetAnnotations(apiClient, repo, Job{ID: 123456, Name: "a job"})
	assert.NoError(t, err)
	assert.Equal(t, result, []Annotation{})
}

func TestRun_Duration(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2022-07-20T11:22:58Z")

	tests := []struct {
		name  string
		json  string
		wants string
	}{
		{
			name: "no run_started_at",
			json: heredoc.Doc(`
				{
					"created_at": "2022-07-20T11:20:13Z",
					"updated_at": "2022-07-20T11:21:16Z",
					"status": "completed"
				}`),
			wants: "1m3s",
		},
		{
			name: "with run_started_at",
			json: heredoc.Doc(`
				{
					"created_at": "2022-07-20T11:20:13Z",
					"run_started_at": "2022-07-20T11:20:55Z",
					"updated_at": "2022-07-20T11:21:16Z",
					"status": "completed"
				}`),
			wants: "21s",
		},
		{
			name: "in_progress",
			json: heredoc.Doc(`
				{
					"created_at": "2022-07-20T11:20:13Z",
					"run_started_at": "2022-07-20T11:20:55Z",
					"updated_at": "2022-07-20T11:21:16Z",
					"status": "in_progress"
				}`),
			wants: "2m3s",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var r Run
			assert.NoError(t, json.Unmarshal([]byte(tt.json), &r))
			assert.Equal(t, tt.wants, r.Duration(now).String())
		})
	}
}
