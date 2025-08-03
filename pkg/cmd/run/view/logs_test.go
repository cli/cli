package view

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/run/shared"
	"github.com/cli/cli/v2/pkg/httpmock"
	ghAPI "github.com/cli/go-gh/v2/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZipLogFetcher(t *testing.T) {
	zr := createZipReader(t, map[string]string{
		"foo.txt": "blah blah",
	})

	fetcher := &zipLogFetcher{
		File: zr.File[0],
	}

	rc, err := fetcher.GetLog()
	assert.NoError(t, err)

	defer rc.Close()

	content, err := io.ReadAll(rc)
	assert.NoError(t, err)
	assert.Equal(t, "blah blah", string(content))
}

func TestApiLogFetcher(t *testing.T) {
	tests := []struct {
		name        string
		httpStubs   func(reg *httpmock.Registry)
		wantErr     string
		wantContent string
	}{
		{
			// This is the real flow as of now. When we call the `/logs`
			// endpoint, the server will respond with a 302 redirect, pointing
			// to the actual log file URL.
			name: "successful with redirect (HTTP 302, then HTTP 200)",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/123/logs"),
					httpmock.WithHeader(
						httpmock.StatusStringResponse(http.StatusFound, ""),
						"Location",
						"https://some.domain/the-actual-log",
					),
				)
				reg.Register(
					httpmock.REST("GET", "the-actual-log"),
					httpmock.StringResponse("blah blah"),
				)
			},
			wantContent: "blah blah",
		},
		{
			name: "successful without redirect (HTTP 200)",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/123/logs"),
					httpmock.StatusStringResponse(http.StatusOK, "blah blah"),
				)
			},
			wantContent: "blah blah",
		},
		{
			name: "failed with not found error (HTTP 404)",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/123/logs"),
					httpmock.StatusStringResponse(http.StatusNotFound, ""),
				)
			},
			wantErr: "log not found: 123",
		},
		{
			name: "failed with server error (HTTP 500)",
			httpStubs: func(reg *httpmock.Registry) {
				reg.Register(
					httpmock.REST("GET", "repos/OWNER/REPO/actions/jobs/123/logs"),
					httpmock.JSONErrorResponse(http.StatusInternalServerError, ghAPI.HTTPError{
						Message:    "blah blah",
						StatusCode: http.StatusInternalServerError,
					}),
				)
			},
			wantErr: "HTTP 500: blah blah (https://api.github.com/repos/OWNER/REPO/actions/jobs/123/logs)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg := &httpmock.Registry{}
			defer reg.Verify(t)

			tt.httpStubs(reg)

			httpClient := &http.Client{Transport: reg}

			fetcher := &apiLogFetcher{
				httpClient: httpClient,
				repo:       ghrepo.New("OWNER", "REPO"),
				jobID:      123,
			}

			rc, err := fetcher.GetLog()

			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
				assert.Nil(t, rc)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, rc)

			content, err := io.ReadAll(rc)
			assert.NoError(t, err)

			assert.NoError(t, rc.Close())
			assert.Equal(t, tt.wantContent, string(content))
		})
	}
}

