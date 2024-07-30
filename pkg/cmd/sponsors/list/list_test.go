package listcmd_test

import (
	"errors"
	"net/http"
	"testing"

	listcmd "github.com/cli/cli/v2/pkg/cmd/sponsors/list"
	"github.com/cli/cli/v2/pkg/cmdutil"
	"github.com/cli/cli/v2/pkg/jsonfieldstest"
	"github.com/google/shlex"
	"github.com/stretchr/testify/require"
)

func TestNewCmdSponsors(t *testing.T) {
	tests := []struct {
		name            string
		args            string
		expectedErr     error
		expectedOptions listcmd.Options
	}{
		{
			name: "org",
			args: "testusername",
			expectedOptions: listcmd.Options{
				User: "testusername",
			},
		},
		{
			name:        "when too many arguments provided, returns a useful error",
			args:        "foo bar",
			expectedErr: cmdutil.FlagErrorf("too many arguments"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &cmdutil.Factory{
				HttpClient: func() (*http.Client, error) {
					return nil, nil
				},
			}

			argv, err := shlex.Split(tt.args)
			require.NoError(t, err)

			var optsSpy listcmd.Options
			cmd := listcmd.NewCmdList(f, func(opts listcmd.Options) error {
				optsSpy = opts
				return nil
			})
			cmd.SetArgs(argv)

			_, err = cmd.ExecuteC()
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expectedOptions.User, optsSpy.User)
		})
	}
}

func TestJSONFields(t *testing.T) {
	jsonfieldstest.ExpectCommandToSupportJSONFields[listcmd.Options](t, listcmd.NewCmdList, []string{
		"login",
	})
}

type fakeSponsorLister struct {
	stubbedSponsors map[listcmd.User]listcmd.Sponsors
	stubbedErr      error
}

func (s fakeSponsorLister) ListSponsors(user listcmd.User) (listcmd.Sponsors, error) {
	if s.stubbedErr != nil {
		return listcmd.Sponsors{}, s.stubbedErr
	}
	return s.stubbedSponsors[user], nil
}

type spySponsorTableRenderer struct {
	spiedSponsors listcmd.Sponsors
}

func (s *spySponsorTableRenderer) Render(sponsors listcmd.Sponsors) error {
	s.spiedSponsors = sponsors
	return nil
}

func TestListSponsorsRendersRetrievedSponsors(t *testing.T) {
	// Given our sponsor lister retrieves some sponsors
	sponsorListRendererSpy := &spySponsorTableRenderer{}
	listOptions := listcmd.Options{
		SponsorLister: fakeSponsorLister{
			stubbedSponsors: map[listcmd.User]listcmd.Sponsors{
				"testusername": {{"sponsor1"}, {"sponsor2"}},
			},
		},
		SponsorListRenderer: sponsorListRendererSpy,
		User:                "testusername",
	}

	// When I run the list command
	err := listcmd.ListSponsors(listOptions)

	// Then it is successful
	require.NoError(t, err)

	// And it renders the list
	require.Equal(t, sponsorListRendererSpy.spiedSponsors, listcmd.Sponsors{{"sponsor1"}, {"sponsor2"}})
}

type fakeUserPrompter struct {
	stubbedUser  listcmd.User
	stubbedError error
}

func (f fakeUserPrompter) PromptForUser() (listcmd.User, error) {
	if f.stubbedError != nil {
		return "", f.stubbedError
	}
	return f.stubbedUser, nil
}

func TestListSponsorsWrapsErrorRetrievingSponsors(t *testing.T) {
	// Given our sponsor lister returns with an error
	listOptions := listcmd.Options{
		SponsorLister: fakeSponsorLister{
			stubbedErr: errors.New("expected test error"),
		},
		User: "testusername",
	}

	// When I run the list command
	err := listcmd.ListSponsors(listOptions)

	// Then it returns an informational error
	require.ErrorContains(t, err, "sponsor list: expected test error")
}

func TestListSponsorsPromptsWhenNoUserProvidedTTY(t *testing.T) {
	// Given no user was provided
	sponsorListRendererSpy := &spySponsorTableRenderer{}
	listOptions := listcmd.Options{
		SponsorLister: fakeSponsorLister{
			stubbedSponsors: map[listcmd.User]listcmd.Sponsors{
				"stubbedusername": {{"sponsor1"}, {"sponsor2"}},
			},
		},
		SponsorListRenderer: sponsorListRendererSpy,

		UserPrompter: fakeUserPrompter{
			stubbedUser: "stubbedusername",
		},
	}

	// When I run the list command
	err := listcmd.ListSponsors(listOptions)

	// Then it is successful
	require.NoError(t, err)

	// And it uses the user provided by the prompt
	require.Equal(t, sponsorListRendererSpy.spiedSponsors, listcmd.Sponsors{{"sponsor1"}, {"sponsor2"}})
}

func TestListingSponsorsWrapsErrorWhenPromptingForUser(t *testing.T) {
	// Given prompting for a user errors
	listOptions := listcmd.Options{
		UserPrompter: fakeUserPrompter{
			stubbedError: errors.New("expected test error"),
		},
	}

	// When I run the list command
	err := listcmd.ListSponsors(listOptions)

	// Then it returns an informational error
	require.ErrorContains(t, err, "prompt for user: expected test error")
}
