package listcmd_test

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/cli/cli/v2/api"
	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/stretchr/testify/require"
)

type gqlQuery struct {
	Query     string
	Variables map[string]any
}

func TestGQLSponsorListing(t *testing.T) {
	// Given the server returns a valid query response
	// Everything between the *** markers could be abstracted but I didn't really want to do it prematurely.
	// I'd rather see the noise and find a pattern in it. Additionally, reading further down you'll see that the
	// test is actually highlighting an annoyance in the implementation, and instead of hiding it behind clever
	// code, I'd rather it was addressed.
	// ***
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First we ensure the body can be successfully decoded as a GQL query
		var q gqlQuery
		if err := json.NewDecoder(r.Body).Decode(&q); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(err.Error()))
			return
		}

		// Then we check that the query is exactly what we expected
		// One downside of this approach is that it's whitespace sensitive where GQL isn't. Food for thought!
		expectedQuery := `query ListSponsors($login:String!){user(login: $login){sponsors(first: 30){nodes{... on Organization{login},... on User{login}}}}}`
		if q.Query != expectedQuery {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("expected query: '%s' but got '%s'", expectedQuery, q.Query)))
			return
		}

		// Then we check that the variables are exactly what we expected
		// We _could_ test the entire request body against a single string but it definitely gets a bit unwieldy to read,
		// so asserting an exact expectation on the query and then a more specific one on the variables is a good balance.
		expectedVariables := map[string]any{"login": "testusername"}
		if !reflect.DeepEqual(q.Variables, expectedVariables) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("expected variables: '%#v' but got '%#v'", expectedVariables, q.Variables)))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"data": {
				"user": {
					"sponsors": {
						"nodes": [
							{
								"login": "sponsor1"
							},
							{
								"login": "sponsor2"
							}
						]
					}
				}
			}
		}`))
	}))
	// Unfortunately, because the APIClient does some nonsense for to prefix URLs with https we have to start
	// the server with TLS and then below we InsecureSkipVerify. In my opinion this is actually an implementation issue
	// that the tests are highlighting. It's kind of following the "if it's hard to test it's hard to use" principle.
	s.StartTLS()
	t.Cleanup(s.Close)

	c := listcmd.GQLSponsorClient{
		Hostname: s.Listener.Addr().String(),
		APIClient: api.NewClientFromHTTP(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}),
	}
	// ***

	// When we list the sponsors
	listedSponsors, err := c.ListSponsors("testusername")

	// Then we expect no error
	require.NoError(t, err)

	// And the sponsors match the query response
	require.Equal(t, listcmd.Sponsors{{"sponsor1"}, {"sponsor2"}}, listedSponsors)
}

func TestGQLSponsorListingServerError(t *testing.T) {
	// Given the server is returning an error
	s := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	s.StartTLS()
	t.Cleanup(s.Close)

	c := listcmd.GQLSponsorClient{
		Hostname: s.Listener.Addr().String(),
		APIClient: api.NewClientFromHTTP(&http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}),
	}

	// When we list the sponsors
	_, err := c.ListSponsors("testusername")

	// Then we expect to see a useful error
	require.ErrorContains(t, err, "list sponsors: non-200 OK status code")
}