func TestGetZipLogMap(t *testing.T) {
	tests := []struct {
		name      string
		job       shared.Job
		zipReader *zip.Reader
		// wantJobLog can be nil (i.e. not found) or string
		wantJobLog any
		// wantStepLogs elements can be nil (i.e. not found) or string
		wantStepLogs []any
	}{
		{
			name: "job log missing from zip, but step log present",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"job foo/1_step one.txt": "step one log",
			}),
			wantJobLog: nil,
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "matching job name and step number 1",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt":          "job log",
				"job foo/1_step one.txt": "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "matching job name and step number 2",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step two",
					Number: 2,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt":          "job log",
				"job foo/2_step two.txt": "step two log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
				"step two log",
			},
		},
		{
			// We should just look for the step number and not the step name.
			name: "matching job name and step number and mismatch step name",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "mismatch",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt":          "job log",
				"job foo/1_step one.txt": "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "matching job name and mismatch step number",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step two",
					Number: 2,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt":          "job log",
				"job foo/1_step one.txt": "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
				nil, // no log for step 2
			},
		},
		{
			name: "matching job name with no step logs in zip",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt": "job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "matching job name with no step data",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt": "job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "matching job name with random prefix and no step logs in zip",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"999999999_job foo.txt": "job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "matching job name with legacy filename and no step logs in zip",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"-9999999999_job foo.txt": "job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "matching job name with legacy filename and no step data",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
			},
			zipReader: createZipReader(t, map[string]string{
				"-9999999999_job foo.txt": "job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "matching job name with both normal and legacy filename",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo.txt":           "job log",
				"-9999999999_job foo.txt": "legacy job log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "one job name is a suffix of another",
			job: shared.Job{
				ID:   123,
				Name: "job foo",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_jjob foo.txt":          "the other job log",
				"jjob foo/1_step one.txt": "the other step one log",
				"1_job foo.txt":           "job log",
				"job foo/1_step one.txt":  "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "escape metacharacters in job name",
			job: shared.Job{
				ID:   123,
				Name: "metacharacters .+*?()|[]{}^$ job",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader:  createZipReader(t, nil),
			wantJobLog: nil,
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "mismatching job name",
			job: shared.Job{
				ID:   123,
				Name: "mismatch",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader:  createZipReader(t, nil),
			wantJobLog: nil,
			wantStepLogs: []any{
				nil, // no log for step 1
			},
		},
		{
			name: "job name with forward slash matches dir with slash removed",
			job: shared.Job{
				ID:   123,
				Name: "job foo / with slash",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo  with slash.txt":          "job log",
				"job foo  with slash/1_step one.txt": "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "job name with colon matches dir with colon removed",
			job: shared.Job{
				ID:   123,
				Name: "job foo : with colon",
				Steps: []shared.Step{{
					Name:   "step one",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"0_job foo  with colon.txt":          "job log",
				"job foo  with colon/1_step one.txt": "step one log",
			}),
			wantJobLog: "job log",
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "job name with really long name (over the ZIP limit)",
			job: shared.Job{
				ID:   123,
				Name: "thisisnineteenchars_thisisnineteenchars_thisisnineteenchars_thisisnineteenchars_thisisnineteenchars_",
				Steps: []shared.Step{{
					Name:   "long name job",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"thisisnineteenchars_thisisnineteenchars_thisisnineteenchars_thisisnineteenchars_thisisnine/1_long name job.txt": "step one log",
			}),
			wantJobLog: nil,
			wantStepLogs: []any{
				"step one log",
			},
		},
		{
			name: "job name that would be truncated by the C# server to split a grapheme",
			job: shared.Job{
				ID:   123,
				Name: "emoji test 😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅",
				Steps: []shared.Step{{
					Name:   "emoji job",
					Number: 1,
				}},
			},
			zipReader: createZipReader(t, map[string]string{
				"emoji test 😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅😅�/1_emoji job.txt": "step one log",
			}),
			wantJobLog: nil,
			wantStepLogs: []any{
				"step one log",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logMap := getZipLogMap(tt.zipReader, []shared.Job{tt.job})

			jobLogFile, ok := logMap.forJob(tt.job.ID)

			switch want := tt.wantJobLog.(type) {
			case nil:
				require.False(t, ok)
				require.Nil(t, jobLogFile)
			case string:
				require.True(t, ok)
				require.NotNil(t, jobLogFile)
				require.Equal(t, want, string(readZipFile(t, jobLogFile)))
			default:
				t.Fatal("wantJobLog must be nil or string")
			}

			for i, wantStepLog := range tt.wantStepLogs {
				stepLogFile, ok := logMap.forStep(tt.job.ID, 1+i) // Step numbers start from 1

				switch want := wantStepLog.(type) {
				case nil:
					require.False(t, ok)
					require.Nil(t, stepLogFile)
				case string:
					require.True(t, ok)
					require.NotNil(t, stepLogFile)

					gotStepLog := readZipFile(t, stepLogFile)
					require.Equal(t, want, string(gotStepLog))
				default:
					t.Fatal("wantStepLog must be nil or string")
				}
			}
		})
	}
}

func readZipFile(t *testing.T, zf *zip.File) []byte {
	rc, err := zf.Open()
	assert.NoError(t, err)
	defer rc.Close()

	content, err := io.ReadAll(rc)
	assert.NoError(t, err)
	return content
}

func createZipReader(t *testing.T, files map[string]string) *zip.Reader {
	raw := createZipArchive(t, files)

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	assert.NoError(t, err)

	return zr
}

func createZipArchive(t *testing.T, files map[string]string) []byte {
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)

	for name, content := range files {
		fileWriter, err := zw.Create(name)
		assert.NoError(t, err)

		_, err = fileWriter.Write([]byte(content))
		assert.NoError(t, err)
	}

	err := zw.Close()
	assert.NoError(t, err)

	return buf.Bytes()
}
