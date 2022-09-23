package webhooks

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/cli/cli/v2/api"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/google/shlex"
	"github.com/stretchr/testify/assert"
)

func TestNewCmdForward(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	f := &cmdutil.Factory{
		IOStreams: ios,
	}

	var opts *hookOptions
	cmd := newCmdForward(f, func(o *hookOptions) error {
		opts = o
		return nil
	})

	events := "issues"
	port := 9999
	repo := "monalisa/smile"
	host := "api.github.localhost"
	run := fmt.Sprintf("--events %s --port %d --repo %s --host %s", events, port, repo, host)

	argv, err := shlex.Split(run)
	assert.NoError(t, err)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err = cmd.ExecuteC()
	assert.NoError(t, err)
	assert.Equal(t, []string{events}, opts.EventTypes)
	assert.Equal(t, port, opts.Port)
	assert.Equal(t, repo, opts.Repo)
	assert.Equal(t, host, opts.Host)
}

func TestForwardRun_HappyPath(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		HttpClient: func() (*http.Client, error) {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			return &http.Client{Transport: tr}, nil
		},
	}

	wsServer := getWSServer(t, false)
	defer wsServer.Close()

	ghAPIServer := getGHAPIServer(wsServer.URL, false, http.StatusOK, t)
	defer ghAPIServer.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	webhookRcvServer, forwarded := getWebhookRcvServer(t, wg.Done, false, http.StatusOK)
	defer webhookRcvServer.Close()
	webhookRcvServerURL = webhookRcvServer.URL

	maxRetries = 1
	cmd := newCmdForward(f, nil)
	events := "issues"
	splitURL := strings.Split(webhookRcvServerURL, ":")
	port := splitURL[len(splitURL)-1]
	repo := "monalisa/smile"
	host := strings.TrimPrefix(ghAPIServer.URL, "https://")

	run := fmt.Sprintf("--events %s --port %s --repo %s --host %s", events, port, repo, host)
	argv, err := shlex.Split(run)
	assert.NoError(t, err)
	cmd.SetArgs(argv)
	cmd.SetIn(&bytes.Buffer{})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err = cmd.ExecuteC()
	assert.NoError(t, err)
	wg.Wait()
	assert.Equal(t, "lol", forwarded.event.Body)
	assert.Equal(t, forwarded.event.Header.Get("Someheader"), "somevalue")
}

func TestForwardRun_Errors(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.SetStdinTTY(true)
	ios.SetStdoutTTY(true)

	f := &cmdutil.Factory{
		IOStreams: ios,
		Config: func() (config.Config, error) {
			return config.NewBlankConfig(), nil
		},
		HttpClient: func() (*http.Client, error) {
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}

			return &http.Client{Transport: tr}, nil
		},
	}

	tests := []struct {
		name               string
		wantAPIErr         bool
		wantWSServerErr    bool
		wantLocalServerErr bool
		wantStatusCode     int
		wantWSServerErrMsg string
		wantOutput         bool
	}{
		{
			name:               "return error when GH API returns an error",
			wantAPIErr:         true,
			wantWSServerErr:    false,
			wantLocalServerErr: false,
			wantStatusCode:     http.StatusNotFound,
		},
		{
			name:               "return error when WS server returns an error",
			wantAPIErr:         false,
			wantWSServerErr:    true,
			wantLocalServerErr: false,
			wantOutput:         true,
		},
		{
			name:               "return error when local web server returns an error",
			wantAPIErr:         false,
			wantWSServerErr:    false,
			wantLocalServerErr: true,
			wantStatusCode:     http.StatusBadRequest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out.Reset()

			wsServer := getWSServer(t, test.wantWSServerErr)
			defer wsServer.Close()

			ghAPIServer := getGHAPIServer(wsServer.URL, test.wantAPIErr, test.wantStatusCode, t)
			defer ghAPIServer.Close()

			var wg sync.WaitGroup
			wg.Add(1)
			webhookRcvServer, forwarded := getWebhookRcvServer(t, wg.Done, test.wantLocalServerErr, test.wantStatusCode)
			defer webhookRcvServer.Close()
			webhookRcvServerURL = webhookRcvServer.URL

			maxRetries = 1
			cmd := newCmdForward(f, nil)
			events := "issues"
			splitURL := strings.Split(webhookRcvServerURL, ":")
			port := splitURL[len(splitURL)-1]
			repo := "monalisa/smile"
			host := strings.TrimPrefix(ghAPIServer.URL, "https://")

			run := fmt.Sprintf("--events %s --port %s --repo %s --host %s", events, port, repo, host)
			argv, err := shlex.Split(run)
			assert.NoError(t, err)
			cmd.SetArgs(argv)
			cmd.SetIn(&bytes.Buffer{})
			if !test.wantOutput {
				cmd.SetOut(&bytes.Buffer{})
			}
			cmd.SetErr(&bytes.Buffer{})

			_, err = cmd.ExecuteC()

			if test.wantAPIErr {
				assert.NotNil(t, err)
				apiErr, ok := err.(api.HTTPError)
				if !ok {
					t.Fatalf("want an api.HTTPError, got: %v", err)
				}
				assert.Equal(t, apiErr.StatusCode, test.wantStatusCode)
			}

			if test.wantWSServerErr {
				assert.Nil(t, err)
				assert.Contains(t, out.String(), "An error occurred, please try again.")
			}

			if test.wantLocalServerErr {
				assert.Nil(t, err)
				assert.Equal(t, forwarded.returnStatusCode, test.wantStatusCode)
				assert.Equal(t, forwarded.event.Body, "lol")
				assert.Equal(t, forwarded.event.Header.Get("Someheader"), "somevalue")
			}
		})
	}
}
